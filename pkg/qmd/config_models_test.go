package qmd_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	qmd "github.com/JieWaZi/wikimesh/pkg/qmd"
)

func TestFileConfigStoreConfigWiresQueryModels(t *testing.T) {
	dir := t.TempDir()
	expanderCommand := filepath.Join(dir, "expand")
	expanderPrompt := filepath.Join(dir, "expand.prompt")
	if err := os.WriteFile(expanderCommand, []byte(fmt.Sprintf(`#!/bin/sh
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-p" ]; then
    shift
    printf '%%s' "$1" > %q
    break
  fi
  shift
done
printf 'lex: alpha exact\nvec: semantic alpha\nhyde: information about alpha\n'
`, expanderPrompt)), 0o755); err != nil {
		t.Fatalf("write expander command: %v", err)
	}
	rerankerCommand := filepath.Join(dir, "rerank")
	if err := os.WriteFile(rerankerCommand, []byte(`#!/bin/sh
printf '0.87'
`), 0o755); err != nil {
		t.Fatalf("write reranker command: %v", err)
	}

	cfg := qmd.FileConfig{
		Models: qmd.ModelsConfig{
			Generate: "generate.gguf",
			Rerank:   "rerank.gguf",
		},
		QueryExpansion: qmd.LlamaCppTextModelConfig{
			Provider: "local",
			Command:  expanderCommand,
		},
		Reranker: qmd.LlamaCppTextModelConfig{
			Provider: "local",
			Command:  rerankerCommand,
		},
	}
	storeCfg := cfg.StoreConfig()
	if storeCfg.QueryExpander == nil {
		t.Fatalf("QueryExpander is nil, want configured model")
	}
	if storeCfg.QueryReranker == nil {
		t.Fatalf("QueryReranker is nil, want configured model")
	}

	expansions, err := storeCfg.QueryExpander.Expand(context.Background(), "alpha")
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if len(expansions) != 3 || expansions[0].Type != qmd.QueryExpansionLex || expansions[1].Type != qmd.QueryExpansionVec || expansions[2].Type != qmd.QueryExpansionHyDE {
		t.Fatalf("expansions = %#v, want parsed lex/vec/hyde lines", expansions)
	}
	intentExpander, ok := storeCfg.QueryExpander.(qmd.QueryExpanderWithOptions)
	if !ok {
		t.Fatalf("QueryExpander does not implement QueryExpanderWithOptions")
	}
	if _, err := intentExpander.ExpandWithOptions(context.Background(), "alpha", qmd.ExpandQueryOptions{Intent: "user login"}); err != nil {
		t.Fatalf("ExpandWithOptions: %v", err)
	}
	promptBytes, err := os.ReadFile(expanderPrompt)
	if err != nil {
		t.Fatalf("read expander prompt: %v", err)
	}
	if !strings.Contains(string(promptBytes), "Query intent: user login") {
		t.Fatalf("expander prompt = %q, want query intent", string(promptBytes))
	}

	scores, err := storeCfg.QueryReranker.Rerank(context.Background(), "alpha", []qmd.QueryRerankDocument{
		{ID: "doc1", File: "qmd://docs/alpha.md", Text: "alpha text"},
	})
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	if len(scores) != 1 || scores[0].ID != "doc1" || scores[0].Score != 0.87 {
		t.Fatalf("scores = %#v, want configured reranker score for doc1", scores)
	}
}
