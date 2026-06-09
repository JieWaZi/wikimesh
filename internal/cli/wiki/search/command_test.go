package searchcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JieWaZi/wikimesh/pkg/qmd"

	"github.com/JieWaZi/wikimesh/internal/app/wikiapp"
)

func TestRunWikiSearchUsesProjectLocalSource(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := newSearchProjectRoot(t)
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
	if err := runWikiSearch(context.Background(), &out, "/missing-root", "sample", "index", []string{"sample"}); err != nil {
		t.Fatalf("runWikiSearch error = %v", err)
	}
	if !strings.Contains(out.String(), "|slug|type|description|\n|sample-topic|topic|sample topic|") {
		t.Fatalf("output = %q, want project index row", out.String())
	}
}

func TestRunWikiSearchIndexMatchesIndependentQueries(t *testing.T) {
	root := newSearchProjectRoot(t)

	var out bytes.Buffer
	if err := runWikiSearch(context.Background(), &out, root, "", "index", []string{"missing", "sample"}); err != nil {
		t.Fatalf("runWikiSearch error = %v", err)
	}
	if !strings.Contains(out.String(), "|slug|type|description|\n|sample-topic|topic|sample topic|") {
		t.Fatalf("output = %q, want row matching one independent query", out.String())
	}
}

func TestRunWikiSearchGlossaryOutputsSlugFirst(t *testing.T) {
	root := newSearchProjectRoot(t)

	var out bytes.Buffer
	if err := runWikiSearch(context.Background(), &out, root, "", "glossary", []string{"sample"}); err != nil {
		t.Fatalf("runWikiSearch error = %v", err)
	}
	if !strings.Contains(out.String(), "|slug|glossary|type|description|\n|sample-topic|样例术语|topic|sample glossary|") {
		t.Fatalf("output = %q, want glossary row with slug first", out.String())
	}
}

func TestRunWikiSearchTopicUsesIndependentQMDQueries(t *testing.T) {
	ctx := context.Background()
	root := newSearchProjectRoot(t)
	writeSearchTopic(t, root, "root-hint.md", "# Root Hint\n\nroot hint runbook.\n")
	writeSearchTopic(t, root, "dns.md", "# DNS Runbook\n\nroot hint operational steps.\n")
	writeSearchQMDIndex(t, ctx, root)

	var out bytes.Buffer
	if err := runWikiSearch(ctx, &out, root, "", "topic", []string{"root hint", "operational"}); err != nil {
		t.Fatalf("runWikiSearch error = %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) < 2 || lines[0] != "|slug|title|score|" || !strings.Contains(lines[1], "|dns|DNS Runbook|") || strings.Contains(lines[1], "dns.md") {
		t.Fatalf("output = %q, want dns topic first without file column", out.String())
	}
}

func newSearchProjectRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "wiki"), 0o755); err != nil {
		t.Fatalf("MkdirAll wiki error = %v", err)
	}
	index := "# Wiki Index\n\n| type | description | slug |\n|---|---|---|\n| topic | sample topic | sample-topic |\n"
	if err := os.WriteFile(filepath.Join(root, "wiki/index.md"), []byte(index), 0o644); err != nil {
		t.Fatalf("WriteFile index error = %v", err)
	}
	glossary := "# Glossary\n\n| glossary | type | description | slug |\n|---|---|---|---|\n| 样例术语 | topic | sample glossary | sample-topic |\n"
	if err := os.WriteFile(filepath.Join(root, "wiki/glossary.md"), []byte(glossary), 0o644); err != nil {
		t.Fatalf("WriteFile glossary error = %v", err)
	}
	return root
}

func writeSearchTopic(t *testing.T, root, name, body string) {
	t.Helper()
	path := filepath.Join(root, "wiki", "topics", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll topic dir error = %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile topic error = %v", err)
	}
}

func writeSearchQMDIndex(t *testing.T, ctx context.Context, root string) {
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
