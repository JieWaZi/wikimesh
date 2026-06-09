package qmd

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// referenceSearchHit 是参考算法查询返回的最小字段集合。
type referenceSearchHit struct {
	// Path 是命中文档在 collection 内的相对路径。
	Path string

	// Title 是命中文档标题。
	Title string

	// Score 是 BM25 映射后的正向分数。
	Score float64
}

// referenceSearchDocuments 使用参考算法的 FTS5 查询结构。
// 这个函数只用于测试和 benchmark，确保正式 Search 的 SQL 形态和排序效果保持一致。
func referenceSearchDocuments(ctx context.Context, store *Store, collection string, query string, limit int) ([]referenceSearchHit, error) {
	if limit <= 0 {
		limit = 10
	}
	ftsQuery, err := buildSearchFTSQuery(query)
	if err != nil {
		return nil, err
	}
	if ftsQuery == "" {
		return nil, nil
	}
	candidateLimit := limit
	if collection != "" {
		candidateLimit = limit * 10
	}

	sqlText := `
WITH fts_matches AS (
	SELECT rowid, bm25(documents_fts, 1.5, 4.0, 1.0) AS bm25_score
	FROM documents_fts
	WHERE documents_fts MATCH ?
	ORDER BY bm25_score ASC
	LIMIT ?
)
SELECT d.rel_path, d.title, fts_matches.bm25_score
FROM fts_matches
JOIN collection_documents d ON d.rowid = fts_matches.rowid
WHERE d.active = 1
`
	args := []any{ftsQuery, candidateLimit}
	if collection != "" {
		sqlText += `  AND d.collection = ?
`
		args = append(args, collection)
	}
	sqlText += `
ORDER BY fts_matches.bm25_score ASC
LIMIT ?
`
	args = append(args, limit)

	rows, err := store.db.ReadDB().QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	hits := make([]referenceSearchHit, 0, limit)
	for rows.Next() {
		var hit referenceSearchHit
		var bm25 float64
		if err := rows.Scan(&hit.Path, &hit.Title, &bm25); err != nil {
			return nil, err
		}
		hit.Score = bm25ToScore(bm25)
		hits = append(hits, hit)
	}
	return hits, rows.Err()
}

func TestSearchMatchesReferenceResults(t *testing.T) {
	ctx := context.Background()
	store := newReferenceSearchFixture(t, 0)
	defer store.Close()

	cases := []struct {
		name       string
		query      string
		collection string
		limit      int
	}{
		{name: "plain", collection: "docs", query: "query_source", limit: 5},
		{name: "phrase", collection: "docs", query: `"root hint"`, limit: 5},
		{name: "unclosed_phrase", collection: "docs", query: `"root hint`, limit: 5},
		{name: "hyphen", collection: "docs", query: "multi-agent", limit: 5},
		{name: "dotted", collection: "docs", query: "2026.4.10", limit: 5},
		{name: "negation", collection: "docs", query: "memory -sports", limit: 5},
		{name: "cjk", collection: "docs", query: "根提示", limit: 5},
		{name: "path", collection: "docs", query: "runbook", limit: 5},
		{name: "title_weight", collection: "docs", query: "cache", limit: 2},
		{name: "global_collection", collection: "", query: "query_source", limit: 5},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			current, err := store.Search(ctx, tc.collection, tc.query, SearchOptions{Limit: tc.limit})
			if err != nil {
				t.Fatalf("current search: %v", err)
			}
			reference, err := referenceSearchDocuments(ctx, store, tc.collection, tc.query, tc.limit)
			if err != nil {
				t.Fatalf("reference search: %v", err)
			}
			if len(reference) == 0 {
				t.Fatalf("reference search returned no hits for %q", tc.query)
			}
			assertSearchMatchesReference(t, current, reference)
		})
	}
}

