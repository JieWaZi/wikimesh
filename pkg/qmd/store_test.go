package qmd_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	qmd "github.com/JieWaZi/wikimesh/pkg/qmd"
	_ "modernc.org/sqlite"
)

type fakeEmbedder struct{}

func (fakeEmbedder) Name() string { return "fake" }

func (fakeEmbedder) Dimensions() int { return 2 }

func (fakeEmbedder) Embed(text string) ([]float32, error) {
	lower := strings.ToLower(text)
	if strings.Contains(lower, "alpha") || strings.Contains(lower, "向量") {
		return []float32{1, 0}, nil
	}
	if strings.Contains(lower, "beta") {
		return []float32{0, 1}, nil
	}
	return []float32{0.2, 0.2}, nil
}

type recordingEmbedder struct {
	name   string
	inputs []string
}

func (e *recordingEmbedder) Name() string {
	if e.name == "" {
		return "recording"
	}
	return e.name
}

func (e *recordingEmbedder) Dimensions() int { return 2 }

func (e *recordingEmbedder) Embed(text string) ([]float32, error) {
	e.inputs = append(e.inputs, text)
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "semantic beta") || strings.Contains(lower, "beta"):
		return []float32{0, 1}, nil
	case strings.Contains(lower, "unrelated"):
		return []float32{-1, 0}, nil
	case strings.Contains(lower, "alpha"):
		return []float32{1, 0}, nil
	default:
		return []float32{0.1, 0}, nil
	}
}

func embedCollectionForTest(t *testing.T, ctx context.Context, store *qmd.Store, collection string) {
	t.Helper()
	if _, err := store.EmbedCollection(ctx, collection, qmd.EmbedOptions{}); err != nil {
		t.Fatalf("EmbedCollection %s: %v", collection, err)
	}
}

type fakeQueryExpander struct {
	items []qmd.QueryExpansion
}

func (e fakeQueryExpander) Expand(_ context.Context, _ string) ([]qmd.QueryExpansion, error) {
	return e.items, nil
}

func TestCollectionUpdateAndSearchIndexesMarkdownAndCJK(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "user-management.md"), []byte("---\ntag: audit\n---\n\n# 用户管理\n\n用户管理探测会检查 operation_field 配置。\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(dir, "wiki.db")
	store, err := qmd.NewStore(qmd.Config{DBPath: dbPath})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	if err := store.AddCollection(ctx, qmd.Collection{Name: "docs", Path: docs, Include: []string{"**/*.md"}}); err != nil {
		t.Fatalf("AddCollection: %v", err)
	}
	result, err := store.UpdateCollection(ctx, "docs", qmd.UpdateOptions{})
	if err != nil {
		t.Fatalf("UpdateCollection: %v", err)
	}
	if result.Indexed != 1 {
		t.Fatalf("Indexed = %d, want 1", result.Indexed)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	var storedDoc string
	if err := db.QueryRow(`SELECT content FROM entries WHERE article_path = ?`, "user-management.md").Scan(&storedDoc); err != nil {
		t.Fatalf("query stored doc: %v", err)
	}
	wantDoc := "---\ntag: audit\n---\n\n# 用户管理\n\n用户管理探测会检查 operation_field 配置。\n"
	if storedDoc != wantDoc {
		t.Fatalf("stored doc = %q, want raw markdown", storedDoc)
	}

	frontmatterHits, err := store.Search(ctx, "docs", "audit", qmd.SearchOptions{Limit: 5})
	if err != nil {
		t.Fatalf("Search frontmatter: %v", err)
	}
	if len(frontmatterHits) != 1 || frontmatterHits[0].Path != "user-management.md" {
		t.Fatalf("Search frontmatter hits = %#v, want user-management.md", frontmatterHits)
	}

	hits, err := store.Search(ctx, "docs", "operation_field", qmd.SearchOptions{Limit: 5})
	if err != nil {
		t.Fatalf("Search operation_field: %v", err)
	}
	if len(hits) != 1 || hits[0].Path != "user-management.md" {
		t.Fatalf("Search operation_field hits = %#v, want user-management.md", hits)
	}

	cjkHits, err := store.Search(ctx, "docs", "用户管理", qmd.SearchOptions{Limit: 5})
	if err != nil {
		t.Fatalf("Search CJK: %v", err)
	}
	if len(cjkHits) != 1 || !strings.Contains(cjkHits[0].Snippet, "用户管理") {
		t.Fatalf("Search CJK hits = %#v, want snippet containing 用户管理", cjkHits)
	}
}

func TestSearchRanksTitleMatchesBeforeBodyMatches(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "title.md"), []byte("# Alpha\n\nBrief note.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "body.md"), []byte("# Ordinary\n\nAlpha appears once in body text.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := qmd.NewStore(qmd.Config{DBPath: filepath.Join(dir, "wiki.db")})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	if err := store.AddCollection(ctx, qmd.Collection{Name: "docs", Path: docs, Include: []string{"**/*.md"}}); err != nil {
		t.Fatalf("AddCollection: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "docs", qmd.UpdateOptions{}); err != nil {
		t.Fatalf("UpdateCollection: %v", err)
	}

	hits, err := store.Search(ctx, "docs", "alpha", qmd.SearchOptions{Limit: 2})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("Search returned %d hits, want 2: %#v", len(hits), hits)
	}
	if hits[0].Path != "title.md" {
		t.Fatalf("first hit = %s, want title.md due to title weighting; hits=%#v", hits[0].Path, hits)
	}
	if hits[0].Score <= hits[1].Score {
		t.Fatalf("title score %.6f must be greater than body score %.6f", hits[0].Score, hits[1].Score)
	}
	if hits[0].Score <= 0 || hits[0].Score >= 1 {
		t.Fatalf("score = %.6f, want stable range (0,1)", hits[0].Score)
	}
}

