package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
)

type codexClient struct {
	// cfg 保存日志等运行时配置。
	cfg Config
	// stdin 是写给 codex app-server 的 JSON-RPC 请求流。
	stdin io.Writer
	// mu 保护请求 ID 和 pending 表。
	mu sync.Mutex
	// nextID 是 JSON-RPC 自增请求 ID。
	nextID int
	// pending 保存等待响应的 JSON-RPC 请求。
	pending map[int]*pendingRPC
	// threadID 是当前连接关注的 Codex thread。
	threadID string
	// turnID 是当前执行中的 Codex turn。
	turnID string
	// onMessage 把 Codex 通知转成 daemon 中间事件。
	onMessage func(Message)
	// onTurnDone 在 turn/completed 或等价终止信号出现时触发。
	onTurnDone func(aborted bool)

	// turnErrorMu 保护 turnError，避免通知读取协程和生命周期协程竞争。
	turnErrorMu sync.Mutex
	// turnError 保存 Codex 终止通知中的失败原因。
	turnError string
}

type pendingRPC struct {
	// method 仅用于错误信息，帮助定位失败的 Codex 方法。
	method string
	// ch 接收对应 id 的 JSON-RPC 响应。
	ch chan rpcResult
}

type rpcResult struct {
	// result 是 JSON-RPC 成功响应体。
	result json.RawMessage
	// err 是 JSON-RPC error 或传输错误。
	err error
}

func (c *codexClient) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	pr := &pendingRPC{method: method, ch: make(chan rpcResult, 1)}
	c.pending[id] = pr
	c.mu.Unlock()

	data, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	})
	if err != nil {
		return nil, err
	}
	// Codex stdio transport 是 JSONL：每条 JSON-RPC 消息以换行分隔。
	data = append(data, '\n')
	if _, err := c.stdin.Write(data); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("write %s: %w", method, err)
	}

	// 等待匹配 id 的响应；如果上层取消，必须移除 pending 防止泄漏。
	select {
	case res := <-pr.ch:
		return res.result, res.err
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	}
}

func (c *codexClient) notify(method string, params any) {
	data, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	})
	data = append(data, '\n')
	_, _ = c.stdin.Write(data)
}

func (c *codexClient) respond(id int, result any) {
	data, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	})
	data = append(data, '\n')
	_, _ = c.stdin.Write(data)
}

func (c *codexClient) respondError(id int, code int, message string) {
	data, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
	data = append(data, '\n')
	_, _ = c.stdin.Write(data)
}

func (c *codexClient) closeAllPending(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, pr := range c.pending {
		pr.ch <- rpcResult{err: err}
		delete(c.pending, id)
	}
}

func (c *codexClient) handleLine(line string) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return
	}
	// 有 id 的消息可能是请求响应，也可能是 Codex 发来的 server request。
	if _, hasID := raw["id"]; hasID {
		if _, hasResult := raw["result"]; hasResult {
			c.handleResponse(raw)
			return
		}
		if _, hasError := raw["error"]; hasError {
			c.handleResponse(raw)
			return
		}
		if _, hasMethod := raw["method"]; hasMethod {
			c.handleServerRequest(raw)
			return
		}
	}
	if _, hasMethod := raw["method"]; hasMethod {
		c.handleNotification(raw)
	}
}

func (c *codexClient) handleResponse(raw map[string]json.RawMessage) {
	var id int
	if err := json.Unmarshal(raw["id"], &id); err != nil {
		return
	}
	c.mu.Lock()
	pr, ok := c.pending[id]
	if ok {
		delete(c.pending, id)
	}
	c.mu.Unlock()
	if !ok {
		return
	}
	if errData, ok := raw["error"]; ok {
		var rpcErr struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(errData, &rpcErr)
		pr.ch <- rpcResult{err: fmt.Errorf("%s: %s (code=%d)", pr.method, rpcErr.Message, rpcErr.Code)}
		return
	}
	pr.ch <- rpcResult{result: raw["result"]}
}

