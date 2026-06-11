package daemonapp

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/JieWaZi/wikimesh/pkg/agent"
)

type runAgentInput struct {
	// ThreadID 是 AG-UI/CopilotKit 的会话 ID，对应本地 sessions.id。
	ThreadID string `json:"threadId"`
	// RunID 是 AG-UI 本轮执行 ID；为空时由 daemon 生成。
	RunID string `json:"runId"`
	// Messages 是 CopilotKit 传入的消息历史，MVP 只取最后一条用户消息。
	Messages []inputMessage `json:"messages"`
	// State 预留给 AG-UI 状态同步，MVP 不解释。
	State any `json:"state,omitempty"`
	// Context 预留给 AG-UI 上下文输入，MVP 不解释。
	Context []any `json:"context,omitempty"`
	// Message 是非标准简化输入，便于 curl 或调试脚本直接调用。
	Message string `json:"message,omitempty"`
	// Cwd 是本轮 Codex 执行目录。
	Cwd string `json:"cwd,omitempty"`
	// Title 是新会话标题或 Codex thread name 候选。
	Title string `json:"title,omitempty"`
	// Project 是 Wikimesh 文档库项目名；未传 cwd 时会解析为本地文档库目录。
	Project string `json:"project,omitempty"`
	// Model 是 Codex 模型覆盖。
	Model string `json:"model,omitempty"`
	// Agent 是兼容字段；实际 agent 以 URL path 为准。
	Agent string `json:"agent,omitempty"`
}

type inputMessage struct {
	// ID 是前端消息 ID，MVP 只透传解析，不落库。
	ID string `json:"id,omitempty"`
	// Role 是 user、assistant 等消息角色。
	Role string `json:"role"`
	// Content 是字符串或 OpenAI/CopilotKit 风格 content parts。
	Content any `json:"content"`
}

func (s *Server) handleAgentRun(w http.ResponseWriter, r *http.Request, agentID string) {
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, errorBadRequest, "method not allowed", nil)
		return
	}
	if !s.isKnownAgent(agentID) {
		writeAPIError(w, http.StatusNotFound, errorAgentNotFound, "Agent not found", map[string]any{"agentId": agentID})
		return
	}
	var input runAgentInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeBadRequest(w, "invalid request body: "+err.Error())
		return
	}
	prompt := latestUserPrompt(input)
	if strings.TrimSpace(prompt) == "" {
		writeBadRequest(w, "missing user message")
		return
	}
	runID := input.RunID
	if runID == "" {
		runID = newID("run")
	}
	// 前端 threadId 映射到本地 session；真正上下文由 rec.CodexThreadID 承载。
	rec, err := s.loadOrCreateRunSession(r.Context(), agentID, input, prompt)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if rec.AgentID != agentID {
		writeAPIError(w, http.StatusConflict, errorAgentMismatch, "Session belongs to a different agent", map[string]any{"sessionId": rec.ID, "agentId": rec.AgentID})
		return
	}
	if running, err := s.store.RunningRunForSession(r.Context(), rec.ID); err == nil && running.ID != "" {
		writeAPIError(w, http.StatusConflict, errorSessionBusy, "Session already has a running run", map[string]any{"sessionId": rec.ID, "runId": running.ID})
		return
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		writeInternalError(w, err)
		return
	}
	effectiveCwd, err := ResolveRunCwd(input.Cwd, input.Project, rec.Cwd, rec.Project)
	if err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	run, err := s.store.CreateRun(r.Context(), RunRecord{ID: runID, SessionID: rec.ID, AgentID: agentID, Status: "running"})
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if err := s.store.UpdateSessionRun(r.Context(), rec.ID, "", run.ID, "running"); err != nil {
		writeInternalError(w, err)
		return
	}
	userMessageID := latestUserMessageID(input)
	if userMessageID == "" {
		userMessageID = newID("msg_user")
	}
	if _, err := s.store.SaveMessage(r.Context(), MessageRecord{
		ID:        userMessageID,
		SessionID: rec.ID,
		RunID:     run.ID,
		Role:      "user",
		Content:   prompt,
	}); err != nil {
		writeInternalError(w, err)
		return
	}

	// AG-UI HTTP transport 使用 SSE，每个事件编码为 data: JSON。
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, _ := w.(http.Flusher)
	emit := func(event map[string]any) error {
		if event["threadId"] == nil {
			event["threadId"] = rec.ID
		}
		if event["runId"] == nil {
			event["runId"] = run.ID
		}
		if event["timestamp"] == nil {
			event["timestamp"] = time.Now().UTC().Format(time.RFC3339Nano)
		}
		data, err := json.Marshal(event)
		if err != nil {
			return err
		}
		eventType, _ := event["type"].(string)
		_, _ = s.store.SaveAGUIEvent(context.Background(), AGUIEventRecord{
			SessionID: rec.ID,
			RunID:     run.ID,
			EventType: eventType,
			EventJSON: string(data),
		})
		if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			return err
		}
		if flusher != nil {
			flusher.Flush()
		}
		return nil
	}

	// AG-UI 要求每个 run 以 RUN_STARTED 开始。
	_ = emit(map[string]any{
		"type": "RUN_STARTED",
	})

	backend, err := agent.New("codex", s.agentConfig())
	if err != nil {
		s.emitRunError(context.Background(), emit, rec.ID, run.ID, err, errorBackendFailed)
		return
	}
	// Codex 的多轮续聊只传 ResumeSessionID，不重放前端消息历史。
	sess, err := backend.Execute(r.Context(), prompt, agent.ExecOptions{
		Cwd:             effectiveCwd,
		Model:           input.Model,
		ThreadName:      firstNonEmpty(input.Title, rec.Title),
		ResumeSessionID: rec.BackendThreadID,
	})
	if err != nil {
		s.emitRunError(context.Background(), emit, rec.ID, run.ID, err, errorBackendFailed)
		return
	}

	// Codex 通知先进入中间 Message，再由 adapter 翻译为 AG-UI 事件。
	adapter := newAGUIAdapter(rec.ID, run.ID, s.store, emit)
	for msg := range sess.Messages {
		if err := adapter.HandleMessage(msg); err != nil {
			s.opts.Logger.Warn("ag-ui emit failed", "error", err)
			_ = s.store.FinishRun(context.Background(), run.ID, "failed", errorBackendFailed, err.Error())
			_ = s.store.UpdateSessionRun(context.Background(), rec.ID, "", run.ID, "failed")
			return
		}
	}
	result := <-sess.Result
	if err := adapter.CloseText(); err != nil {
		s.opts.Logger.Warn("ag-ui close text failed", "error", err)
		return
	}

	status := "idle"
	if result.Error != "" || result.Status == "failed" || result.Status == "timeout" {
		status = "failed"
		_ = s.store.FinishRun(context.Background(), run.ID, "failed", errorBackendFailed, firstNonEmpty(result.Error, result.Status))
		if err := s.store.UpdateSessionRun(context.Background(), rec.ID, result.SessionID, run.ID, status); err != nil {
			s.opts.Logger.Warn("update session run failed", "session_id", rec.ID, "error", err)
		}
		_ = emit(map[string]any{
			"type":    "RUN_ERROR",
			"code":    errorBackendFailed,
			"message": firstNonEmpty(result.Error, result.Status),
		})
	} else {
		_ = s.store.FinishRun(context.Background(), run.ID, "completed", "", "")
		if err := s.store.UpdateSessionRun(context.Background(), rec.ID, result.SessionID, run.ID, status); err != nil {
			s.opts.Logger.Warn("update session run failed", "session_id", rec.ID, "error", err)
		}
		_ = emit(map[string]any{
			"type": "RUN_FINISHED",
		})
	}
}

