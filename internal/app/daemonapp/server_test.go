package daemonapp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestAgentsEndpoint(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/agents", nil)
	rr := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Agents []struct {
			ID string `json:"id"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Agents) != 1 || resp.Agents[0].ID != "codex" {
		t.Fatalf("agents = %#v", resp.Agents)
	}
}

func TestSessionCRUDAndMessages(t *testing.T) {
	srv := newTestServer(t)
	body := bytes.NewBufferString(`{"id":"sess_test","agentId":"codex","title":"新对话","metadata":{"pinned":true}}`)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/v1/sessions", body))
	if rr.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", rr.Code, rr.Body.String())
	}

	if _, err := srv.store.SaveMessage(t.Context(), MessageRecord{ID: "msg_1", SessionID: "sess_test", Role: "user", Content: "hello"}); err != nil {
		t.Fatalf("SaveMessage: %v", err)
	}
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/sessions/sess_test", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("get status = %d body=%s", rr.Code, rr.Body.String())
	}
	var detail struct {
		Messages []MessageRecord `json:"messages"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &detail); err != nil {
		t.Fatalf("unmarshal detail: %v", err)
	}
	if len(detail.Messages) != 1 || detail.Messages[0].Content != "hello" {
		t.Fatalf("messages = %#v", detail.Messages)
	}

	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPatch, "/v1/sessions/sess_test", bytes.NewBufferString(`{"title":"新的标题"}`)))
	if rr.Code != http.StatusOK {
		t.Fatalf("patch status = %d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodDelete, "/v1/sessions/sess_test", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("delete status = %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestJSONErrorFormat(t *testing.T) {
	srv := newTestServer(t)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/sessions/missing", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var resp apiError
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if resp.Error.Code != errorSessionNotFound {
		t.Fatalf("error code = %q", resp.Error.Code)
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	srv, err := NewServer(Options{DBPath: filepath.Join(t.TempDir(), "state.db")})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	return srv
}