func TestSearchDefaultLimitMatchesQMDFTSDefault(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 25; i++ {
		name := filepath.Join(docs, "alpha-"+string(rune('a'+i))+".md")
		if err := os.WriteFile(name, []byte("# Alpha\n\nAlpha default limit contract.\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	store, err := qmd.NewStore(qmd.Config{DBPath: filepath.Join(dir, "wiki.db")})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	if err := store.AddCollection(ctx, qmd.Collection{Name: "docs", Path: docs, Include: []string{"**/*.md"}}); err != nil {
		t.Fatalf("AddCollection: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "docs", qmd.UpdateOptions{}); err != nil {
		t.Fatalf("UpdateCollection: %v", err)
	}

	hits, err := store.Search(ctx, "docs", "alpha", qmd.SearchOptions{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 20 {
		t.Fatalf("default Search returned %d hits, want qmd searchFTS default 20", len(hits))
	}
}

func TestSearchFiltersCollectionInsideQueryBeforeLimit(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	if err := os.MkdirAll(a, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(b, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		name := filepath.Join(a, "alpha-"+string(rune('a'+i))+".md")
		if err := os.WriteFile(name, []byte("# Alpha\n\nAlpha repeated alpha alpha.\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(b, "only.md"), []byte("# Alpha\n\nAlpha in target collection.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := qmd.NewStore(qmd.Config{DBPath: filepath.Join(dir, "wiki.db")})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	if err := store.AddCollection(ctx, qmd.Collection{Name: "a", Path: a, Include: []string{"**/*.md"}}); err != nil {
		t.Fatalf("AddCollection a: %v", err)
	}
	if err := store.AddCollection(ctx, qmd.Collection{Name: "b", Path: b, Include: []string{"**/*.md"}}); err != nil {
		t.Fatalf("AddCollection b: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "a", qmd.UpdateOptions{}); err != nil {
		t.Fatalf("UpdateCollection a: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "b", qmd.UpdateOptions{}); err != nil {
		t.Fatalf("UpdateCollection b: %v", err)
	}

	hits, err := store.Search(ctx, "b", "alpha", qmd.SearchOptions{Limit: 1})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 || hits[0].Collection != "b" || hits[0].Path != "only.md" {
		t.Fatalf("collection-filtered search hits = %#v, want b/only.md", hits)
	}
}

func TestSearchSupportsLexSyntaxHyphenDottedNegationAndCJK(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"agent.md":  "# Multi Agent\n\nmulti-agent memory DEC-0054 version 2026.4.10 用户管理.\n",
		"sports.md": "# Sports\n\nmulti-agent sports memory DEC-0054 version 2026.4.10.\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(docs, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	store, err := qmd.NewStore(qmd.Config{DBPath: filepath.Join(dir, "wiki.db")})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	if err := store.AddCollection(ctx, qmd.Collection{Name: "docs", Path: docs, Include: []string{"**/*.md"}}); err != nil {
		t.Fatalf("AddCollection: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "docs", qmd.UpdateOptions{}); err != nil {
		t.Fatalf("UpdateCollection: %v", err)
	}

	cases := []struct {
		name  string
		query string
		want  string
	}{
		{name: "hyphen", query: "multi-agent", want: "agent.md"},
		{name: "dotted", query: "2026.4.10", want: "sports.md"},
		{name: "negation", query: "memory -sports", want: "agent.md"},
		{name: "cjk", query: "用户管理", want: "agent.md"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hits, err := store.Search(ctx, "docs", tc.query, qmd.SearchOptions{Limit: 5})
			if err != nil {
				t.Fatalf("Search %q: %v", tc.query, err)
			}
			if len(hits) == 0 || hits[0].Path != tc.want {
				t.Fatalf("Search %q hits = %#v, want first %s", tc.query, hits, tc.want)
			}
		})
	}
}

func TestSearchRanksTitleMatchesAboveBodyMatches(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "title.md"), []byte("# Cache\n\nshort note\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "body.md"), []byte("# Misc\n\ncache cache cache cache cache cache cache cache cache cache\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := qmd.NewStore(qmd.Config{DBPath: filepath.Join(dir, "wiki.db")})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	if err := store.AddCollection(ctx, qmd.Collection{Name: "docs", Path: docs, Include: []string{"**/*.md"}}); err != nil {
		t.Fatalf("AddCollection: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "docs", qmd.UpdateOptions{}); err != nil {
		t.Fatalf("UpdateCollection: %v", err)
	}

	hits, err := store.Search(ctx, "docs", "cache", qmd.SearchOptions{Limit: 2})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("len(hits) = %d, want 2: %#v", len(hits), hits)
	}
	if hits[0].Path != "body.md" {
		t.Fatalf("first hit = %s, want body.md because BM25 keeps正文重复命中优先; hits=%#v", hits[0].Path, hits)
	}
	if hits[0].Score <= 0 || hits[0].Score >= 1 {
		t.Fatalf("score = %f, want stable positive score in [0,1)", hits[0].Score)
	}
}

func TestSearchCollectionFilterDoesNotLoseHitsToOtherCollections(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	many := filepath.Join(dir, "many")
	target := filepath.Join(dir, "target")
	if err := os.MkdirAll(many, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 40; i++ {
		name := filepath.Join(many, strings.ReplaceAll("other-cache-"+string(rune('a'+(i%26)))+".md", string(filepath.Separator), "-"))
		if err := os.WriteFile(name, []byte("# Cache\n\ncache from another collection\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(target, "wanted.md"), []byte("# Cache\n\ncache in target collection\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := qmd.NewStore(qmd.Config{DBPath: filepath.Join(dir, "wiki.db")})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	if err := store.AddCollection(ctx, qmd.Collection{Name: "many", Path: many, Include: []string{"**/*.md"}}); err != nil {
		t.Fatalf("AddCollection many: %v", err)
	}
	if err := store.AddCollection(ctx, qmd.Collection{Name: "target", Path: target, Include: []string{"**/*.md"}}); err != nil {
		t.Fatalf("AddCollection target: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "many", qmd.UpdateOptions{}); err != nil {
		t.Fatalf("Update many: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "target", qmd.UpdateOptions{}); err != nil {
		t.Fatalf("Update target: %v", err)
	}

	hits, err := store.Search(ctx, "target", "cache", qmd.SearchOptions{Limit: 1})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("hits = %#v, want empty result when reference candidate limit is occupied by other collections", hits)
	}
}

func TestSearchMatchesContinuousCJKText(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "cjk.md"), []byte("# 系统功能\n\n用户管理探测流程记录。\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := qmd.NewStore(qmd.Config{DBPath: filepath.Join(dir, "wiki.db")})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	if err := store.AddCollection(ctx, qmd.Collection{Name: "docs", Path: docs, Include: []string{"**/*.md"}}); err != nil {
		t.Fatalf("AddCollection: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "docs", qmd.UpdateOptions{}); err != nil {
		t.Fatalf("UpdateCollection: %v", err)
	}

	hits, err := store.Search(ctx, "docs", "用户管理探测", qmd.SearchOptions{Limit: 3})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 || hits[0].Path != "cjk.md" {
		t.Fatalf("hits = %#v, want cjk.md", hits)
	}
}

func TestSearchMatchesCJKTextAdjacentToLatinText(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "mixed.md"), []byte("# 系统功能\n\nabc用户管理def 仍然应该能被中文短语查询命中。\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := qmd.NewStore(qmd.Config{DBPath: filepath.Join(dir, "wiki.db")})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	if err := store.AddCollection(ctx, qmd.Collection{Name: "docs", Path: docs, Include: []string{"**/*.md"}}); err != nil {
		t.Fatalf("AddCollection: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "docs", qmd.UpdateOptions{}); err != nil {
		t.Fatalf("UpdateCollection: %v", err)
	}

	hits, err := store.Search(ctx, "docs", "用户管理", qmd.SearchOptions{Limit: 3})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 || hits[0].Path != "mixed.md" {
		t.Fatalf("hits = %#v, want mixed.md", hits)
	}
	if !strings.Contains(hits[0].Snippet, "abc 用户管理 def") {
		t.Fatalf("snippet = %q, want restored CJK text with readable Latin boundaries", hits[0].Snippet)
	}
}

func TestVectorSearchAndQueryUseCollectionChunks(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "alpha.md"), []byte("# Alpha\n\nAlpha 向量 document explains local embeddings.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "beta.md"), []byte("# Beta\n\nBeta document is about a different topic.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := qmd.NewStore(qmd.Config{
		DBPath:    filepath.Join(dir, "wiki.db"),
		ChunkSize: 50,
		Embedder:  fakeEmbedder{},
	})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	if err := store.AddCollection(ctx, qmd.Collection{Name: "docs", Path: docs, Include: []string{"**/*.md"}}); err != nil {
		t.Fatalf("AddCollection: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "docs", qmd.UpdateOptions{}); err != nil {
		t.Fatalf("UpdateCollection: %v", err)
	}
	embedCollectionForTest(t, ctx, store, "docs")

	vectorHits, err := store.VSearch(ctx, "docs", "find alpha embeddings", qmd.SearchOptions{Limit: 2})
	if err != nil {
		t.Fatalf("VSearch: %v", err)
	}
	if len(vectorHits) == 0 || vectorHits[0].Path != "alpha.md" {
		t.Fatalf("VSearch hits = %#v, want alpha.md first", vectorHits)
	}

	answer, err := store.Query(ctx, "docs", "find alpha embeddings", qmd.QueryOptions{Limit: 2})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(answer.Results) == 0 || answer.Results[0].Path != "alpha.md" {
		t.Fatalf("Query results = %#v, want alpha.md first", answer.Results)
	}
	if answer.Answer != "" {
		t.Fatalf("Query Answer = %q, want empty without LLM", answer.Answer)
	}
}

func TestVSearchFormatsDocumentAndQueryEmbeddingInputs(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "alpha.md"), []byte("# Alpha\n\nAlpha vector text.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	embedder := &recordingEmbedder{}
	store, err := qmd.NewStore(qmd.Config{
		DBPath:    filepath.Join(dir, "wiki.db"),
		ChunkSize: 50,
		Embedder:  embedder,
	})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	if err := store.AddCollection(ctx, qmd.Collection{Name: "docs", Path: docs, Include: []string{"**/*.md"}}); err != nil {
		t.Fatalf("AddCollection: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "docs", qmd.UpdateOptions{}); err != nil {
		t.Fatalf("UpdateCollection: %v", err)
	}
	embedCollectionForTest(t, ctx, store, "docs")
	if len(embedder.inputs) == 0 || !strings.HasPrefix(embedder.inputs[0], "title: Alpha | text: ") {
		t.Fatalf("document embedding input = %#v, want title/text formatted input", embedder.inputs)
	}

	embedder.inputs = nil
	if _, err := store.VSearch(ctx, "docs", "alpha", qmd.SearchOptions{Limit: 1, MinScore: -1}); err != nil {
		t.Fatalf("VSearch: %v", err)
	}
	if len(embedder.inputs) == 0 || embedder.inputs[0] != "task: search result | query: alpha" {
		t.Fatalf("query embedding input = %#v, want task/query formatted input", embedder.inputs)
	}
}

func TestVSearchUsesQwenEmbeddingInstructionFormat(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "alpha.md"), []byte("# Alpha\n\nAlpha vector text.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	embedder := &recordingEmbedder{name: "ollama/qwen3-embedding"}
	store, err := qmd.NewStore(qmd.Config{
		DBPath:    filepath.Join(dir, "wiki.db"),
		ChunkSize: 50,
		Embedder:  embedder,
	})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	if err := store.AddCollection(ctx, qmd.Collection{Name: "docs", Path: docs, Include: []string{"**/*.md"}}); err != nil {
		t.Fatalf("AddCollection: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "docs", qmd.UpdateOptions{}); err != nil {
		t.Fatalf("UpdateCollection: %v", err)
	}
	embedCollectionForTest(t, ctx, store, "docs")
	if len(embedder.inputs) == 0 || embedder.inputs[0] != "Alpha\n# Alpha\n\nAlpha vector text." {
		t.Fatalf("document embedding input = %#v, want Qwen raw title/body format", embedder.inputs)
	}

	embedder.inputs = nil
	if _, err := store.VSearch(ctx, "docs", "alpha", qmd.SearchOptions{Limit: 1, MinScore: -1}); err != nil {
		t.Fatalf("VSearch: %v", err)
	}
	want := "Instruct: Retrieve relevant documents for the given query\nQuery: alpha"
	if len(embedder.inputs) == 0 || embedder.inputs[0] != want {
		t.Fatalf("query embedding input = %#v, want Qwen query instruction format", embedder.inputs)
	}
}

func TestVSearchEmbeddingFingerprintMatchesQMDContract(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "alpha.md"), []byte("# Alpha\n\nAlpha vector text.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(dir, "wiki.db")
	embedder := &recordingEmbedder{name: "ollama/qwen3-embedding"}
	store, err := qmd.NewStore(qmd.Config{
		DBPath:    dbPath,
		ChunkSize: 50,
		Embedder:  embedder,
	})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.AddCollection(ctx, qmd.Collection{Name: "docs", Path: docs, Include: []string{"**/*.md"}}); err != nil {
		t.Fatalf("AddCollection: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "docs", qmd.UpdateOptions{}); err != nil {
		t.Fatalf("UpdateCollection: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close store: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath+"?mode=ro")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	var actual string
	if err := db.QueryRow(`SELECT embedding_model FROM collection_documents WHERE collection='docs' AND rel_path='alpha.md'`).Scan(&actual); err != nil {
		t.Fatalf("read embedding fingerprint: %v", err)
	}

	significant := strings.Join([]string{
		"model:ollama/qwen3-embedding",
		"query:Instruct: Retrieve relevant documents for the given query\nQuery: __qmd_embedding_query_probe__",
		"doc:__qmd_embedding_title_probe__\n__qmd_embedding_document_probe__",
		"chunk_tokens:50",
		"chunk_overlap_tokens:7",
	}, "\n")
	sum := sha256.Sum256([]byte(significant))
	want := hex.EncodeToString(sum[:])[:6]
	if actual != want {
		t.Fatalf("embedding fingerprint = %q, want qmd sha256 prefix %q from significant input:\n%s", actual, want, significant)
	}
}

func TestVSearchUsesVecAndHyDEExpansionsOnly(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "alpha.md"), []byte("# Alpha\n\nAlpha topic.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "beta.md"), []byte("# Beta\n\nBeta topic.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	embedder := &recordingEmbedder{}
	store, err := qmd.NewStore(qmd.Config{
		DBPath:    filepath.Join(dir, "wiki.db"),
		ChunkSize: 50,
		Embedder:  embedder,
		QueryExpander: fakeQueryExpander{items: []qmd.QueryExpansion{
			{Type: qmd.QueryExpansionLex, Text: "lex beta"},
			{Type: qmd.QueryExpansionVec, Text: "alpha"},
			{Type: qmd.QueryExpansionVec, Text: "semantic beta"},
			{Type: qmd.QueryExpansionHyDE, Text: "hypothetical beta"},
		}},
	})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	if err := store.AddCollection(ctx, qmd.Collection{Name: "docs", Path: docs, Include: []string{"**/*.md"}}); err != nil {
		t.Fatalf("AddCollection: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "docs", qmd.UpdateOptions{}); err != nil {
		t.Fatalf("UpdateCollection: %v", err)
	}
	embedCollectionForTest(t, ctx, store, "docs")

	embedder.inputs = nil
	hits, err := store.VSearch(ctx, "docs", "alpha", qmd.SearchOptions{Limit: 2, MinScore: -1})
	if err != nil {
		t.Fatalf("VSearch: %v", err)
	}
	seenBeta := false
	for _, hit := range hits {
		if hit.Path == "beta.md" {
			seenBeta = true
			break
		}
	}
	if !seenBeta {
		t.Fatalf("expanded VSearch hits = %#v, want beta.md recalled by vec/hyde expansion", hits)
	}
	joined := strings.Join(embedder.inputs, "\n")
	if strings.Contains(joined, "lex beta") {
		t.Fatalf("VSearch embedded lex expansion input: %#v", embedder.inputs)
	}
	alphaInputs := 0
	for _, input := range embedder.inputs {
		if strings.Contains(input, "alpha") {
			alphaInputs++
		}
	}
	if alphaInputs != 1 {
		t.Fatalf("VSearch alpha inputs = %#v, want duplicate original expansion filtered", embedder.inputs)
	}
	if !strings.Contains(joined, "semantic beta") || !strings.Contains(joined, "hypothetical beta") {
		t.Fatalf("VSearch inputs = %#v, want vec and hyde expansions", embedder.inputs)
	}
}

func TestVSearchSkipsEmbeddingWhenNoVectorsLikeQMD(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "alpha.md"), []byte("# Alpha\n\nAlpha topic.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	embedder := &recordingEmbedder{}
	store, err := qmd.NewStore(qmd.Config{
		DBPath:   filepath.Join(dir, "wiki.db"),
		Embedder: embedder,
	})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	if err := store.AddCollection(ctx, qmd.Collection{Name: "docs", Path: docs, Include: []string{"**/*.md"}}); err != nil {
		t.Fatalf("AddCollection: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "docs", qmd.UpdateOptions{}); err != nil {
		t.Fatalf("UpdateCollection: %v", err)
	}

	hits, err := store.VSearch(ctx, "docs", "alpha", qmd.SearchOptions{Limit: 5})
	if err != nil {
		t.Fatalf("VSearch: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("VSearch hits = %#v, want empty result without vectors", hits)
	}
	if len(embedder.inputs) != 0 {
		t.Fatalf("VSearch embed inputs = %#v, want vector path skipped without vectors", embedder.inputs)
	}
}

func TestSearchVectorSkipsEmbeddingWhenNoVectorsLikeQMD(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "alpha.md"), []byte("# Alpha\n\nAlpha topic.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	embedder := &recordingEmbedder{}
	store, err := qmd.NewStore(qmd.Config{
		DBPath:   filepath.Join(dir, "wiki.db"),
		Embedder: embedder,
	})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	if err := store.AddCollection(ctx, qmd.Collection{Name: "docs", Path: docs, Include: []string{"**/*.md"}}); err != nil {
		t.Fatalf("AddCollection: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "docs", qmd.UpdateOptions{}); err != nil {
		t.Fatalf("UpdateCollection: %v", err)
	}

	hits, err := store.SearchVector(ctx, "alpha", qmd.VectorSearchOptions{Collection: "docs", Limit: 5})
	if err != nil {
		t.Fatalf("SearchVector: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("SearchVector hits = %#v, want empty result without vectors", hits)
	}
	if len(embedder.inputs) != 0 {
		t.Fatalf("SearchVector embed inputs = %#v, want vector path skipped without vectors", embedder.inputs)
	}
}

func TestVSearchDefaultMinScoreFiltersWeakVectorMatches(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "alpha.md"), []byte("# Alpha\n\nAlpha topic.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := qmd.NewStore(qmd.Config{
		DBPath:    filepath.Join(dir, "wiki.db"),
		ChunkSize: 50,
		Embedder:  &recordingEmbedder{},
	})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	if err := store.AddCollection(ctx, qmd.Collection{Name: "docs", Path: docs, Include: []string{"**/*.md"}}); err != nil {
		t.Fatalf("AddCollection: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "docs", qmd.UpdateOptions{}); err != nil {
		t.Fatalf("UpdateCollection: %v", err)
	}
	embedCollectionForTest(t, ctx, store, "docs")

	hits, err := store.VSearch(ctx, "docs", "unrelated", qmd.SearchOptions{Limit: 1})
	if err != nil {
		t.Fatalf("VSearch default min score: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("VSearch default min score hits = %#v, want weak negative matches filtered", hits)
	}

	hits, err = store.VSearch(ctx, "docs", "unrelated", qmd.SearchOptions{Limit: 1, MinScore: -2})
	if err != nil {
		t.Fatalf("VSearch explicit low min score: %v", err)
	}
	if len(hits) != 1 || hits[0].Path != "alpha.md" {
		t.Fatalf("VSearch explicit low min score hits = %#v, want alpha.md", hits)
	}
}

func TestVSearchEmptyCollectionSearchesAllCollections(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	notes := filepath.Join(dir, "notes")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(notes, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "alpha-doc.md"), []byte("# Alpha Doc\n\nAlpha topic.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notes, "alpha-note.md"), []byte("# Alpha Note\n\nAlpha topic.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := qmd.NewStore(qmd.Config{
		DBPath:    filepath.Join(dir, "wiki.db"),
		ChunkSize: 50,
		Embedder:  fakeEmbedder{},
	})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	if err := store.AddCollection(ctx, qmd.Collection{Name: "docs", Path: docs, Include: []string{"**/*.md"}}); err != nil {
		t.Fatalf("AddCollection docs: %v", err)
	}
	if err := store.AddCollection(ctx, qmd.Collection{Name: "notes", Path: notes, Include: []string{"**/*.md"}}); err != nil {
		t.Fatalf("AddCollection notes: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "docs", qmd.UpdateOptions{}); err != nil {
		t.Fatalf("UpdateCollection docs: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "notes", qmd.UpdateOptions{}); err != nil {
		t.Fatalf("UpdateCollection notes: %v", err)
	}
	embedCollectionForTest(t, ctx, store, "docs")
	embedCollectionForTest(t, ctx, store, "notes")

	hits, err := store.VSearch(ctx, "", "alpha", qmd.SearchOptions{Limit: 10, MinScore: -1})
	if err != nil {
		t.Fatalf("VSearch: %v", err)
	}
	seen := map[string]bool{}
	for _, hit := range hits {
		seen[hit.Collection+"/"+hit.Path] = true
	}
	if !seen["docs/alpha-doc.md"] || !seen["notes/alpha-note.md"] {
		t.Fatalf("VSearch empty collection hits = %#v, want both collections", hits)
	}
}

func TestEmbedCollectionRebuildsVectorsWhenEmbeddingFingerprintChanges(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "alpha.md"), []byte("# Alpha\n\nAlpha topic.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(dir, "wiki.db")
	first := &recordingEmbedder{name: "model-a"}
	store, err := qmd.NewStore(qmd.Config{DBPath: dbPath, ChunkSize: 50, Embedder: first})
	if err != nil {
		t.Fatalf("NewStore first: %v", err)
	}
	if err := store.AddCollection(ctx, qmd.Collection{Name: "docs", Path: docs, Include: []string{"**/*.md"}}); err != nil {
		t.Fatalf("AddCollection first: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "docs", qmd.UpdateOptions{}); err != nil {
		t.Fatalf("Update first: %v", err)
	}
	if _, err := store.EmbedCollection(ctx, "docs", qmd.EmbedOptions{}); err != nil {
		t.Fatalf("Embed first: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close first: %v", err)
	}

	second := &recordingEmbedder{name: "model-b"}
	store, err = qmd.NewStore(qmd.Config{DBPath: dbPath, ChunkSize: 50, Embedder: second})
	if err != nil {
		t.Fatalf("NewStore second: %v", err)
	}
	defer store.Close()
	if err := store.AddCollection(ctx, qmd.Collection{Name: "docs", Path: docs, Include: []string{"**/*.md"}}); err != nil {
		t.Fatalf("AddCollection second: %v", err)
	}
	result, err := store.EmbedCollection(ctx, "docs", qmd.EmbedOptions{})
	if err != nil {
		t.Fatalf("Embed second: %v", err)
	}
	if result.Embedded == 0 || len(second.inputs) == 0 {
		t.Fatalf("second embed result = %#v inputs=%#v, want vectors rebuilt after fingerprint change", result, second.inputs)
	}
}

func TestUpdateCollectionDoesNotGenerateEmbeddings(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "alpha.md"), []byte("# Alpha\n\nAlpha topic.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	embedder := &recordingEmbedder{name: "model-a"}
	store, err := qmd.NewStore(qmd.Config{DBPath: filepath.Join(dir, "wiki.db"), ChunkSize: 50, Embedder: embedder})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	if err := store.AddCollection(ctx, qmd.Collection{Name: "docs", Path: docs, Include: []string{"**/*.md"}}); err != nil {
		t.Fatalf("AddCollection: %v", err)
	}

	result, err := store.UpdateCollection(ctx, "docs", qmd.UpdateOptions{})
	if err != nil {
		t.Fatalf("UpdateCollection: %v", err)
	}
	if result.Embedded != 0 || len(embedder.inputs) != 0 {
		t.Fatalf("update result=%#v inputs=%#v, want no embedding work", result, embedder.inputs)
	}
}

func TestEmbeddingAPIProviderStoresChunkVectors(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "alpha.md"), []byte("# Alpha\n\nAlpha API embedding text.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Fatalf("path = %q, want /embeddings", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("Authorization = %q, want bearer key", r.Header.Get("Authorization"))
		}
		var body struct {
			Model string `json:"model"`
			Input string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body.Model != "api-embedding" || !strings.Contains(body.Input, "Alpha") {
			t.Fatalf("body = %#v, want configured model and document text", body)
		}
		requests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[1,0,0]}]}`))
	}))
	defer server.Close()

	dbPath := filepath.Join(dir, "wiki.db")
	store, err := qmd.NewStore(qmd.Config{
		DBPath:    dbPath,
		ChunkSize: 50,
		Embedding: qmd.EmbeddingConfig{
			Provider: "api",
			Model:    "api-embedding",
			APIKey:   "test-key",
			BaseURL:  server.URL,
		},
	})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.AddCollection(ctx, qmd.Collection{Name: "docs", Path: docs, Include: []string{"**/*.md"}}); err != nil {
		t.Fatalf("AddCollection: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "docs", qmd.UpdateOptions{}); err != nil {
		t.Fatalf("UpdateCollection: %v", err)
	}
	result, err := store.EmbedCollection(ctx, "docs", qmd.EmbedOptions{})
	if err != nil {
		t.Fatalf("EmbedCollection: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close store: %v", err)
	}
	if result.Embedded == 0 || requests == 0 {
		t.Fatalf("embed result=%#v requests=%d, want API embeddings generated", result, requests)
	}

	assertStoredEmbedding(t, dbPath, "api/api-embedding", 3)
}

func TestEmbeddingOllamaProviderStoresChunkVectors(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "alpha.md"), []byte("# Alpha\n\nAlpha Ollama embedding text.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embeddings" {
			t.Fatalf("path = %q, want /api/embeddings", r.URL.Path)
		}
		var body struct {
			Model  string `json:"model"`
			Prompt string `json:"prompt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body.Model != "nomic-embed-text" || !strings.Contains(body.Prompt, "Alpha") {
			t.Fatalf("body = %#v, want configured model and document prompt", body)
		}
		requests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"embedding":[0,1]}`))
	}))
	defer server.Close()

	dbPath := filepath.Join(dir, "wiki.db")
	store, err := qmd.NewStore(qmd.Config{
		DBPath:    dbPath,
		ChunkSize: 50,
		Embedding: qmd.EmbeddingConfig{
			Provider: "ollama",
			Model:    "nomic-embed-text",
			BaseURL:  server.URL,
		},
	})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.AddCollection(ctx, qmd.Collection{Name: "docs", Path: docs, Include: []string{"**/*.md"}}); err != nil {
		t.Fatalf("AddCollection: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "docs", qmd.UpdateOptions{}); err != nil {
		t.Fatalf("UpdateCollection: %v", err)
	}
	result, err := store.EmbedCollection(ctx, "docs", qmd.EmbedOptions{})
	if err != nil {
		t.Fatalf("EmbedCollection: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close store: %v", err)
	}
	if result.Embedded == 0 || requests == 0 {
		t.Fatalf("embed result=%#v requests=%d, want Ollama embeddings generated", result, requests)
	}

	assertStoredEmbedding(t, dbPath, "ollama/nomic-embed-text", 2)
}

func TestEmbeddingLocalProviderStoresChunkVectors(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "alpha.md"), []byte("# Alpha\n\nAlpha local embedding text.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	command := filepath.Join(dir, "llama-embedding")
	if err := os.WriteFile(command, []byte(`#!/bin/sh
printf '{"data":[{"embedding":[0.5,0.25,0.125]}]}'
`), 0o755); err != nil {
		t.Fatalf("write command: %v", err)
	}
	modelPath := filepath.Join(dir, "embedding-model.gguf")

	dbPath := filepath.Join(dir, "wiki.db")
	store, err := qmd.NewStore(qmd.Config{
		DBPath:    dbPath,
		ChunkSize: 50,
		Embedding: qmd.EmbeddingConfig{
			Provider: "local",
			Model:    modelPath,
			Command:  command,
		},
	})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.AddCollection(ctx, qmd.Collection{Name: "docs", Path: docs, Include: []string{"**/*.md"}}); err != nil {
		t.Fatalf("AddCollection: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "docs", qmd.UpdateOptions{}); err != nil {
		t.Fatalf("UpdateCollection: %v", err)
	}
	result, err := store.EmbedCollection(ctx, "docs", qmd.EmbedOptions{})
	if err != nil {
		t.Fatalf("EmbedCollection: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close store: %v", err)
	}
	if result.Embedded == 0 {
		t.Fatalf("embed result=%#v, want local embeddings generated", result)
	}

	assertStoredEmbedding(t, dbPath, "local/"+modelPath, 3)
}

func assertStoredEmbedding(t *testing.T, dbPath string, wantModel string, wantDims int) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath+"?mode=ro")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	var model, fingerprint string
	var dims, blobLen int
	if err := db.QueryRow(`SELECT model, embed_fingerprint, dimensions, length(embedding) FROM vec_chunks LIMIT 1`).Scan(&model, &fingerprint, &dims, &blobLen); err != nil {
		t.Fatalf("read vec_chunks: %v", err)
	}
	if model != wantModel {
		t.Fatalf("stored model = %q, want %q", model, wantModel)
	}
	if fingerprint == "" {
		t.Fatalf("stored fingerprint is empty")
	}
	if dims != wantDims || blobLen != wantDims*4 {
		t.Fatalf("stored dims=%d blobLen=%d, want dims=%d blobLen=%d", dims, blobLen, wantDims, wantDims*4)
	}

	var vecRows int
	table := "vec_chunks_vec_2"
	if wantDims == 3 {
		table = "vec_chunks_vec_3"
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&vecRows); err != nil {
		t.Fatalf("read %s: %v", table, err)
	}
	if vecRows == 0 {
		t.Fatalf("%s rows = 0, want sqlite-vec row", table)
	}
}
