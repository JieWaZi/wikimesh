// Package agent 提供面向 coding agent 的最小运行时抽象。
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
)

// Backend 是执行单次 agent turn 的统一接口。
type Backend interface {
	Execute(ctx context.Context, prompt string, opts ExecOptions) (*Session, error)
}

// ExecOptions 描述一次 Codex 执行所需的可选参数。
type ExecOptions struct {
	// Cwd 是 Codex 执行命令和读取仓库上下文的工作目录。
	Cwd string
	// Model 是传给 Codex thread 的模型覆盖；为空时使用 Codex 默认配置。
	Model string
	// SystemPrompt 是传给 Codex 的 developer instructions。
	SystemPrompt string
	// ThreadName 是 Codex 原生 thread 标题，便于 Codex 历史列表展示。
	ThreadName string
	// Timeout 是单轮执行的硬超时；0 表示只随上层 context 取消。
	Timeout time.Duration
	// ResumeSessionID 是 Codex thread.id；非空时优先 thread/resume 续聊。
	ResumeSessionID string
}

// Session 表示一次运行中的 agent 执行。
type Session struct {
	// Messages 流式返回 Codex 通知转换后的中间事件。
	Messages <-chan Message
	// Result 返回本轮执行的最终状态，且只会发送一次。
	Result <-chan Result
}

// MessageType 是 daemon 内部归一化后的 agent 事件类型。
type MessageType string

const (
	MessageText       MessageType = "text"
	MessageToolUse    MessageType = "tool-use"
	MessageToolResult MessageType = "tool-result"
	MessageStatus     MessageType = "status"
	MessageError      MessageType = "error"
)

// Message 是 Codex 通知转换后的中间事件。
type Message struct {
	// Type 是归一化后的事件类型。
	Type MessageType
	// Content 是文本、错误等直接展示内容。
	Content string
	// Tool 是工具名，例如 exec_command 或 patch_apply。
	Tool string
	// CallID 是 Codex 工具调用 ID，用于配对开始和结果事件。
	CallID string
	// Input 是工具调用参数，供 AG-UI TOOL_CALL_ARGS 使用。
	Input map[string]any
	// Output 是工具执行结果文本。
	Output string
	// Status 是运行状态，例如 running。
	Status string
	// SessionID 是 Codex thread.id。
	SessionID string
	// TurnID 是 Codex turn.id。
	TurnID string
	// Raw 保留 Codex 原始通知，便于后续调试和协议兼容。
	Raw json.RawMessage
}

// Result 是一次 agent turn 的最终结果。
type Result struct {
	// Status 是 completed、failed、aborted 或 timeout。
	Status string
	// Output 是本轮聚合后的助手文本输出。
	Output string
	// Error 是失败原因；成功时为空。
	Error string
	// DurationMs 是本轮执行耗时，单位毫秒。
	DurationMs int64
	// SessionID 是本轮使用或创建的 Codex thread.id。
	SessionID string
	// TurnID 是本轮 Codex turn.id。
	TurnID string
}

// Config 配置具体 agent 后端。
type Config struct {
	// ExecutablePath 是 Codex CLI 路径；为空时使用 PATH 中的 codex。
	ExecutablePath string
	// Env 是追加到子进程环境变量中的键值。
	Env map[string]string
	// Logger 是运行时日志输出；为空时使用 slog.Default。
	Logger *slog.Logger
}

// New 创建指定类型的 agent 后端。MVP 仅支持 codex。
func New(agentType string, cfg Config) (Backend, error) {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	switch agentType {
	case "codex":
		return &codexBackend{cfg: cfg}, nil
	default:
		return nil, fmt.Errorf("unknown agent type %q (supported: codex)", agentType)
	}
}