func TestSearchMatchesReferenceCandidateLimit(t *testing.T) {
	ctx := context.Background()
	store := newCollectionCandidateFixture(t)
	defer store.Close()

	current, err := store.Search(ctx, "target", "cache", SearchOptions{Limit: 1})
	if err != nil {
		t.Fatalf("current search: %v", err)
	}
	reference, err := referenceSearchDocuments(ctx, store, "target", "cache", 1)
	if err != nil {
		t.Fatalf("reference search: %v", err)
	}
	assertSearchMatchesReference(t, current, reference)
}

func TestSearchManyFusesIndependentQueriesWithRRF(t *testing.T) {
	ctx := context.Background()
	store := newReferenceSearchFixture(t, 0)
	defer store.Close()

	hits, err := store.SearchMany(ctx, "docs", []string{"root hint", "operational"}, SearchOptions{Limit: 5})
	if err != nil {
		t.Fatalf("SearchMany: %v", err)
	}
	paths := searchResultPaths(hits)
	if !strings.HasPrefix(paths, "ops/runbooks/dns.md,") {
		t.Fatalf("paths = %s, want document matching both queries first", paths)
	}
	if !strings.Contains(paths, "root-hint.md") {
		t.Fatalf("paths = %s, want result from first independent query", paths)
	}
	if hits[0].Score != 1 {
		t.Fatalf("top fused score = %.6f, want normalized score 1", hits[0].Score)
	}
}

