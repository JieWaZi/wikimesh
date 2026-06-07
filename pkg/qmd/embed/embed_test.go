package embed

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestAPIEmbedderPostsOpenAICompatibleEmbeddingsRequest(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotBody struct {
		Model string `json:"model"`
		Input string `json:"input"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2,0.3]}]}`))
	}))
	defer server.Close()

	embedder := &APIEmbedder{
		provider: "api",
		model:    "test-embedding",
		apiKey:   "test-key",
		baseURL:  server.URL,
		client:   *server.Client(),
	}

	vec, err := embedder.Embed("hello embeddings")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if gotPath != "/embeddings" {
		t.Fatalf("path = %q, want /embeddings", gotPath)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("Authorization = %q, want bearer key", gotAuth)
	}
	if gotBody.Model != "test-embedding" || gotBody.Input != "hello embeddings" {
		t.Fatalf("body = %#v, want model/input", gotBody)
	}
	if len(vec) != 3 || embedder.Dimensions() != 3 {
		t.Fatalf("vec len=%d dims=%d, want auto-detected 3d", len(vec), embedder.Dimensions())
	}
}

func TestOllamaEmbedderUsesConfiguredBaseURL(t *testing.T) {
	var gotPath string
	var gotBody struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"embedding":[1,0]}`))
	}))
	defer server.Close()

	embedder := &OllamaEmbedder{
		model:   "nomic-embed-text",
		baseURL: server.URL,
		client:  *server.Client(),
	}

	vec, err := embedder.Embed("local text")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if gotPath != "/api/embeddings" {
		t.Fatalf("path = %q, want /api/embeddings", gotPath)
	}
	if gotBody.Model != "nomic-embed-text" || gotBody.Prompt != "local text" {
		t.Fatalf("body = %#v, want model/prompt", gotBody)
	}
	if len(vec) != 2 || embedder.Dimensions() != 2 {
		t.Fatalf("vec len=%d dims=%d, want auto-detected 2d", len(vec), embedder.Dimensions())
	}
}

func TestLlamaCppEmbedderExecutesCommandAndParsesEmbedding(t *testing.T) {
	dir := t.TempDir()
	command := filepath.Join(dir, "llama-embedding")
	if err := os.WriteFile(command, []byte(`#!/bin/sh
printf '{"data":[{"embedding":[0.25,0.75,1.25]}]}'
`), 0o755); err != nil {
		t.Fatalf("write command: %v", err)
	}

	embedder, err := NewLlamaCppEmbedder(LlamaCppConfig{
		Command:   command,
		ModelPath: filepath.Join(dir, "model.gguf"),
	})
	if err != nil {
		t.Fatalf("NewLlamaCppEmbedder: %v", err)
	}

	vec, err := embedder.Embed("local gguf text")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if got := embedder.Name(); got != "local/"+filepath.Join(dir, "model.gguf") {
		t.Fatalf("Name = %q, want model path", got)
	}
	if len(vec) != 3 || embedder.Dimensions() != 3 {
		t.Fatalf("vec len=%d dims=%d, want auto-detected 3d", len(vec), embedder.Dimensions())
	}
}
