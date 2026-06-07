package embed

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLlamaCppEmbedderUsesYzmaBackendWhenCommandIsNotConfigured(t *testing.T) {
	modelPath := filepath.Join(t.TempDir(), "model.gguf")
	embedder, err := NewLlamaCppEmbedder(LlamaCppConfig{
		ModelPath: modelPath,
	})
	if err != nil {
		t.Fatalf("NewLlamaCppEmbedder: %v", err)
	}
	if got := embedder.Name(); got != "local/"+modelPath {
		t.Fatalf("Name = %q, want local model path", got)
	}

	_, err = embedder.Embed("local gguf text")
	if err == nil {
		t.Fatalf("Embed error = nil, want unavailable yzma backend error")
	}
	msg := err.Error()
	for _, want := range []string{
		"local embedding yzma backend",
		".wikimesh/lib",
		"model lib install",
		"embedding.command",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("Embed error = %q, want %q", msg, want)
		}
	}
}
