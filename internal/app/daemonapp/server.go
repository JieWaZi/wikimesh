package daemonapp

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/JieWaZi/wikimesh/pkg/agent"
)

// Options 配置本地 Codex daemon。
type Options struct {
	// Addr 是 daemon HTTP 监听地址。
	Addr string
	// DBPath 是 SQLite 状态库路径；为空时使用 ~/.wikimesh/daemon/state.db。
	DBPath string
	// CodexExecutable 是 codex 可执行文件路径；为空时使用 PATH 中的 codex。
	CodexExecutable string
	// Logger 是 daemon 日志输出；为空时使用 slog.Default。
	Logger *slog.Logger
}

// Server 是 wikimesh 的本地 Codex daemon HTTP 服务。
type Server struct {
	// opts 保存 daemon 启动参数。
	opts Options
	// store 保存 wikimesh session 到 Codex thread 的最小映射。
	store *Store
	// startedAt 用于 /health 展示运行时长。
	startedAt time.Time
	// cancel 由 /shutdown 触发，关闭前台 daemon。
	cancel context.CancelFunc
}

// NewServer 创建 daemon 服务。
func NewServer(opts Options) (*Server, error) {
	if opts.Addr == "" {
		opts.Addr = "127.0.0.1:19515"
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	store, err := OpenStore(opts.DBPath)
	if err != nil {
		return nil, err
	}
	return &Server{opts: opts, store: store, startedAt: time.Now()}, nil
}

// Close 关闭 daemon 资源。
func (s *Server) Close() error {
	return s.store.Close()
}

// Run 启动 HTTP 服务并阻塞到 context 取消。
func (s *Server) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	defer cancel()

	ln, err := net.Listen("tcp", s.opts.Addr)
	if err != nil {
		return err
	}
	server := &http.Server{Handler: s.Handler()}
	go func() {
		// 与 Multica 的本地 health server 类似，顶层 context 取消时优雅停服。
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	err = server.Serve(ln)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Handler 返回可测试的 HTTP handler。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/shutdown", s.handleShutdown)
	mux.HandleFunc("/v1/agents", s.handleAgents)
	mux.HandleFunc("/v1/agents/", s.handleAgentPath)
	mux.HandleFunc("/v1/sessions", s.handleSessions)
	mux.HandleFunc("/v1/sessions/", s.handleSession)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	execPath := s.opts.CodexExecutable
	if execPath == "" {
		execPath = "codex"
	}
	_, codexErr := exec.LookPath(execPath)
	writeJSON(w, http.StatusOK, map[string]any{
		"status":       "running",
		"version":      serviceVersion,
		"startedAt":    s.startedAt.UTC().Format(time.RFC3339Nano),
		"pid":          os.Getpid(),
		"os":           runtime.GOOS,
		"uptime":       time.Since(s.startedAt).Truncate(time.Second).String(),
		"agents":       []string{defaultAgentID},
		"defaultAgent": defaultAgentID,
		"capabilities": map[string]bool{
			"sessions":      true,
			"history":       true,
			"aguiSse":       true,
			"cancelRun":     false,
			"renameSession": true,
			"deleteSession": true,
		},
		"agentsStatus": map[string]any{
			defaultAgentID: map[string]any{
				"available": codexErr == nil,
				"error":     errorString(codexErr),
			},
		},
	})
}

func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "shutting_down"})
	if s.cancel != nil {
		go s.cancel()
	}
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		agentID := strings.TrimSpace(r.URL.Query().Get("agent"))
		local, err := s.store.ListSessions(r.Context(), agentID, limit)
		if err != nil {
			writeInternalError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"sessions": local, "nextCursor": nil})
	case http.MethodPost:
		var req createSessionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeBadRequest(w, "invalid request body: "+err.Error())
			return
		}
		agentID := firstNonEmpty(req.AgentID, req.Agent)
		if agentID == "" {
			agentID = defaultAgentID
		}
		if !s.isKnownAgent(agentID) {
			writeAPIError(w, http.StatusNotFound, errorAgentNotFound, "Agent not found", map[string]any{"agentId": agentID})
			return
		}
		cwd, err := ResolveCwd(req.Cwd, req.Project)
		if err != nil {
			writeBadRequest(w, err.Error())
			return
		}
		rec, err := s.store.CreateSession(r.Context(), SessionRecord{
			ID:       req.ID,
			AgentID:  agentID,
			Title:    firstNonEmpty(req.Title, defaultSessionTitle),
			Cwd:      cwd,
			Project:  req.Project,
			Metadata: req.Metadata,
		})
		if err != nil {
			writeInternalError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"session": rec})
	default:
		writeAPIError(w, http.StatusMethodNotAllowed, errorBadRequest, "method not allowed", nil)
	}
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/v1/sessions/")
	id = strings.Trim(id, "/")
	if id == "" {
		writeBadRequest(w, "missing session id")
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.readSession(w, r, id)
	case http.MethodPatch:
		s.patchSession(w, r, id)
	case http.MethodDelete:
		s.deleteSession(w, r, id)
	default:
		writeAPIError(w, http.StatusMethodNotAllowed, errorBadRequest, "method not allowed", nil)
	}
}

