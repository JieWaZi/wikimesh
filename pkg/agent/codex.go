package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const codexGracefulShutdownTimeout = 5 * time.Second

type codexBackend struct {
	// cfg 保存 Codex 可执行文件、环境变量和日志配置。
	cfg Config
}

// CodexThreadsResult 是 Codex thread/list 的原始响应。
type CodexThreadsResult struct {
	// Raw 保留 Codex 原始 JSON，避免 MVP 阶段过早绑定协议结构。
	Raw json.RawMessage
}

// CodexThreadResult 是 Codex thread/read 的原始响应。
type CodexThreadResult struct {
	// Raw 保留 Codex 原始 JSON，交给 web 层或后续适配层解释。
	Raw json.RawMessage
}

// ListCodexThreads 通过 Codex app-server 读取本地 Codex 历史会话列表。
func ListCodexThreads(ctx context.Context, cfg Config, params map[string]any) (CodexThreadsResult, error) {
	raw, err := codexOneShot(ctx, cfg, "thread/list", params)
	if err != nil {
		return CodexThreadsResult{}, err
	}
	return CodexThreadsResult{Raw: raw}, nil
}

// ReadCodexThread 通过 Codex app-server 读取单个本地 Codex thread。
func ReadCodexThread(ctx context.Context, cfg Config, threadID string, includeTurns bool) (CodexThreadResult, error) {
	params := map[string]any{
		"threadId":     threadID,
		"includeTurns": includeTurns,
	}
	raw, err := codexOneShot(ctx, cfg, "thread/read", params)
	if err != nil {
		return CodexThreadResult{}, err
	}
	return CodexThreadResult{Raw: raw}, nil
}

