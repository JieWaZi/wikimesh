package qmd_test

import (
	"context"
	"strings"
	"testing"

	qmd "github.com/JieWaZi/wikimesh/pkg/qmd"
)

func TestLocalQueryModelsDefaultToYzmaBackendWhenCommandIsNotConfigured(t *testing.T) {
	t.Setenv("YZMA_LIB", "")
	t.Setenv("WIKIMESH_YZMA_LIB", "")
	t.Chdir(t.TempDir())
	cfg := qmd.LlamaCppTextModelConfig{
		Provider: "local",
		Model:    ".wikimesh/models/model.gguf",
	}

	expander := qmd.NewLlamaCppQueryExpander(cfg)
	if expander == nil {
		t.Fatalf("QueryExpander is nil, want local yzma-backed expander")
	}
	if _, err := expander.Expand(context.Background(), "alpha"); err == nil || !strings.Contains(err.Error(), "local text yzma backend") || !strings.Contains(err.Error(), "model lib install") {
		t.Fatalf("Expand error = %v, want unavailable yzma backend error", err)
	}

	reranker := qmd.NewLlamaCppQueryReranker(cfg)
	if reranker == nil {
		t.Fatalf("QueryReranker is nil, want local yzma-backed reranker")
	}
	if _, err := reranker.Rerank(context.Background(), "alpha", []qmd.QueryRerankDocument{{ID: "1", Text: "alpha document"}}); err == nil || !strings.Contains(err.Error(), "local text yzma backend") || !strings.Contains(err.Error(), "model lib install") {
		t.Fatalf("Rerank error = %v, want unavailable yzma backend error", err)
	}
}
