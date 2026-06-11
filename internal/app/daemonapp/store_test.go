package daemonapp

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestStoreCreateAndUpdateSession(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	rec, err := store.CreateSession(context.Background(), SessionRecord{
		ID:      "sess_test",
		Title:   "测试会话",
		Cwd:     "/tmp/repo",
		Project: "demo",
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if rec.ID != "sess_test" || rec.Status != "idle" {
		t.Fatalf("unexpected session: %#v", rec)
	}

	if err := store.UpdateSessionRun(context.Background(), rec.ID, "thr_123", "run_1", "running"); err != nil {
		t.Fatalf("UpdateSessionRun: %v", err)
	}
	got, err := store.GetSession(context.Background(), rec.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.BackendThreadID != "thr_123" || got.LastRunID != "run_1" || got.Status != "running" {
		t.Fatalf("updated session = %#v", got)
	}
}

func TestStoreListSessions(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	if _, err := store.CreateSession(context.Background(), SessionRecord{ID: "sess_a"}); err != nil {
		t.Fatalf("CreateSession a: %v", err)
	}
	if _, err := store.CreateSession(context.Background(), SessionRecord{ID: "sess_b"}); err != nil {
		t.Fatalf("CreateSession b: %v", err)
	}
	got, err := store.ListSessions(context.Background(), "", 10)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("session count = %d, want 2", len(got))
	}
}

func TestStoreRunMessageAndEvent(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	if _, err := store.CreateSession(context.Background(), SessionRecord{ID: "sess_test"}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if _, err := store.CreateRun(context.Background(), RunRecord{ID: "run_1", SessionID: "sess_test", AgentID: "codex"}); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := store.SaveMessage(context.Background(), MessageRecord{ID: "msg_1", SessionID: "sess_test", RunID: "run_1", Role: "user", Content: "你好"}); err != nil {
		t.Fatalf("SaveMessage: %v", err)
	}
	messages, err := store.ListMessages(context.Background(), "sess_test")
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(messages) != 1 || messages[0].Content != "你好" {
		t.Fatalf("messages = %#v", messages)
	}
	if _, err := store.SaveAGUIEvent(context.Background(), AGUIEventRecord{SessionID: "sess_test", RunID: "run_1", EventType: "RUN_STARTED", EventJSON: `{"type":"RUN_STARTED"}`}); err != nil {
		t.Fatalf("SaveAGUIEvent: %v", err)
	}
	if err := store.FinishRun(context.Background(), "run_1", "completed", "", ""); err != nil {
		t.Fatalf("FinishRun: %v", err)
	}
}

func TestStoreRunningRunForSession(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	if _, err := store.CreateSession(context.Background(), SessionRecord{ID: "sess_busy"}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if _, err := store.CreateRun(context.Background(), RunRecord{ID: "run_busy", SessionID: "sess_busy", AgentID: "codex"}); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	run, err := store.RunningRunForSession(context.Background(), "sess_busy")
	if err != nil {
		t.Fatalf("RunningRunForSession: %v", err)
	}
	if run.ID != "run_busy" {
		t.Fatalf("running run = %#v", run)
	}
	if err := store.FinishRun(context.Background(), "run_busy", "completed", "", ""); err != nil {
		t.Fatalf("FinishRun: %v", err)
	}
	if _, err := store.RunningRunForSession(context.Background(), "sess_busy"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("RunningRunForSession after finish err = %v, want sql.ErrNoRows", err)
	}
}