func (b *codexBackend) Execute(ctx context.Context, prompt string, opts ExecOptions) (*Session, error) {
	execPath := b.cfg.ExecutablePath
	if strings.TrimSpace(execPath) == "" {
		execPath = "codex"
	}
	if _, err := exec.LookPath(execPath); err != nil {
		return nil, fmt.Errorf("codex executable not found at %q: %w", execPath, err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	if opts.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, opts.Timeout)
	}

	// 与 Multica 保持一致：Codex daemon 集成走 app-server stdio JSON-RPC，
	// 不使用 codex exec，这样才能拿到 thread id、turn 事件和工具事件。
	cmd := exec.CommandContext(runCtx, execPath, "app-server", "--listen", "stdio://")
	cmd.WaitDelay = 10 * time.Second
	if opts.Cwd != "" {
		cmd.Dir = opts.Cwd
	}
	cmd.Env = buildEnv(b.cfg.Env)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("codex stdout pipe: %w", err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("codex stdin pipe: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = io.MultiWriter(&stderr)

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start codex: %w", err)
	}
	b.cfg.Logger.Info("codex app-server started", "pid", cmd.Process.Pid, "cwd", opts.Cwd)

	msgCh := make(chan Message, 256)
	resCh := make(chan Result, 1)
	turnDone := make(chan bool, 1)

	var outputMu sync.Mutex
	var output strings.Builder

	c := &codexClient{
		cfg:     b.cfg,
		stdin:   stdin,
		pending: make(map[int]*pendingRPC),
		onMessage: func(msg Message) {
			// 聚合文本只用于最终 Result；实时展示仍以 Messages 流为准。
			if msg.Type == MessageText {
				outputMu.Lock()
				output.WriteString(msg.Content)
				outputMu.Unlock()
			}
			select {
			case msgCh <- msg:
			default:
			}
		},
		onTurnDone: func(aborted bool) {
			select {
			case turnDone <- aborted:
			default:
			}
		},
	}

	readerDone := make(chan struct{})
	go readCodexLines(stdout, c, readerDone)

	var waitOnce sync.Once
	drainAndWait := func() {
		waitOnce.Do(func() {
			// 先关 stdin 让 Codex 正常退出，再 Wait，避免 stderr/stdout 还未刷完。
			_ = stdin.Close()
			_ = cmd.Wait()
		})
	}

	go func() {
		defer cancel()
		defer close(msgCh)
		defer close(resCh)
		defer drainAndWait()

		startedAt := time.Now()
		finalStatus := "completed"
		finalError := ""

		// Codex app-server 要求每个 transport 连接先 initialize 再 initialized。
		if err := c.initialize(runCtx); err != nil {
			drainAndWait()
			resCh <- Result{Status: "failed", Error: appendStderr(fmt.Sprintf("codex initialize failed: %v", err), stderr.String()), DurationMs: time.Since(startedAt).Milliseconds()}
			return
		}

		// 优先恢复已有 Codex thread；恢复失败时按 Multica 策略退回新 thread。
		threadID, resumed, err := c.startOrResumeThread(runCtx, opts)
		if err != nil {
			drainAndWait()
			resCh <- Result{Status: "failed", Error: appendStderr(err.Error(), stderr.String()), DurationMs: time.Since(startedAt).Milliseconds()}
			return
		}
		c.threadID = threadID
		if resumed {
			b.cfg.Logger.Info("codex thread resumed", "thread_id", threadID)
		} else {
			b.cfg.Logger.Info("codex thread started", "thread_id", threadID)
		}

		// thread 确定后，启动本轮 turn；后续进度都从 stdout 通知流读取。
		_, err = c.request(runCtx, "turn/start", map[string]any{
			"threadId": threadID,
			"input": []map[string]any{
				{"type": "text", "text": prompt},
			},
		})
		if err != nil {
			drainAndWait()
			resCh <- Result{Status: "failed", SessionID: threadID, Error: appendStderr(fmt.Sprintf("codex turn/start failed: %v", err), stderr.String()), DurationMs: time.Since(startedAt).Milliseconds()}
			return
		}

		// 等待 turn/completed、错误通知或上层 context 取消。
		select {
		case aborted := <-turnDone:
			if aborted {
				finalStatus = "aborted"
				finalError = "turn was aborted"
			} else if errMsg := c.getTurnError(); errMsg != "" {
				finalStatus = "failed"
				finalError = errMsg
			}
		case <-runCtx.Done():
			if runCtx.Err() == context.DeadlineExceeded {
				finalStatus = "timeout"
				finalError = fmt.Sprintf("codex timed out after %s", opts.Timeout)
			} else {
				finalStatus = "aborted"
				finalError = "execution cancelled"
			}
		}

		// 让 Codex 有机会优雅退出并刷新事件/遥测；超时后再取消进程。
		_ = stdin.Close()
		select {
		case <-readerDone:
		case <-time.After(codexGracefulShutdownTimeout):
			cancel()
			<-readerDone
		}
		drainAndWait()

		outputMu.Lock()
		finalOutput := output.String()
		outputMu.Unlock()
		if finalError != "" {
			finalError = appendStderr(finalError, stderr.String())
		}
		resCh <- Result{
			Status:     finalStatus,
			Output:     finalOutput,
			Error:      finalError,
			DurationMs: time.Since(startedAt).Milliseconds(),
			SessionID:  threadID,
			TurnID:     c.turnID,
		}
	}()

	return &Session{Messages: msgCh, Result: resCh}, nil
}

func codexOneShot(ctx context.Context, cfg Config, method string, params map[string]any) (json.RawMessage, error) {
	execPath := cfg.ExecutablePath
	if strings.TrimSpace(execPath) == "" {
		execPath = "codex"
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if _, err := exec.LookPath(execPath); err != nil {
		return nil, fmt.Errorf("codex executable not found at %q: %w", execPath, err)
	}
	// 历史读取也复用 app-server 协议，避免直接解析 ~/.codex/sessions。
	cmd := exec.CommandContext(ctx, execPath, "app-server", "--listen", "stdio://")
	cmd.WaitDelay = 10 * time.Second
	cmd.Env = buildEnv(cfg.Env)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("codex stdout pipe: %w", err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("codex stdin pipe: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start codex: %w", err)
	}
	c := &codexClient{cfg: cfg, stdin: stdin, pending: make(map[int]*pendingRPC)}
	readerDone := make(chan struct{})
	go readCodexLines(stdout, c, readerDone)
	defer func() {
		_ = stdin.Close()
		_ = cmd.Wait()
	}()
	if err := c.initialize(ctx); err != nil {
		return nil, appendStderrErr(fmt.Errorf("codex initialize failed: %w", err), stderr.String())
	}
	raw, err := c.request(ctx, method, params)
	if err != nil {
		return nil, appendStderrErr(fmt.Errorf("codex %s failed: %w", method, err), stderr.String())
	}
	_ = stdin.Close()
	select {
	case <-readerDone:
	case <-time.After(codexGracefulShutdownTimeout):
	}
	return raw, nil
}

func readCodexLines(stdout io.Reader, c *codexClient, done chan<- struct{}) {
	defer close(done)
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		c.handleLine(line)
	}
	// stdout 结束时释放所有等待中的 JSON-RPC 请求，避免调用方永久阻塞。
	c.closeAllPending(fmt.Errorf("codex process exited"))
}

func buildEnv(extra map[string]string) []string {
	env := os.Environ()
	for k, v := range extra {
		env = append(env, k+"="+v)
	}
	return env
}

func appendStderr(msg, stderr string) string {
	stderr = strings.TrimSpace(stderr)
	if stderr == "" {
		return msg
	}
	if len(stderr) > 2048 {
		// 错误信息只带尾部，避免 Codex 大量 stderr 淹没 API 响应。
		stderr = stderr[len(stderr)-2048:]
	}
	return msg + "\n[codex stderr]\n" + stderr
}

func appendStderrErr(err error, stderr string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s", appendStderr(err.Error(), stderr))
}

func (c *codexClient) initialize(ctx context.Context) error {
	_, err := c.request(ctx, "initialize", map[string]any{
		"clientInfo": map[string]any{
			"name":    "wikimesh",
			"title":   "Wikimesh",
			"version": "0.1.0",
		},
		"capabilities": map[string]any{
			"experimentalApi": true,
		},
	})
	if err != nil {
		return err
	}
	c.notify("initialized", map[string]any{})
	return nil
}

func (c *codexClient) startOrResumeThread(ctx context.Context, opts ExecOptions) (string, bool, error) {
	if opts.ResumeSessionID != "" {
		// Codex thread 是真正的上下文持久化载体；wikimesh 只保存它的 ID。
		raw, err := c.request(ctx, "thread/resume", map[string]any{
			"threadId":              opts.ResumeSessionID,
			"cwd":                   nilIfEmpty(opts.Cwd),
			"model":                 nilIfEmpty(opts.Model),
			"developerInstructions": nilIfEmpty(opts.SystemPrompt),
		})
		if err == nil {
			if threadID := extractThreadID(raw); threadID != "" {
				return threadID, true, nil
			}
		}
		c.cfg.Logger.Warn("codex thread/resume failed; falling back to thread/start", "thread_id", opts.ResumeSessionID, "error", err)
	}
	// 新会话开启时持久化扩展历史，便于后续 thread/read/list 供前端回看。
	raw, err := c.request(ctx, "thread/start", map[string]any{
		"model":                  nilIfEmpty(opts.Model),
		"cwd":                    nilIfEmpty(opts.Cwd),
		"developerInstructions":  nilIfEmpty(opts.SystemPrompt),
		"persistExtendedHistory": true,
	})
	if err != nil {
		return "", false, fmt.Errorf("codex thread/start failed: %w", err)
	}
	threadID := extractThreadID(raw)
	if threadID == "" {
		return "", false, fmt.Errorf("codex thread/start returned no thread ID")
	}
	if opts.ThreadName != "" {
		_, _ = c.request(ctx, "thread/name/set", map[string]any{
			"threadId": threadID,
			"name":     opts.ThreadName,
		})
	}
	return threadID, false, nil
}

func extractThreadID(raw json.RawMessage) string {
	var parsed struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	_ = json.Unmarshal(raw, &parsed)
	return parsed.Thread.ID
}

func nilIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
