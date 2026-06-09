package querycmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JieWaZi/wikimesh/internal/app/wikiapp"
	"github.com/JieWaZi/wikimesh/pkg/qmd"
)

func TestQueryCommandJoinsQuestionAndDoesNotExposeStructuredQueries(t *testing.T) {
	ctx := context.Background()
	root := newQueryProjectRoot(t)
	writeQueryDoc(t, root, "topics/user-management.md", "# 用户管理\n\nuser management operational steps.\n")
	writeQueryQMDIndex(t, ctx, root)

	cmd := NewCommand()
	if cmd.Flags().Lookup("queries") != nil {
		t.Fatalf("top-level query must not expose low-level --queries flag")
	}
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"user", "management", "--root", root, "--no-rerank"})
	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatalf("ExecuteContext error = %v", err)
	}
	var result qmd.QueryResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("parse output JSON: %v\n%s", err, out.String())
	}
	if result.Question != "user management" {
		t.Fatalf("Question = %q, want joined positional question", result.Question)
	}
	if len(result.Results) == 0 || result.Results[0].Path != "topics/user-management.md" {
		t.Fatalf("results = %#v, want indexed topic hit", result.Results)
	}
}

func TestRunWikiQueryUsesProjectLocalSourceAndSearchQueries(t *testing.T) {
	ctx := context.Background()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := newQueryProjectRoot(t)
	writeQueryDoc(t, root, "topics/user-management.md", "# 用户管理\n\nuser management runbook.\n")
	writeQueryDoc(t, root, "workflows/operation.md", "# 操作流程\n\noperational recovery path.\n")
	writeQueryQMDIndex(t, ctx, root)

	if err := wikiapp.SaveRepoConfig(wikiapp.RepoConfig{
		ProjectName:  "Sample",
		ProjectSlug:  "sample",
		Language:     "zh",
		ActiveSource: wikiapp.SourceLocal,
		Sources: wikiapp.RepoSources{
			Local: &wikiapp.RepoSource{Type: wikiapp.SourceLocal, Path: root},
		},
	}); err != nil {
		t.Fatalf("SaveRepoConfig error = %v", err)
	}

	var out bytes.Buffer
	err := runWikiQuery(ctx, &out, "/missing-root", "sample", []string{"wiki"}, queryOptions{
		question:      "where is user management handled",
		searchQueries: []string{"operational"},
		limit:         5,
		noRerank:      true,
	})
	if err != nil {
		t.Fatalf("runWikiQuery error = %v", err)
	}
	if !strings.Contains(out.String(), "workflows/operation.md") {
		t.Fatalf("output = %q, want auxiliary search-query to recall workflow doc", out.String())
	}
}

func newQueryProjectRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "wiki"), 0o755); err != nil {
		t.Fatalf("MkdirAll wiki error = %v", err)
	}
	return root
}

func writeQueryDoc(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, "wiki", filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll doc dir error = %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile doc error = %v", err)
	}
}

func writeQueryQMDIndex(t *testing.T, ctx context.Context, root string) {
	t.Helper()
	configPath := filepath.Join(root, ".wikimesh", "qmd.yaml")
	dbPath := filepath.Join(root, ".wikimesh", "wiki.db")
	cfg := qmd.FileConfig{
		DBPath: dbPath,
		Collections: []qmd.Collection{
			{Name: "wiki", Path: filepath.Join(root, "wiki"), Include: []string{"**/*.md"}},
		},
	}
	store, err := qmd.NewStore(cfg.StoreConfig())
	if err != nil {
		t.Fatalf("NewStore error = %v", err)
	}
	defer store.Close()
	for _, collection := range cfg.Collections {
		if err := store.AddCollection(ctx, collection); err != nil {
			t.Fatalf("AddCollection error = %v", err)
		}
		if _, err := store.UpdateCollection(ctx, collection.Name, qmd.UpdateOptions{}); err != nil {
			t.Fatalf("UpdateCollection error = %v", err)
		}
	}
	cfg.DBPath = ".wikimesh/wiki.db"
	cfg.Collections[0].Path = "wiki"
	if err := qmd.SaveConfigFile(configPath, cfg); err != nil {
		t.Fatalf("SaveConfigFile error = %v", err)
	}
}
