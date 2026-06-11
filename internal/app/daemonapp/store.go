package daemonapp

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// SessionRecord 是 MeshWiki 对外会话与 agent 原生 thread 的映射。
type SessionRecord struct {
	// ID 是 wikimesh 前端会话 ID，也作为 AG-UI threadId。
	ID string `json:"id"`
	// AgentID 是该会话绑定的 agent，MVP 固定为 codex。
	AgentID string `json:"agentId"`
	// BackendThreadID 是 Codex 原生 thread.id，负责真正的上下文续聊。
	BackendThreadID string `json:"backendThreadId,omitempty"`
	// Title 是前端展示标题。
	Title string `json:"title,omitempty"`
	// Cwd 是 Codex 执行目录。
	Cwd string `json:"cwd,omitempty"`
	// Project 是 Wikimesh 文档库项目名。
	Project string `json:"project,omitempty"`
	// LastRunID 是最近一次 AG-UI run id。
	LastRunID string `json:"lastRunId,omitempty"`
	// Status 是轻量运行状态，例如 idle、running、failed。
	Status string `json:"status"`
	// Metadata 保存 web 侧附加元信息。
	Metadata map[string]any `json:"metadata,omitempty"`
	// CreatedAt 是创建时间，使用 RFC3339Nano 字符串。
	CreatedAt string `json:"createdAt"`
	// UpdatedAt 是最近更新时间，使用 RFC3339Nano 字符串。
	UpdatedAt string `json:"updatedAt"`
}

// RunRecord 记录一次用户消息触发的 agent 执行。
type RunRecord struct {
	// ID 是 AG-UI runId。
	ID string `json:"id"`
	// SessionID 是所属 MeshWiki session。
	SessionID string `json:"sessionId"`
	// AgentID 是执行该 run 的 agent。
	AgentID string `json:"agentId"`
	// Status 是 queued、running、completed、failed 或 cancelled。
	Status string `json:"status"`
	// ErrorCode 是失败错误码。
	ErrorCode string `json:"errorCode,omitempty"`
	// ErrorMessage 是失败错误信息。
	ErrorMessage string `json:"errorMessage,omitempty"`
	// StartedAt 是开始执行时间。
	StartedAt string `json:"startedAt,omitempty"`
	// FinishedAt 是结束时间。
	FinishedAt string `json:"finishedAt,omitempty"`
	// CreatedAt 是创建时间。
	CreatedAt string `json:"createdAt"`
	// UpdatedAt 是更新时间。
	UpdatedAt string `json:"updatedAt"`
}

// MessageRecord 是前端可直接渲染的 normalized message。
type MessageRecord struct {
	// ID 是稳定消息 ID。
	ID string `json:"id"`
	// SessionID 是所属会话。
	SessionID string `json:"-"`
	// RunID 是产生该消息的 run。
	RunID string `json:"-"`
	// Role 是 user、assistant 或 tool。
	Role string `json:"role"`
	// Content 是 OpenAI/AG-UI 兼容消息内容。
	Content any `json:"content"`
	// ToolCallID 是 tool message 对应的工具调用 ID。
	ToolCallID string `json:"toolCallId,omitempty"`
	// ToolCalls 是 assistant message 上的工具调用列表。
	ToolCalls []map[string]any `json:"toolCalls,omitempty"`
	// CreatedAt 是消息创建时间。
	CreatedAt string `json:"createdAt"`
	// Ordinal 是会话内顺序号。
	Ordinal int64 `json:"-"`
}

// AGUIEventRecord 保存原始 AG-UI 事件，便于后续 replay/debug。
type AGUIEventRecord struct {
	// ID 是事件记录 ID。
	ID string `json:"id"`
	// SessionID 是所属会话。
	SessionID string `json:"sessionId"`
	// RunID 是所属 run。
	RunID string `json:"runId"`
	// EventType 是 AG-UI event type。
	EventType string `json:"eventType"`
	// EventJSON 是完整事件 JSON。
	EventJSON string `json:"eventJson"`
	// CreatedAt 是事件时间。
	CreatedAt string `json:"createdAt"`
	// Ordinal 是 run 内事件顺序号。
	Ordinal int64 `json:"ordinal"`
}

// Store 保存 MeshWiki daemon 的会话、运行、消息和事件索引。
type Store struct {
	// db 是 SQLite 连接。
	db *sql.DB
}

// DefaultDBPath 返回 daemon 默认 SQLite 路径。
func DefaultDBPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".wikimesh", "daemon", "state.db"), nil
}