func (s *Server) loadOrCreateRunSession(ctx context.Context, agentID string, input runAgentInput, prompt string) (SessionRecord, error) {
	id := input.ThreadID
	if id == "" {
		id = newID("sess")
	}
	rec, err := s.store.GetSession(ctx, id)
	if err == nil {
		return rec, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return SessionRecord{}, err
	}
	codexThreadID := ""
	if strings.HasPrefix(id, "thr_") {
		// 调试过渡：允许直接把 Codex thread id 当成前端 threadId 传入。
		codexThreadID = id
	}
	cwd, err := ResolveRunCwd(input.Cwd, input.Project, "", "")
	if err != nil {
		return SessionRecord{}, err
	}
	return s.store.CreateSession(ctx, SessionRecord{
		ID:              id,
		AgentID:         agentID,
		BackendThreadID: codexThreadID,
		Title:           firstNonEmpty(input.Title, autoTitle(prompt), defaultSessionTitle),
		Cwd:             cwd,
		Project:         input.Project,
	})
}

func (s *Server) emitRunError(ctx context.Context, emit func(map[string]any) error, threadID, runID string, err error, code string) {
	_ = s.store.UpdateSessionRun(ctx, threadID, "", runID, "failed")
	_ = s.store.FinishRun(ctx, runID, "failed", code, err.Error())
	_ = emit(map[string]any{
		"type":     "RUN_ERROR",
		"threadId": threadID,
		"runId":    runID,
		"code":     code,
		"message":  err.Error(),
	})
}

func latestUserPrompt(input runAgentInput) string {
	// 优先使用简化 message；否则兼容 AG-UI/CopilotKit 的 messages 数组。
	if strings.TrimSpace(input.Message) != "" {
		return input.Message
	}
	for i := len(input.Messages) - 1; i >= 0; i-- {
		msg := input.Messages[i]
		if msg.Role != "" && msg.Role != "user" {
			continue
		}
		if text := messageContentText(msg.Content); text != "" {
			return text
		}
	}
	return ""
}

func latestUserMessageID(input runAgentInput) string {
	for i := len(input.Messages) - 1; i >= 0; i-- {
		msg := input.Messages[i]
		if msg.Role != "" && msg.Role != "user" {
			continue
		}
		if messageContentText(msg.Content) != "" {
			return msg.ID
		}
	}
	return ""
}