func (c *codexClient) handleServerRequest(raw map[string]json.RawMessage) {
	var id int
	_ = json.Unmarshal(raw["id"], &id)
	var method string
	_ = json.Unmarshal(raw["method"], &method)
	switch method {
	case "item/commandExecution/requestApproval", "execCommandApproval":
		// MVP daemon 先采用 Multica 的无人值守策略：Codex 请求审批时自动接受。
		c.respond(id, map[string]any{"decision": "accept"})
	case "item/fileChange/requestApproval", "applyPatchApproval":
		c.respond(id, map[string]any{"decision": "accept"})
	case "mcpServer/elicitation/request":
		c.respond(id, map[string]any{"action": "accept", "content": nil, "_meta": nil})
	default:
		c.respondError(id, -32601, fmt.Sprintf("unhandled server request: %s", method))
	}
}

func (c *codexClient) handleNotification(raw map[string]json.RawMessage) {
	var method string
	_ = json.Unmarshal(raw["method"], &method)
	var params map[string]any
	if p, ok := raw["params"]; ok {
		_ = json.Unmarshal(p, &params)
	}
	rawBytes, _ := json.Marshal(raw)

	// Codex 早期版本使用 codex/event 包一层 msg；新版本直接发 turn/* 和 item/*。
	if method == "codex/event" || strings.HasPrefix(method, "codex/event/") {
		msgData, ok := params["msg"].(map[string]any)
		if ok {
			c.handleLegacyEvent(msgData, rawBytes)
		}
		return
	}
	c.handleRawNotification(method, params, rawBytes)
}

func (c *codexClient) handleLegacyEvent(msg map[string]any, raw json.RawMessage) {
	switch msgType, _ := msg["type"].(string); msgType {
	case "task_started":
		c.emit(Message{Type: MessageStatus, Status: "running", SessionID: c.threadID, TurnID: c.turnID, Raw: raw})
	case "agent_message":
		text, _ := msg["message"].(string)
		if text != "" {
			c.emit(Message{Type: MessageText, Content: text, SessionID: c.threadID, TurnID: c.turnID, Raw: raw})
		}
	case "exec_command_begin":
		callID, _ := msg["call_id"].(string)
		command, _ := msg["command"].(string)
		c.emit(Message{Type: MessageToolUse, Tool: "exec_command", CallID: callID, Input: map[string]any{"command": command}, SessionID: c.threadID, TurnID: c.turnID, Raw: raw})
	case "exec_command_end":
		callID, _ := msg["call_id"].(string)
		output, _ := msg["output"].(string)
		c.emit(Message{Type: MessageToolResult, Tool: "exec_command", CallID: callID, Output: output, SessionID: c.threadID, TurnID: c.turnID, Raw: raw})
	case "patch_apply_begin":
		callID, _ := msg["call_id"].(string)
		c.emit(Message{Type: MessageToolUse, Tool: "patch_apply", CallID: callID, SessionID: c.threadID, TurnID: c.turnID, Raw: raw})
	case "patch_apply_end":
		callID, _ := msg["call_id"].(string)
		c.emit(Message{Type: MessageToolResult, Tool: "patch_apply", CallID: callID, SessionID: c.threadID, TurnID: c.turnID, Raw: raw})
	case "task_complete":
		c.finishTurn(false)
	case "turn_aborted":
		c.finishTurn(true)
	}
}

