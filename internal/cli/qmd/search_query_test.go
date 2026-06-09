package qmdcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/JieWaZi/wikimesh/pkg/qmd"
)

func TestRunTopLevelSearchUsesIndependentQueries(t *testing.T) {
	ctx := context.Background()
	store := newQMDCommandSearchStore(t)
	defer store.Close()

	var out bytes.Buffer
	err := runTopLevelSearch(ctx, &out, store, []string{"docs"}, []string{"root hint", "operational"}, 5, 0, false, false, false)
	if err != nil {
		t.Fatalf("runTopLevelSearch: %v", err)
	}

	var results []qmd.SearchResult
	if err := json.Unmarshal(out.Bytes(), &results); err != nil {
		t.Fatalf("decode search output: %v", err)
	}
	if len(results) == 0 || results[0].Path != "ops/runbooks/dns.md" {
		t.Fatalf("results = %#v, want document matching both queries first", results)
	}
}

func TestRunTopLevelQueryUsesSearchQueries(t *testing.T) {
	ctx := context.Background()
	store := newQMDCommandSearchStore(t)
	defer store.Close()

	var out bytes.Buffer
	err := runTopLevelQuery(ctx, &out, store, []string{"docs"}, queryCLIOptions{
		question:      "combined retrieval",
		searchQueries: []string{"root hint", "operational"},
		limit:         5,
		noRerank:      true,
	})
	if err != nil {
		t.Fatalf("runTopLevelQuery: %v", err)
	}

	var result qmd.QueryResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("decode query output: %v", err)
	}
	if len(result.Results) == 0 || result.Results[0].Path != "ops/runbooks/dns.md" {
		t.Fatalf("results = %#v, want document matching both queries first", result.Results)
	}
}

func newQMDCommandSearchStore(t *testing.T) *qmd.Store {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(filepath.Join(docs, "ops", "runbooks"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	files := map[string]string{
		"root-hint.md":        "# Root Hint\n\nroot hint runbook.\n",
		"ops/runbooks/dns.md": "# DNS Runbook\n\nroot hint operational steps.\n",
	}
	for name, body := range files {
		path := filepath.Join(docs, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	store, err := qmd.NewStore(qmd.Config{DBPath: filepath.Join(dir, "wiki.db")})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.AddCollection(ctx, qmd.Collection{Name: "docs", Path: docs, Include: []string{"**/*.md"}}); err != nil {
		t.Fatalf("AddCollection: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "docs", qmd.UpdateOptions{}); err != nil {
		t.Fatalf("UpdateCollection: %v", err)
	}
	return store
}