func (s *Server) readSession(w http.ResponseWriter, r *http.Request, id string) {
	rec, err := s.store.GetSession(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeAPIError(w, http.StatusNotFound, errorSessionNotFound, "Session not found", map[string]any{"sessionId": id})
		return
	}
	if err != nil {
		writeInternalError(w, err)
		return
	}
	messages, err := s.store.ListMessages(r.Context(), id)
	historyError := any(nil)
	if err != nil {
		messages = []MessageRecord{}
		historyError = err.Error()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"session":      rec,
		"messages":     messages,
		"hasMore":      false,
		"historyError": historyError,
	})
}

func (s *Server) patchSession(w http.ResponseWriter, r *http.Request, id string) {
	var req updateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "invalid request body: "+err.Error())
		return
	}
	rec, err := s.store.UpdateSession(r.Context(), id, req.Title, req.Metadata)
	if errors.Is(err, sql.ErrNoRows) {
		writeAPIError(w, http.StatusNotFound, errorSessionNotFound, "Session not found", map[string]any{"sessionId": id})
		return
	}
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"session": rec})
}

func (s *Server) deleteSession(w http.ResponseWriter, r *http.Request, id string) {
	if err := s.store.DeleteSession(r.Context(), id); errors.Is(err, sql.ErrNoRows) {
		writeAPIError(w, http.StatusNotFound, errorSessionNotFound, "Session not found", map[string]any{"sessionId": id})
		return
	} else if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "sessionId": id})
}

func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, errorBadRequest, "method not allowed", nil)
		return
	}
	available, errMsg := s.agentAvailability(defaultAgentID)
	writeJSON(w, http.StatusOK, map[string]any{
		"agents": []map[string]any{{
			"id":          defaultAgentID,
			"label":       "Codex",
			"description": "Local Codex coding agent",
			"icon":        "codex",
			"default":     true,
			"available":   available,
			"error":       errMsg,
			"capabilities": map[string]bool{
				"streaming": true,
				"tools":     true,
				"history":   true,
				"cancel":    false,
			},
		}},
	})
}

func (s *Server) handleAgentPath(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/v1/agents/")
	parts := strings.Split(strings.Trim(trimmed, "/"), "/")
	if len(parts) == 2 && parts[1] == "runs" {
		s.handleAgentRun(w, r, parts[0])
		return
	}
	writeAPIError(w, http.StatusNotFound, errorBadRequest, "not found", nil)
}

func (s *Server) agentConfig() agent.Config {
	return agent.Config{
		ExecutablePath: s.opts.CodexExecutable,
		Logger:         s.opts.Logger,
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func limitOrDefault(v, fallback int) int {
	if v <= 0 {
		return fallback
	}
	return v
}

type createSessionRequest struct {
	// ID 是前端指定的 wikimesh session id；为空时由服务端生成。
	ID string `json:"id"`
	// AgentID 是会话绑定的 agent。
	AgentID string `json:"agentId"`
	// Agent 是 agentId 的兼容别名。
	Agent string `json:"agent"`
	// Title 是前端展示标题。
	Title string `json:"title"`
	// Cwd 是后续 Codex 执行目录。
	Cwd string `json:"cwd"`
	// Project 是 Wikimesh 文档库项目名；未传 cwd 时会解析为本地文档库目录。
	Project string `json:"project"`
	// Metadata 是前端附加元数据。
	Metadata map[string]any `json:"metadata"`
}

type updateSessionRequest struct {
	// Title 为 nil 表示不更新标题；空字符串会被规范为默认标题。
	Title *string `json:"title"`
	// Metadata 为 nil 表示不更新 metadata。
	Metadata map[string]any `json:"metadata"`
}

func (s *Server) isKnownAgent(agentID string) bool {
	return agentID == defaultAgentID
}

func (s *Server) agentAvailability(agentID string) (bool, string) {
	if !s.isKnownAgent(agentID) {
		return false, "unknown agent"
	}
	execPath := s.opts.CodexExecutable
	if execPath == "" {
		execPath = "codex"
	}
	if _, err := exec.LookPath(execPath); err != nil {
		return false, err.Error()
	}
	return true, ""
}