func messageContentText(v any) string {
	// CopilotKit 可能传字符串，也可能传 content parts；MVP 只抽取 text。
	switch x := v.(type) {
	case string:
		return x
	case []any:
		var parts []string
		for _, item := range x {
			if m, ok := item.(map[string]any); ok {
				if text, _ := m["text"].(string); text != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

type aguiAdapter struct {
	// threadID 是 AG-UI threadId，即 wikimesh session id。
	threadID string
	// runID 是本轮 AG-UI run id。
	runID string
	// emit 负责实际写 SSE。
	emit func(map[string]any) error
	// store 保存 normalized messages。
	store *Store
	// messageID 是当前打开的 assistant 文本消息 ID。
	messageID string
	// openText 标记 TEXT_MESSAGE_START 是否已经发送。
	openText bool
	// text 聚合当前 assistant 文本，用于 TEXT_MESSAGE_END 时落库。
	text strings.Builder
	// parentMessageID 是 tool call 绑定的 assistant message。
	parentMessageID string
}

func newAGUIAdapter(threadID, runID string, store *Store, emit func(map[string]any) error) *aguiAdapter {
	return &aguiAdapter{threadID: threadID, runID: runID, store: store, emit: emit}
}

func (a *aguiAdapter) HandleMessage(msg agent.Message) error {
	switch msg.Type {
	case agent.MessageText:
		if msg.Content == "" {
			return nil
		}
		if !a.openText {
			// AG-UI 文本必须按 START -> CONTENT* -> END 成对输出。
			a.messageID = newID("msg")
			a.parentMessageID = a.messageID
			a.text.Reset()
			a.openText = true
			if err := a.emitEvent(map[string]any{
				"type":      "TEXT_MESSAGE_START",
				"messageId": a.messageID,
				"role":      "assistant",
			}); err != nil {
				return err
			}
		}
		a.text.WriteString(msg.Content)
		return a.emitEvent(map[string]any{
			"type":      "TEXT_MESSAGE_CONTENT",
			"messageId": a.messageID,
			"delta":     msg.Content,
		})
	case agent.MessageToolUse:
		// 工具事件出现前先关闭文本消息，避免前端消息生命周期交叉。
		if err := a.CloseText(); err != nil {
			return err
		}
		toolCallID := firstNonEmpty(msg.CallID, newID("tool"))
		parentMessageID := firstNonEmpty(a.parentMessageID, newID("msg_assistant"))
		a.parentMessageID = parentMessageID
		if err := a.emitEvent(map[string]any{
			"type":            "TOOL_CALL_START",
			"parentMessageId": parentMessageID,
			"toolCallId":      toolCallID,
			"toolCallName":    msg.Tool,
		}); err != nil {
			return err
		}
		if len(msg.Input) > 0 {
			args, _ := json.Marshal(msg.Input)
			if err := a.emitEvent(map[string]any{
				"type":       "TOOL_CALL_ARGS",
				"toolCallId": toolCallID,
				"delta":      string(args),
			}); err != nil {
				return err
			}
		}
		return nil
	case agent.MessageToolResult:
		// Codex 工具结果映射为 AG-UI RESULT + END，供前端展示命令输出。
		if err := a.CloseText(); err != nil {
			return err
		}
		toolCallID := firstNonEmpty(msg.CallID, newID("tool"))
		if err := a.emitEvent(map[string]any{
			"type":       "TOOL_CALL_END",
			"toolCallId": toolCallID,
		}); err != nil {
			return err
		}
		if msg.Output != "" {
			messageID := newID("msg_tool")
			if _, err := a.store.SaveMessage(context.Background(), MessageRecord{
				ID:         messageID,
				SessionID:  a.threadID,
				RunID:      a.runID,
				Role:       "tool",
				ToolCallID: toolCallID,
				Content:    msg.Output,
			}); err != nil {
				return err
			}
			return a.emitEvent(map[string]any{
				"type":       "TOOL_CALL_RESULT",
				"messageId":  messageID,
				"role":       "tool",
				"toolCallId": toolCallID,
				"content":    msg.Output,
			})
		}
		return nil
	case agent.MessageError:
		return a.emitEvent(map[string]any{
			"type":    "RUN_ERROR",
			"code":    errorBackendFailed,
			"message": msg.Content,
		})
	default:
		return nil
	}
}

func (a *aguiAdapter) CloseText() error {
	if !a.openText {
		return nil
	}
	a.openText = false
	if _, err := a.store.SaveMessage(context.Background(), MessageRecord{
		ID:        a.messageID,
		SessionID: a.threadID,
		RunID:     a.runID,
		Role:      "assistant",
		Content:   a.text.String(),
	}); err != nil {
		return err
	}
	return a.emitEvent(map[string]any{
		"type":      "TEXT_MESSAGE_END",
		"messageId": a.messageID,
	})
}

func (a *aguiAdapter) emitEvent(event map[string]any) error {
	if event["threadId"] == nil {
		event["threadId"] = a.threadID
	}
	if event["runId"] == nil {
		event["runId"] = a.runID
	}
	return a.emit(event)
}

func autoTitle(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return ""
	}
	runes := []rune(prompt)
	if len(runes) > autoSessionTitleRunes {
		return string(runes[:autoSessionTitleRunes])
	}
	return prompt
}