func (c *codexClient) handleRawNotification(method string, params map[string]any, raw json.RawMessage) {
	if threadID, ok := params["threadId"].(string); ok && c.threadID != "" && threadID != c.threadID {
		// app-server 可能在同一连接上报告其他 thread 的事件，MVP 只关心当前 run。
		return
	}
	switch method {
	case "turn/started":
		if turnID := extractNestedString(params, "turn", "id"); turnID != "" {
			c.turnID = turnID
		}
		c.emit(Message{Type: MessageStatus, Status: "running", SessionID: c.threadID, TurnID: c.turnID, Raw: raw})
	case "turn/completed":
		if turnID := extractNestedString(params, "turn", "id"); turnID != "" {
			c.turnID = turnID
		}
		status := extractNestedString(params, "turn", "status")
		if status == "failed" {
			errMsg := extractNestedString(params, "turn", "error", "message")
			if errMsg == "" {
				errMsg = "codex turn failed"
			}
			c.setTurnError(errMsg)
		}
		c.finishTurn(status == "cancelled" || status == "canceled" || status == "aborted" || status == "interrupted")
	case "error":
		errMsg := extractNestedString(params, "error", "message")
		if errMsg == "" {
			errMsg = extractNestedString(params, "message")
		}
		if errMsg != "" {
			c.setTurnError(errMsg)
			c.emit(Message{Type: MessageError, Content: errMsg, SessionID: c.threadID, TurnID: c.turnID, Raw: raw})
			c.finishTurn(false)
		}
	case "thread/status/changed":
		if status := extractNestedString(params, "status", "type"); status == "idle" && c.turnID != "" {
			c.finishTurn(false)
		}
	default:
		if strings.HasPrefix(method, "item/") {
			c.handleItemNotification(method, params, raw)
		}
	}
}

func (c *codexClient) handleItemNotification(method string, params map[string]any, raw json.RawMessage) {
	item, _ := params["item"].(map[string]any)
	if item == nil {
		return
	}
	itemType, _ := item["type"].(string)
	itemID, _ := item["id"].(string)
	switch {
	case method == "item/started" && itemType == "commandExecution":
		command, _ := item["command"].(string)
		c.emit(Message{Type: MessageToolUse, Tool: "exec_command", CallID: itemID, Input: map[string]any{"command": command}, SessionID: c.threadID, TurnID: c.turnID, Raw: raw})
	case method == "item/completed" && itemType == "commandExecution":
		output, _ := item["aggregatedOutput"].(string)
		c.emit(Message{Type: MessageToolResult, Tool: "exec_command", CallID: itemID, Output: output, SessionID: c.threadID, TurnID: c.turnID, Raw: raw})
	case method == "item/started" && itemType == "fileChange":
		c.emit(Message{Type: MessageToolUse, Tool: "patch_apply", CallID: itemID, SessionID: c.threadID, TurnID: c.turnID, Raw: raw})
	case method == "item/completed" && itemType == "fileChange":
		c.emit(Message{Type: MessageToolResult, Tool: "patch_apply", CallID: itemID, SessionID: c.threadID, TurnID: c.turnID, Raw: raw})
	case method == "item/agentMessage/delta":
		text := extractItemText(item)
		if text != "" {
			c.emit(Message{Type: MessageText, Content: text, SessionID: c.threadID, TurnID: c.turnID, Raw: raw})
		}
	case method == "item/completed" && itemType == "agentMessage":
		text := extractItemText(item)
		if text != "" {
			c.emit(Message{Type: MessageText, Content: text, SessionID: c.threadID, TurnID: c.turnID, Raw: raw})
		}
	}
}

func extractItemText(item map[string]any) string {
	// Codex 不同版本的 agent message 字段名略有差异，这里按宽松顺序取文本。
	for _, key := range []string{"delta", "text", "message", "content"} {
		if s, _ := item[key].(string); s != "" {
			return s
		}
	}
	return ""
}

func (c *codexClient) emit(msg Message) {
	if c.onMessage != nil {
		c.onMessage(msg)
	}
}

func (c *codexClient) finishTurn(aborted bool) {
	if c.onTurnDone == nil {
		return
	}
	c.onTurnDone(aborted)
}

func (c *codexClient) setTurnError(msg string) {
	if msg == "" {
		return
	}
	c.turnErrorMu.Lock()
	defer c.turnErrorMu.Unlock()
	if c.turnError == "" {
		c.turnError = msg
	}
}

func (c *codexClient) getTurnError() string {
	c.turnErrorMu.Lock()
	defer c.turnErrorMu.Unlock()
	return c.turnError
}

func extractNestedString(m map[string]any, keys ...string) string {
	// Codex 通知常见为 map 嵌套结构，使用安全读取避免为每种事件建结构体。
	var current any = m
	for _, key := range keys {
		obj, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = obj[key]
	}
	s, _ := current.(string)
	return s
}
