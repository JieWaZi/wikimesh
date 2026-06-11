package agent

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeStdin struct {
	mu   sync.Mutex
	data []byte
}

func (f *fakeStdin) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data = append(f.data, p...)
	return len(p), nil
}

func (f *fakeStdin) lines() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Split(strings.TrimSpace(string(f.data)), "\n")
}

func newTestCodexClient(t *testing.T) (*codexClient, *fakeStdin, *[]Message) {
	t.Helper()
	fs := &fakeStdin{}
	var messages []Message
	c := &codexClient{
		cfg:     Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))},
		stdin:   fs,
		pending: make(map[int]*pendingRPC),
		onMessage: func(msg Message) {
			messages = append(messages, msg)
		},
		onTurnDone: func(bool) {},
	}
	return c, fs, &messages
}

func TestCodexRequestWritesJSONRPCAndReceivesResult(t *testing.T) {
	c, fs, _ := newTestCodexClient(t)
	done := make(chan json.RawMessage, 1)
	go func() {
		raw, err := c.request(context.Background(), "thread/start", map[string]any{"model": "gpt-test"})
		if err != nil {
			t.Errorf("request returned error: %v", err)
			return
		}
		done <- raw
	}()
	waitForLine(t, fs)
	var req map[string]any
	if err := json.Unmarshal([]byte(fs.lines()[0]), &req); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	if req["method"] != "thread/start" {
		t.Fatalf("method = %v, want thread/start", req["method"])
	}
	c.handleLine(`{"jsonrpc":"2.0","id":1,"result":{"thread":{"id":"thr_1"}}}`)
	select {
	case raw := <-done:
		if got := extractThreadID(raw); got != "thr_1" {
			t.Fatalf("thread id = %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("request did not complete")
	}
}

func TestCodexHandleServerRequestAutoApproves(t *testing.T) {
	c, fs, _ := newTestCodexClient(t)
	c.handleLine(`{"jsonrpc":"2.0","id":10,"method":"item/commandExecution/requestApproval","params":{}}`)
	var resp map[string]any
	if err := json.Unmarshal([]byte(fs.lines()[0]), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	result := resp["result"].(map[string]any)
	if result["decision"] != "accept" {
		t.Fatalf("decision = %v, want accept", result["decision"])
	}
}

func TestCodexLegacyEventMapsMessages(t *testing.T) {
	c, _, messages := newTestCodexClient(t)
	c.threadID = "thr_1"
	c.handleLine(`{"jsonrpc":"2.0","method":"codex/event","params":{"msg":{"type":"agent_message","message":"hello"}}}`)
	if len(*messages) != 1 {
		t.Fatalf("message count = %d", len(*messages))
	}
	if (*messages)[0].Type != MessageText || (*messages)[0].Content != "hello" {
		t.Fatalf("message = %#v", (*messages)[0])
	}
}

func TestCodexRawItemMapsToolAndText(t *testing.T) {
	c, _, messages := newTestCodexClient(t)
	c.threadID = "thr_1"
	c.handleLine(`{"jsonrpc":"2.0","method":"item/started","params":{"threadId":"thr_1","item":{"id":"cmd_1","type":"commandExecution","command":"go test ./..."}}}`)
	c.handleLine(`{"jsonrpc":"2.0","method":"item/agentMessage/delta","params":{"threadId":"thr_1","item":{"id":"msg_1","type":"agentMessage","delta":"done"}}}`)
	if len(*messages) != 2 {
		t.Fatalf("message count = %d", len(*messages))
	}
	if (*messages)[0].Type != MessageToolUse || (*messages)[0].Tool != "exec_command" {
		t.Fatalf("tool message = %#v", (*messages)[0])
	}
	if (*messages)[1].Type != MessageText || (*messages)[1].Content != "done" {
		t.Fatalf("text message = %#v", (*messages)[1])
	}
}

func waitForLine(t *testing.T, fs *fakeStdin) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if len(fs.lines()) > 0 && fs.lines()[0] != "" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out waiting for written line")
}