func TestBuildSearchFTSQueryMatchesLexSyntaxBoundaries(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  string
	}{
		{name: "negative_dotted", query: "memory -2026.4.10", want: `"memory"* NOT ("2026"* AND "4"* AND "10"*)`},
		{name: "unicode_term", query: "café", want: `"café"*`},
		{name: "apostrophe_term", query: "don't", want: `"don't"*`},
		{name: "invalid_dotted_falls_back_to_plain", query: "foo.bar!", want: `"foobar"*`},
		{name: "trailing_hyphen_falls_back_to_plain", query: "cache-", want: `"cache"*`},
		{name: "quote_starts_new_token", query: `foo"bar"`, want: `"foo"* AND "bar"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := buildSearchFTSQuery(tc.query)
			if err != nil {
				t.Fatalf("buildSearchFTSQuery(%q): %v", tc.query, err)
			}
			if got != tc.want {
				t.Fatalf("buildSearchFTSQuery(%q) = %q, want %q", tc.query, got, tc.want)
			}
		})
	}
}

func BenchmarkSearchCurrentAndReference(b *testing.B) {
	ctx := context.Background()
	store := newReferenceSearchFixture(b, 1200)
	defer store.Close()
	queries := []string{
		"root hint",
		"multi-agent",
		"2026.4.10",
		"根提示",
		"cache",
	}

	b.Run("current", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			query := queries[i%len(queries)]
			hits, err := store.Search(ctx, "docs", query, SearchOptions{Limit: 10})
			if err != nil {
				b.Fatal(err)
			}
			if len(hits) == 0 {
				b.Fatalf("query %q returned no hits", query)
			}
		}
	})

	b.Run("reference", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			query := queries[i%len(queries)]
			hits, err := referenceSearchDocuments(ctx, store, "docs", query, 10)
			if err != nil {
				b.Fatal(err)
			}
			if len(hits) == 0 {
				b.Fatalf("query %q returned no hits", query)
			}
		}
	})
}

type testingFatalHelper interface {
	Helper()
	Fatalf(format string, args ...any)
	TempDir() string
}

func newReferenceSearchFixture(t testingFatalHelper, extraDocs int) *Store {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(filepath.Join(docs, "ops", "runbooks"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}

	files := map[string]string{
		"root-hint.md":          "# Root Hint\n\n根提示探测会检查 query_source 配置。\n\nroot hint runbook.\n",
		"agent.md":              "# Multi Agent\n\nmulti-agent memory DEC-0054 version 2026.4.10 根提示.\n",
		"sports.md":             "# Sports\n\nmulti-agent sports memory DEC-0054 version 2026.4.10.\n",
		"title-cache.md":        "# Cache\n\nshort note\n",
		"body-cache.md":         "# Misc\n\ncache cache cache cache cache cache cache cache cache cache\n",
		"ops/runbooks/dns.md":   "# DNS Runbook\n\nroot hint operational steps.\n",
		"ops/runbooks/cache.md": "# Cache Runbook\n\ncache operational checklist.\n",
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
	for i := 0; i < extraDocs; i++ {
		body := fmt.Sprintf("# Generated %04d\n\nroot hint cache multi-agent version 2026.4.10 根提示 batch %04d.\n", i, i)
		name := filepath.Join(docs, fmt.Sprintf("generated/%04d.md", i))
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			t.Fatalf("mkdir generated: %v", err)
		}
		if err := os.WriteFile(name, []byte(body), 0o644); err != nil {
			t.Fatalf("write generated: %v", err)
		}
	}

	store, err := NewStore(Config{DBPath: filepath.Join(dir, "wiki.db")})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.AddCollection(ctx, Collection{Name: "docs", Path: docs, Include: []string{"**/*.md"}}); err != nil {
		t.Fatalf("AddCollection: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "docs", UpdateOptions{}); err != nil {
		t.Fatalf("UpdateCollection: %v", err)
	}
	return store
}

func newCollectionCandidateFixture(t testingFatalHelper) *Store {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	many := filepath.Join(dir, "many")
	target := filepath.Join(dir, "target")
	if err := os.MkdirAll(many, 0o755); err != nil {
		t.Fatalf("mkdir many: %v", err)
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	for i := 0; i < 40; i++ {
		name := filepath.Join(many, fmt.Sprintf("other-cache-%02d.md", i))
		if err := os.WriteFile(name, []byte("# Cache\n\ncache from another collection\n"), 0o644); err != nil {
			t.Fatalf("write many: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(target, "wanted.md"), []byte("# Cache\n\ncache in target collection\n"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}

	store, err := NewStore(Config{DBPath: filepath.Join(dir, "wiki.db")})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.AddCollection(ctx, Collection{Name: "many", Path: many, Include: []string{"**/*.md"}}); err != nil {
		t.Fatalf("AddCollection many: %v", err)
	}
	if err := store.AddCollection(ctx, Collection{Name: "target", Path: target, Include: []string{"**/*.md"}}); err != nil {
		t.Fatalf("AddCollection target: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "many", UpdateOptions{}); err != nil {
		t.Fatalf("UpdateCollection many: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "target", UpdateOptions{}); err != nil {
		t.Fatalf("UpdateCollection target: %v", err)
	}
	return store
}

func assertSearchMatchesReference(t *testing.T, current []SearchResult, reference []referenceSearchHit) {
	t.Helper()
	if len(current) != len(reference) {
		t.Fatalf("current len=%d reference len=%d; current=%s reference=%s", len(current), len(reference), searchResultPaths(current), referenceHitPaths(reference))
	}
	for i := range reference {
		if current[i].Path != reference[i].Path {
			t.Fatalf("hit[%d] current=%s reference=%s; current=%s reference=%s", i, current[i].Path, reference[i].Path, searchResultPaths(current), referenceHitPaths(reference))
		}
		if math.Abs(current[i].Score-reference[i].Score) > 1e-12 {
			t.Fatalf("score[%d] current=%.12f reference=%.12f; current=%s reference=%s", i, current[i].Score, reference[i].Score, searchResultPaths(current), referenceHitPaths(reference))
		}
	}
}

func searchResultPaths(results []SearchResult) string {
	paths := make([]string, 0, len(results))
	for _, hit := range results {
		paths = append(paths, hit.Path)
	}
	return strings.Join(paths, ",")
}

func referenceHitPaths(results []referenceSearchHit) string {
	paths := make([]string, 0, len(results))
	for _, hit := range results {
		paths = append(paths, hit.Path)
	}
	return strings.Join(paths, ",")
}
