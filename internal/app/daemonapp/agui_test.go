package daemonapp

import (
	"path/filepath"
	"testing"

	"github.com/JieWaZi/wikimesh/pkg/agent"
)

func TestAGUIAdapterTextLifecycle(t *testing.T) {
	store := newTestStore(t)
	var events []map[string]any
	adapter := newAGUIAdapter("sess_1", "run_1", store, func(event map[string]any) error {
		events = append(events, event)
		return nil
	})
	if err := adapter.HandleMessage(agent.Message{Type: agent.MessageText, Content: "hello"}); err != nil {
		t.Fatalf("HandleMessage text: %v", err)
	}
	if err := adapter.HandleMessage(agent.Message{Type: agent.MessageText, Content: " world"}); err != nil {
		t.Fatalf("HandleMessage text 2: %v", err)
	}
	if err := adapter.CloseText(); err != nil {
		t.Fatalf("CloseText: %v", err)
	}
	assertEventTypes(t, events, []string{"TEXT_MESSAGE_START", "TEXT_MESSAGE_CONTENT", "TEXT_MESSAGE_CONTENT", "TEXT_MESSAGE_END"})
	messages, err := store.ListMessages(t.Context(), "sess_1")
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(messages) != 1 || messages[0].Role != "assistant" || messages[0].Content != "hello world" {
		t.Fatalf("messages = %#v", messages)
	}
}

func TestAGUIAdapterToolClosesText(t *testing.T) {
	store := newTestStore(t)
	var events []map[string]any
	adapter := newAGUIAdapter("sess_1", "run_1", store, func(event map[string]any) error {
		events = append(events, event)
		return nil
	})
	if err := adapter.HandleMessage(agent.Message{Type: agent.MessageText, Content: "before"}); err != nil {
		t.Fatalf("HandleMessage text: %v", err)
	}
	if err := adapter.HandleMessage(agent.Message{Type: agent.MessageToolUse, Tool: "exec_command", CallID: "call_1", Input: map[string]any{"command": "ls"}}); err != nil {
		t.Fatalf("HandleMessage tool use: %v", err)
	}
	if err := adapter.HandleMessage(agent.Message{Type: agent.MessageToolResult, Tool: "exec_command", CallID: "call_1", Output: "ok"}); err != nil {
		t.Fatalf("HandleMessage tool result: %v", err)
	}
	assertEventTypes(t, events, []string{
		"TEXT_MESSAGE_START",
		"TEXT_MESSAGE_CONTENT",
		"TEXT_MESSAGE_END",
		"TOOL_CALL_START",
		"TOOL_CALL_ARGS",
		"TOOL_CALL_END",
		"TOOL_CALL_RESULT",
	})
	if events[5]["toolCallId"] != "call_1" || events[6]["content"] != "ok" {
		t.Fatalf("tool events = %#v", events[5:])
	}
}

func assertEventTypes(t *testing.T, events []map[string]any, want []string) {
	t.Helper()
	if len(events) != len(want) {
		t.Fatalf("event count = %d, want %d: %#v", len(events), len(want), events)
	}
	for i, event := range events {
		if event["type"] != want[i] {
			t.Fatalf("event[%d] type = %v, want %s", i, event["type"], want[i])
		}
	}
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := OpenStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.CreateSession(t.Context(), SessionRecord{ID: "sess_1"}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	return store
}