// OpenStore 打开并初始化 SQLite store。
func OpenStore(path string) (*Store, error) {
	if path == "" {
		// 调用方可通过 daemon start --db 或 Options.DBPath 覆盖默认路径。
		var err error
		path, err = DefaultDBPath()
		if err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.ensureSchema(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close 关闭底层数据库连接。
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) ensureSchema(ctx context.Context) error {
	// 表结构按 MeshWiki service P0 契约设计；旧字段迁移保持向后兼容。
	if _, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS sessions (
	id TEXT PRIMARY KEY,
	agent_id TEXT NOT NULL DEFAULT 'codex',
	backend_thread_id TEXT,
	title TEXT,
	cwd TEXT,
	project TEXT,
	last_run_id TEXT,
	status TEXT NOT NULL,
	metadata_json TEXT,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_agent_id ON sessions(agent_id);
CREATE INDEX IF NOT EXISTS idx_sessions_backend_thread_id ON sessions(backend_thread_id);
CREATE INDEX IF NOT EXISTS idx_sessions_updated_at ON sessions(updated_at);

CREATE TABLE IF NOT EXISTS runs (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL,
	agent_id TEXT NOT NULL,
	status TEXT NOT NULL,
	error_code TEXT,
	error_message TEXT,
	started_at TEXT,
	finished_at TEXT,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_runs_session_id ON runs(session_id);
CREATE INDEX IF NOT EXISTS idx_runs_updated_at ON runs(updated_at);

CREATE TABLE IF NOT EXISTS messages (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL,
	run_id TEXT,
	role TEXT NOT NULL,
	content_json TEXT NOT NULL,
	tool_call_id TEXT,
	tool_calls_json TEXT,
	created_at TEXT NOT NULL,
	ordinal INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_messages_session_ordinal ON messages(session_id, ordinal);

CREATE TABLE IF NOT EXISTS agui_events (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL,
	run_id TEXT NOT NULL,
	event_type TEXT NOT NULL,
	event_json TEXT NOT NULL,
	created_at TEXT NOT NULL,
	ordinal INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_agui_events_session_ordinal ON agui_events(session_id, ordinal);
CREATE INDEX IF NOT EXISTS idx_agui_events_run_ordinal ON agui_events(run_id, ordinal);
`); err != nil {
		return err
	}
	return s.ensureSessionColumns(ctx)
}

func (s *Store) ensureSessionColumns(ctx context.Context) error {
	columns, err := s.tableColumns(ctx, "sessions")
	if err != nil {
		return err
	}
	add := func(name, ddl string) error {
		if columns[name] {
			return nil
		}
		_, err := s.db.ExecContext(ctx, "ALTER TABLE sessions ADD COLUMN "+ddl)
		return err
	}
	if err := add("agent_id", "agent_id TEXT NOT NULL DEFAULT 'codex'"); err != nil {
		return err
	}
	if err := add("backend_thread_id", "backend_thread_id TEXT"); err != nil {
		return err
	}
	if err := add("metadata_json", "metadata_json TEXT"); err != nil {
		return err
	}
	if columns["codex_thread_id"] {
		_, err := s.db.ExecContext(ctx, `UPDATE sessions SET backend_thread_id = COALESCE(backend_thread_id, codex_thread_id) WHERE codex_thread_id IS NOT NULL`)
		return err
	}
	return nil
}

func (s *Store) tableColumns(ctx context.Context, table string) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return nil, err
		}
		columns[name] = true
	}
	return columns, rows.Err()
}

// CreateSession 创建前端会话；若 ID 已存在，幂等返回旧记录。
func (s *Store) CreateSession(ctx context.Context, rec SessionRecord) (SessionRecord, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if rec.ID == "" {
		rec.ID = newID("sess")
	}
	if existing, err := s.GetSession(ctx, rec.ID); err == nil {
		return existing, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return SessionRecord{}, err
	}
	if rec.AgentID == "" {
		rec.AgentID = "codex"
	}
	if strings.TrimSpace(rec.Title) == "" {
		rec.Title = "新对话"
	}
	if rec.Status == "" {
		rec.Status = "idle"
	}
	if rec.CreatedAt == "" {
		rec.CreatedAt = now
	}
	rec.UpdatedAt = now
	metadataJSON, err := marshalOptionalJSON(rec.Metadata)
	if err != nil {
		return SessionRecord{}, err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO sessions (id, agent_id, backend_thread_id, title, cwd, project, last_run_id, status, metadata_json, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, rec.ID, rec.AgentID, rec.BackendThreadID, rec.Title, rec.Cwd, rec.Project, rec.LastRunID, rec.Status, metadataJSON, rec.CreatedAt, rec.UpdatedAt)
	return rec, err
}

// GetSession 读取指定会话。
func (s *Store) GetSession(ctx context.Context, id string) (SessionRecord, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, agent_id, backend_thread_id, title, cwd, project, last_run_id, status, metadata_json, created_at, updated_at
FROM sessions WHERE id = ?
`, id)
	var rec SessionRecord
	var metadataJSON sql.NullString
	err := row.Scan(&rec.ID, &rec.AgentID, &rec.BackendThreadID, &rec.Title, &rec.Cwd, &rec.Project, &rec.LastRunID, &rec.Status, &metadataJSON, &rec.CreatedAt, &rec.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return SessionRecord{}, err
	}
	if err != nil {
		return SessionRecord{}, err
	}
	rec.Metadata = unmarshalMap(metadataJSON.String)
	return rec, err
}

// ListSessions 按更新时间倒序返回本地 session 映射。
func (s *Store) ListSessions(ctx context.Context, agentID string, limit int) ([]SessionRecord, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	query := `SELECT id, agent_id, backend_thread_id, title, cwd, project, last_run_id, status, metadata_json, created_at, updated_at FROM sessions`
	var args []any
	if strings.TrimSpace(agentID) != "" {
		query += ` WHERE agent_id = ?`
		args = append(args, agentID)
	}
	query += ` ORDER BY updated_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SessionRecord
	for rows.Next() {
		var rec SessionRecord
		var metadataJSON sql.NullString
		if err := rows.Scan(&rec.ID, &rec.AgentID, &rec.BackendThreadID, &rec.Title, &rec.Cwd, &rec.Project, &rec.LastRunID, &rec.Status, &metadataJSON, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
			return nil, err
		}
		rec.Metadata = unmarshalMap(metadataJSON.String)
		out = append(out, rec)
	}
	return out, rows.Err()
}

// UpdateSessionRun 更新会话的 backend thread、最近 run 与状态。
func (s *Store) UpdateSessionRun(ctx context.Context, id string, backendThreadID string, runID string, status string) error {
	if status == "" {
		status = "idle"
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	// 空 backendThreadID/runID 表示本次不更新该字段，避免失败路径覆盖已有映射。
	res, err := s.db.ExecContext(ctx, `
UPDATE sessions
SET backend_thread_id = COALESCE(NULLIF(?, ''), backend_thread_id),
	last_run_id = COALESCE(NULLIF(?, ''), last_run_id),
	status = ?,
	updated_at = ?
WHERE id = ?
`, backendThreadID, runID, status, now, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("session %q not found", id)
	}
	return nil
}

// UpdateSession 更新会话标题和 metadata。
func (s *Store) UpdateSession(ctx context.Context, id string, title *string, metadata map[string]any) (SessionRecord, error) {
	rec, err := s.GetSession(ctx, id)
	if err != nil {
		return SessionRecord{}, err
	}
	if title != nil {
		rec.Title = truncateTitle(*title)
		if rec.Title == "" {
			rec.Title = defaultSessionTitle
		}
	}
	if metadata != nil {
		rec.Metadata = metadata
	}
	rec.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	metadataJSON, err := marshalOptionalJSON(rec.Metadata)
	if err != nil {
		return SessionRecord{}, err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE sessions SET title = ?, metadata_json = ?, updated_at = ? WHERE id = ?`, rec.Title, metadataJSON, rec.UpdatedAt, id)
	if err != nil {
		return SessionRecord{}, err
	}
	return rec, nil
}

// DeleteSession 删除 MeshWiki session 映射，不删除 Codex 原生 thread。
func (s *Store) DeleteSession(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// CreateRun 新建 running run 记录。
func (s *Store) CreateRun(ctx context.Context, run RunRecord) (RunRecord, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if run.ID == "" {
		run.ID = newID("run")
	}
	if run.Status == "" {
		run.Status = "running"
	}
	if run.AgentID == "" {
		run.AgentID = "codex"
	}
	run.CreatedAt = now
	run.UpdatedAt = now
	if run.StartedAt == "" {
		run.StartedAt = now
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO runs (id, session_id, agent_id, status, error_code, error_message, started_at, finished_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, run.ID, run.SessionID, run.AgentID, run.Status, run.ErrorCode, run.ErrorMessage, run.StartedAt, run.FinishedAt, run.CreatedAt, run.UpdatedAt)
	return run, err
}

// FinishRun 结束 run 并记录状态和错误。
func (s *Store) FinishRun(ctx context.Context, id string, status string, errorCode string, errorMessage string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
UPDATE runs SET status = ?, error_code = ?, error_message = ?, finished_at = ?, updated_at = ? WHERE id = ?
`, status, errorCode, errorMessage, now, now, id)
	return err
}

// RunningRunForSession 返回同一 session 的 running run。
func (s *Store) RunningRunForSession(ctx context.Context, sessionID string) (RunRecord, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, session_id, agent_id, status, error_code, error_message, started_at, finished_at, created_at, updated_at
FROM runs WHERE session_id = ? AND status = 'running' ORDER BY updated_at DESC LIMIT 1
`, sessionID)
	var run RunRecord
	err := row.Scan(&run.ID, &run.SessionID, &run.AgentID, &run.Status, &run.ErrorCode, &run.ErrorMessage, &run.StartedAt, &run.FinishedAt, &run.CreatedAt, &run.UpdatedAt)
	return run, err
}

// SaveMessage 保存 normalized message。
func (s *Store) SaveMessage(ctx context.Context, msg MessageRecord) (MessageRecord, error) {
	if msg.ID == "" {
		msg.ID = newID("msg")
	}
	if msg.CreatedAt == "" {
		msg.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if msg.Ordinal == 0 {
		next, err := s.nextOrdinal(ctx, "messages", "session_id", msg.SessionID)
		if err != nil {
			return MessageRecord{}, err
		}
		msg.Ordinal = next
	}
	contentJSON, err := json.Marshal(msg.Content)
	if err != nil {
		return MessageRecord{}, err
	}
	toolCallsJSON, err := marshalOptionalJSON(msg.ToolCalls)
	if err != nil {
		return MessageRecord{}, err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT OR REPLACE INTO messages (id, session_id, run_id, role, content_json, tool_call_id, tool_calls_json, created_at, ordinal)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
`, msg.ID, msg.SessionID, msg.RunID, msg.Role, string(contentJSON), msg.ToolCallID, toolCallsJSON, msg.CreatedAt, msg.Ordinal)
	return msg, err
}

// ListMessages 返回前端可直接渲染的历史消息。
func (s *Store) ListMessages(ctx context.Context, sessionID string) ([]MessageRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, session_id, run_id, role, content_json, tool_call_id, tool_calls_json, created_at, ordinal
FROM messages WHERE session_id = ? ORDER BY ordinal ASC
`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MessageRecord
	for rows.Next() {
		var msg MessageRecord
		var contentJSON string
		var toolCallsJSON sql.NullString
		if err := rows.Scan(&msg.ID, &msg.SessionID, &msg.RunID, &msg.Role, &contentJSON, &msg.ToolCallID, &toolCallsJSON, &msg.CreatedAt, &msg.Ordinal); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(contentJSON), &msg.Content); err != nil {
			msg.Content = ""
		}
		if toolCallsJSON.Valid && toolCallsJSON.String != "" {
			_ = json.Unmarshal([]byte(toolCallsJSON.String), &msg.ToolCalls)
		}
		out = append(out, msg)
	}
	return out, rows.Err()
}

// SaveAGUIEvent 保存 AG-UI 原始事件。
func (s *Store) SaveAGUIEvent(ctx context.Context, event AGUIEventRecord) (AGUIEventRecord, error) {
	if event.ID == "" {
		event.ID = newID("evt")
	}
	if event.CreatedAt == "" {
		event.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if event.Ordinal == 0 {
		next, err := s.nextOrdinal(ctx, "agui_events", "run_id", event.RunID)
		if err != nil {
			return AGUIEventRecord{}, err
		}
		event.Ordinal = next
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO agui_events (id, session_id, run_id, event_type, event_json, created_at, ordinal)
VALUES (?, ?, ?, ?, ?, ?, ?)
`, event.ID, event.SessionID, event.RunID, event.EventType, event.EventJSON, event.CreatedAt, event.Ordinal)
	return event, err
}

func (s *Store) nextOrdinal(ctx context.Context, table string, column string, value string) (int64, error) {
	query := fmt.Sprintf("SELECT COALESCE(MAX(ordinal), 0) + 1 FROM %s WHERE %s = ?", table, column)
	var next int64
	err := s.db.QueryRowContext(ctx, query, value).Scan(&next)
	return next, err
}

func marshalOptionalJSON(v any) (string, error) {
	if v == nil {
		return "", nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func unmarshalMap(raw string) map[string]any {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func truncateTitle(title string) string {
	title = strings.TrimSpace(title)
	runes := []rune(title)
	if len(runes) > maxSessionTitleRunes {
		return string(runes[:maxSessionTitleRunes])
	}
	return title
}
