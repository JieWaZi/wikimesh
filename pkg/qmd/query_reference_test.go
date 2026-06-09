package qmd

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type queryContractEmbedder struct{}

func (queryContractEmbedder) Name() string { return "qmd-contract/query" }

func (queryContractEmbedder) Dimensions() int { return 2 }

func (queryContractEmbedder) Embed(text string) ([]float32, error) {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "semantic beta"), strings.Contains(lower, "beta-only"):
		return []float32{0, 1}, nil
	case strings.Contains(lower, "alpha"):
		return []float32{1, 0}, nil
	default:
		return []float32{0.1, 0.1}, nil
	}
}

type countingQueryEmbedder struct {
	texts   []string
	batches [][]string
}

func (e *countingQueryEmbedder) Name() string { return "qmd-contract/counting-query" }

func (e *countingQueryEmbedder) Dimensions() int { return queryContractEmbedder{}.Dimensions() }

func (e *countingQueryEmbedder) Embed(text string) ([]float32, error) {
	e.texts = append(e.texts, text)
	return queryContractEmbedder{}.Embed(text)
}

func (e *countingQueryEmbedder) EmbedBatch(texts []string) ([][]float32, error) {
	copied := append([]string(nil), texts...)
	e.batches = append(e.batches, copied)
	vectors := make([][]float32, 0, len(texts))
	for _, text := range texts {
		vec, err := queryContractEmbedder{}.Embed(text)
		if err != nil {
			return nil, err
		}
		vectors = append(vectors, vec)
	}
	return vectors, nil
}

func (e *countingQueryEmbedder) reset() {
	e.texts = nil
	e.batches = nil
}

type queryContractExpander struct {
	items []QueryExpansion
	calls int
}

func (e *queryContractExpander) Expand(_ context.Context, _ string) ([]QueryExpansion, error) {
	e.calls++
	return e.items, nil
}

type intentAwareQueryExpander struct {
	items   []QueryExpansion
	intents []string
}

func (e *intentAwareQueryExpander) Expand(_ context.Context, _ string) ([]QueryExpansion, error) {
	e.intents = append(e.intents, "")
	return e.items, nil
}

func (e *intentAwareQueryExpander) ExpandWithOptions(_ context.Context, _ string, opts ExpandQueryOptions) ([]QueryExpansion, error) {
	e.intents = append(e.intents, opts.Intent)
	return e.items, nil
}

type queryContractReranker struct {
	seen    []QueryRerankDocument
	queries []string
}

func (r *queryContractReranker) Rerank(_ context.Context, query string, docs []QueryRerankDocument) ([]QueryRerankScore, error) {
	r.queries = append(r.queries, query)
	r.seen = append(r.seen, docs...)
	out := make([]QueryRerankScore, 0, len(docs))
	for _, doc := range docs {
		score := 0.1
		switch {
		case strings.Contains(doc.File, "rerank.md"):
			score = 1.0
		case strings.Contains(doc.File, "alpha.md"):
			score = 0.2
		}
		out = append(out, QueryRerankScore{ID: doc.ID, File: doc.File, Score: score})
	}
	return out, nil
}

func TestQueryMatchesQMDRRFContractWithoutRerank(t *testing.T) {
	ctx := context.Background()
	store := newReferenceQueryFixture(t, Config{
		ChunkSize: 50,
		Embedder:  queryContractEmbedder{},
		QueryExpander: &queryContractExpander{items: []QueryExpansion{
			{Type: QueryExpansionLex, Text: "lex-only"},
			{Type: QueryExpansionVec, Text: "semantic beta"},
			{Type: QueryExpansionHyDE, Text: "beta-only hypothetical answer"},
		}},
	})
	defer store.Close()

	result, err := store.Query(ctx, "docs", "alpha", QueryOptions{
		Limit:          5,
		CandidateLimit: 5,
		SkipRerank:     true,
		Explain:        true,
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if result.Answer != "" {
		t.Fatalf("Answer = %q, want empty retrieval-only result", result.Answer)
	}
	paths := resultPaths(result.Results)
	if len(paths) < 4 {
		t.Fatalf("paths = %#v, want qmd candidate pool with original, lex, and vector hits", paths)
	}
	if indexOfPath(paths, "alpha.md") < 0 || indexOfPath(paths, "lex.md") < 0 {
		t.Fatalf("paths = %#v, want original and lex expansion hits", paths)
	}
	if indexOfPath(paths, "beta.md") < 0 {
		t.Fatalf("paths = %#v, want vector expansion to recall beta.md", paths)
	}
	alpha := findResultPath(t, result.Results, "alpha.md")
	if alpha.File != "qmd://docs/alpha.md" {
		t.Fatalf("File = %q, want qmd URI", alpha.File)
	}
	if alpha.Context != "Alpha context" {
		t.Fatalf("Context = %q, want most specific path context", alpha.Context)
	}
	if !strings.Contains(alpha.BestChunk, "alpha exact") {
		t.Fatalf("BestChunk = %q, want keyword-overlap chunk", alpha.BestChunk)
	}
	if alpha.Explain == nil {
		t.Fatalf("Explain missing for alpha result")
	}
	alphaTrace := alpha.Explain
	if result.Results[0].Explain == nil || result.Results[0].Explain.RRF.TopRankBonus != 0.05 {
		t.Fatalf("first result top-rank bonus = %#v, want qmd rank1 bonus 0.05", result.Results[0].Explain)
	}
	if len(alphaTrace.RRF.Contributions) < 2 {
		t.Fatalf("alpha contributions = %#v, want original FTS and original vector", alphaTrace.RRF.Contributions)
	}
	if !hasOriginalDoubleWeight(alphaTrace.RRF.Contributions) {
		t.Fatalf("alpha contributions = %#v, want original 2x weight contribution", alphaTrace.RRF.Contributions)
	}
	if result.Results[0].Score != 1.0 || result.Results[1].Score != 0.5 || math.Abs(result.Results[2].Score-1.0/3.0) > 1e-12 {
		t.Fatalf("skip-rerank scores = %.6f %.6f %.6f, want qmd position scores", result.Results[0].Score, result.Results[1].Score, result.Results[2].Score)
	}
}

func TestQueryReranksChunksAndBlendsByRRFPosition(t *testing.T) {
	ctx := context.Background()
	reranker := &queryContractReranker{}
	store := newReferenceQueryFixture(t, Config{
		ChunkSize:     10,
		ChunkOverlap:  0.1,
		Embedder:      queryContractEmbedder{},
		QueryReranker: reranker,
	})
	defer store.Close()

	result, err := store.Query(ctx, "docs", "alpha", QueryOptions{
		Limit:          5,
		CandidateLimit: 2,
		Explain:        true,
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(reranker.seen) != 2 {
		t.Fatalf("reranker docs = %d, want candidate limit 2", len(reranker.seen))
	}
	for _, doc := range reranker.seen {
		if strings.Count(doc.Text, "\n\n") > 2 {
			t.Fatalf("reranker received overly large body for %s: %q", doc.File, doc.Text)
		}
	}
	paths := resultPaths(result.Results)
	want := []string{"alpha.md", "rerank.md"}
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
	alpha := result.Results[0]
	if alpha.Explain == nil {
		t.Fatalf("alpha explain missing")
	}
	if alpha.Explain.RRF.Weight != 0.75 {
		t.Fatalf("alpha RRF blend weight = %.2f, want qmd top-3 protection weight 0.75", alpha.Explain.RRF.Weight)
	}
	wantAlphaScore := 0.75*1.0 + 0.25*0.2
	if math.Abs(alpha.Score-wantAlphaScore) > 1e-12 {
		t.Fatalf("alpha blended score = %.6f, want %.6f", alpha.Score, wantAlphaScore)
	}
}

func TestQueryPassesIntentToRerankerLikeQMD(t *testing.T) {
	ctx := context.Background()
	reranker := &queryContractReranker{}
	store := newReferenceQueryFixture(t, Config{
		ChunkSize:     50,
		Embedder:      queryContractEmbedder{},
		QueryReranker: reranker,
	})
	defer store.Close()

	_, err := store.Query(ctx, "docs", "alpha", QueryOptions{
		Limit:  2,
		Intent: "user login",
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(reranker.queries) != 1 || reranker.queries[0] != "user login\n\nalpha" {
		t.Fatalf("reranker queries = %#v, want qmd intent-prefixed query", reranker.queries)
	}
}

func TestQueryBestChunkPosUsesSourceOffsetLikeQMD(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	body := strings.Join([]string{
		"# Position",
		"opening opening opening opening opening opening opening opening opening opening",
		"## Later",
		"needlepos answer chunk with source offset.",
	}, "\n\n")
	if err := os.WriteFile(filepath.Join(docs, "position.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(Config{
		DBPath:       filepath.Join(dir, "wiki.db"),
		ChunkSize:    8,
		ChunkOverlap: 0,
	})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	if err := store.AddCollection(ctx, Collection{Name: "docs", Path: docs}); err != nil {
		t.Fatalf("AddCollection: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "docs", UpdateOptions{}); err != nil {
		t.Fatalf("UpdateCollection: %v", err)
	}

	result, err := store.Query(ctx, "docs", "needlepos", QueryOptions{Limit: 1, SkipRerank: true})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(result.Results) != 1 {
		t.Fatalf("results = %#v, want one position hit", result.Results)
	}
	hit := result.Results[0]
	if !strings.Contains(hit.BestChunk, "needlepos") {
		t.Fatalf("BestChunk = %q, want marker chunk", hit.BestChunk)
	}
	wantPos := strings.Index(body, hit.BestChunk)
	if wantPos <= 0 {
		t.Fatalf("could not locate BestChunk in source body at positive offset: pos=%d chunk=%q", wantPos, hit.BestChunk)
	}
	if hit.BestChunkPos != wantPos {
		t.Fatalf("BestChunkPos = %d, want source offset %d", hit.BestChunkPos, wantPos)
	}
}

func TestBestQueryChunkUsesIntentTermsLikeQMD(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	body := strings.Join([]string{
		"# Intent Chunk",
		"opening filler without the target term.",
		"## Recovery",
		"recoverytoken detailed recovery path.",
	}, "\n\n")
	if err := os.WriteFile(filepath.Join(docs, "intent.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(Config{
		DBPath:       filepath.Join(dir, "wiki.db"),
		ChunkSize:    8,
		ChunkOverlap: 0,
	})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	if err := store.AddCollection(ctx, Collection{Name: "docs", Path: docs}); err != nil {
		t.Fatalf("AddCollection: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "docs", UpdateOptions{}); err != nil {
		t.Fatalf("UpdateCollection: %v", err)
	}
	meta, err := store.getDocByCollectionPath("docs", "intent.md")
	if err != nil {
		t.Fatalf("getDocByCollectionPath: %v", err)
	}
	if meta == nil {
		t.Fatal("intent.md metadata missing")
	}

	chunk, _, _, err := store.bestQueryChunk(meta.id, "unmatched question", "recoverytoken", "")
	if err != nil {
		t.Fatalf("bestQueryChunk: %v", err)
	}
	if !strings.Contains(chunk, "recoverytoken") {
		t.Fatalf("BestChunk = %q, want intent term chunk", chunk)
	}
}

func TestQueryIntentTermsTrimUnicodePunctuationLikeQMD(t *testing.T) {
	score := queryChunkOverlapScore("missing question", "what about 【API】", "what about api route")
	if score != 1 {
		t.Fatalf("intent overlap score = %d, want Unicode punctuation trimmed and stop words ignored", score)
	}
}

func BenchmarkQueryQMDContract(b *testing.B) {
	ctx := context.Background()
	store := newReferenceQueryFixture(b, Config{
		ChunkSize: 50,
		Embedder:  queryContractEmbedder{},
		QueryExpander: &queryContractExpander{items: []QueryExpansion{
			{Type: QueryExpansionLex, Text: "lex-only"},
			{Type: QueryExpansionVec, Text: "semantic beta"},
		}},
	})
	defer store.Close()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := store.Query(ctx, "docs", "alpha", QueryOptions{Limit: 5, CandidateLimit: 10, SkipRerank: true})
		if err != nil {
			b.Fatalf("Query: %v", err)
		}
		if len(result.Results) == 0 {
			b.Fatal("Query returned no hits")
		}
	}
}

func TestQueryExpandsOnceAndRoutesTypedQueries(t *testing.T) {
	ctx := context.Background()
	expander := &queryContractExpander{items: []QueryExpansion{
		{Type: QueryExpansionVec, Text: "semantic beta"},
	}}
	store := newReferenceQueryFixture(t, Config{
		ChunkSize:     50,
		Embedder:      queryContractEmbedder{},
		QueryExpander: expander,
	})
	defer store.Close()

	result, err := store.Query(ctx, "docs", "alpha", QueryOptions{Limit: 5, SkipRerank: true})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if expander.calls != 1 {
		t.Fatalf("QueryExpander calls = %d, want exactly one query-layer expansion", expander.calls)
	}
	if indexOfPath(resultPaths(result.Results), "beta.md") < 0 {
		t.Fatalf("results = %#v, want vec expansion to route to vector search", resultPaths(result.Results))
	}
}

func TestPublicQMDPrimitiveMethods(t *testing.T) {
	ctx := context.Background()
	expander := &queryContractExpander{items: []QueryExpansion{
		{Type: QueryExpansionLex, Text: "lex-only"},
		{Type: QueryExpansionVec, Text: "semantic beta"},
	}}
	store := newReferenceQueryFixture(t, Config{
		ChunkSize:     50,
		Embedder:      queryContractEmbedder{},
		QueryExpander: expander,
	})
	defer store.Close()

	lexHits, err := store.SearchLex(ctx, "alpha", LexSearchOptions{Collection: "docs", Limit: 1})
	if err != nil {
		t.Fatalf("SearchLex: %v", err)
	}
	if len(lexHits) != 1 || lexHits[0].Path != "alpha.md" {
		t.Fatalf("SearchLex hits = %#v, want alpha.md", lexHits)
	}

	vecHits, err := store.SearchVector(ctx, "alpha", VectorSearchOptions{Collection: "docs", Limit: 1, MinScore: -1})
	if err != nil {
		t.Fatalf("SearchVector: %v", err)
	}
	if len(vecHits) != 1 || vecHits[0].Path == "beta.md" {
		t.Fatalf("SearchVector hits = %#v, want raw vector search without expansion-derived beta result", vecHits)
	}
	if expander.calls != 0 {
		t.Fatalf("QueryExpander calls = %d, want SearchLex/SearchVector to avoid expansion", expander.calls)
	}

	expanded, err := store.ExpandQuery(ctx, "alpha", ExpandQueryOptions{Intent: "manual test"})
	if err != nil {
		t.Fatalf("ExpandQuery: %v", err)
	}
	if expander.calls != 1 {
		t.Fatalf("QueryExpander calls = %d, want ExpandQuery to call configured expander once", expander.calls)
	}
	if len(expanded) != 2 || expanded[1].Type != QueryExpansionVec || expanded[1].Text != "semantic beta" {
		t.Fatalf("expanded = %#v, want configured typed queries", expanded)
	}
}

func TestExpandQueryPassesIntentToAwareExpander(t *testing.T) {
	ctx := context.Background()
	expander := &intentAwareQueryExpander{items: []QueryExpansion{
		{Type: QueryExpansionLex, Text: "auth middleware"},
	}}
	store := newReferenceQueryFixture(t, Config{
		ChunkSize:     50,
		Embedder:      queryContractEmbedder{},
		QueryExpander: expander,
	})
	defer store.Close()

	expanded, err := store.ExpandQuery(ctx, "auth flow", ExpandQueryOptions{Intent: "user login"})
	if err != nil {
		t.Fatalf("ExpandQuery: %v", err)
	}
	if len(expanded) != 1 || expanded[0].Text != "auth middleware" {
		t.Fatalf("expanded = %#v, want configured typed query", expanded)
	}
	if len(expander.intents) != 1 || expander.intents[0] != "user login" {
		t.Fatalf("expander intents = %#v, want user login", expander.intents)
	}
}

func TestExpandQueryFiltersOriginalTextLikeQMD(t *testing.T) {
	ctx := context.Background()
	expander := &queryContractExpander{items: []QueryExpansion{
		{Type: QueryExpansionLex, Text: "alpha"},
		{Type: QueryExpansionVec, Text: "alpha"},
		{Type: QueryExpansionLex, Text: "lex-only"},
	}}
	store := newReferenceQueryFixture(t, Config{
		ChunkSize:     50,
		Embedder:      queryContractEmbedder{},
		QueryExpander: expander,
	})
	defer store.Close()

	expanded, err := store.ExpandQuery(ctx, "alpha", ExpandQueryOptions{})
	if err != nil {
		t.Fatalf("ExpandQuery: %v", err)
	}
	if len(expanded) != 1 || expanded[0].Text != "lex-only" {
		t.Fatalf("expanded = %#v, want duplicate original query removed", expanded)
	}
}

func TestQueryPassesIntentToAwareExpander(t *testing.T) {
	ctx := context.Background()
	expander := &intentAwareQueryExpander{items: []QueryExpansion{
		{Type: QueryExpansionVec, Text: "semantic beta"},
	}}
	store := newReferenceQueryFixture(t, Config{
		ChunkSize:     50,
		Embedder:      queryContractEmbedder{},
		QueryExpander: expander,
	})
	defer store.Close()

	result, err := store.Query(ctx, "docs", "alpha", QueryOptions{
		Limit:      5,
		SkipRerank: true,
		Intent:     "user login",
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(expander.intents) != 1 || expander.intents[0] != "user login" {
		t.Fatalf("expander intents = %#v, want user login", expander.intents)
	}
	if indexOfPath(resultPaths(result.Results), "beta.md") < 0 {
		t.Fatalf("results = %#v, want intent-backed vec expansion to recall beta.md", resultPaths(result.Results))
	}
}

func TestQueryBatchEmbedsVectorQueriesLikeQMD(t *testing.T) {
	ctx := context.Background()
	embedder := &countingQueryEmbedder{}
	store := newReferenceQueryFixture(t, Config{
		ChunkSize: 50,
		Embedder:  embedder,
		QueryExpander: &queryContractExpander{items: []QueryExpansion{
			{Type: QueryExpansionLex, Text: "lex-only"},
			{Type: QueryExpansionVec, Text: "semantic beta"},
			{Type: QueryExpansionHyDE, Text: "beta-only hypothetical answer"},
		}},
	})
	defer store.Close()
	embedder.reset()

	if _, err := store.Query(ctx, "docs", "alpha", QueryOptions{Limit: 5, SkipRerank: true}); err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(embedder.batches) != 1 {
		t.Fatalf("embed batches = %#v, want one batch for original vec/hyde queries", embedder.batches)
	}
	got := strings.Join(embedder.batches[0], "\n")
	for _, want := range []string{"alpha", "semantic beta", "beta-only hypothetical answer"} {
		if !strings.Contains(got, want) {
			t.Fatalf("embed batch = %#v, want %q", embedder.batches[0], want)
		}
	}
	if len(embedder.texts) != 0 {
		t.Fatalf("single embeds = %#v, want Query vector path to use batch embedder", embedder.texts)
	}
}

func TestQueryAcceptsPreExpandedQueriesWithoutAutoExpansion(t *testing.T) {
	ctx := context.Background()
	expander := &queryContractExpander{items: []QueryExpansion{
		{Type: QueryExpansionLex, Text: "lex-only"},
	}}
	store := newReferenceQueryFixture(t, Config{
		ChunkSize:     50,
		Embedder:      queryContractEmbedder{},
		QueryExpander: expander,
	})
	defer store.Close()

	result, err := store.Query(ctx, "docs", "alpha", QueryOptions{
		Limit:      5,
		SkipRerank: true,
		Queries: []QueryExpansion{
			{Type: QueryExpansionVec, Query: "semantic beta"},
		},
	})
	if err != nil {
		t.Fatalf("Query with pre-expanded queries: %v", err)
	}
	if expander.calls != 0 {
		t.Fatalf("QueryExpander calls = %d, want pre-expanded queries to skip auto expansion", expander.calls)
	}
	if indexOfPath(resultPaths(result.Results), "beta.md") < 0 {
		t.Fatalf("results = %#v, want structured vec query to recall beta.md", resultPaths(result.Results))
	}
}

func TestQueryStructuredQueriesBatchEmbedLikeQMD(t *testing.T) {
	ctx := context.Background()
	embedder := &countingQueryEmbedder{}
	store := newReferenceQueryFixture(t, Config{
		ChunkSize: 50,
		Embedder:  embedder,
	})
	defer store.Close()
	embedder.reset()

	_, err := store.Query(ctx, "docs", "", QueryOptions{
		Limit:      5,
		SkipRerank: true,
		Queries: []QueryExpansion{
			{Type: QueryExpansionVec, Query: "semantic beta"},
			{Type: QueryExpansionHyDE, Query: "beta-only hypothetical answer"},
		},
	})
	if err != nil {
		t.Fatalf("Query with structured queries: %v", err)
	}
	if len(embedder.batches) != 1 {
		t.Fatalf("embed batches = %#v, want one batch for structured vec/hyde queries", embedder.batches)
	}
	got := strings.Join(embedder.batches[0], "\n")
	if !strings.Contains(got, "semantic beta") || !strings.Contains(got, "beta-only hypothetical answer") {
		t.Fatalf("embed batch = %#v, want both structured semantic queries", embedder.batches[0])
	}
	if len(embedder.texts) != 0 {
		t.Fatalf("single embeds = %#v, want structured vector path to use batch embedder", embedder.texts)
	}
}

func TestQuerySkipsVectorSearchWhenNoEmbeddingsLikeQMD(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "alpha.md"), []byte("# Alpha\n\nalpha exact retrieval chunk.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	embedder := &countingQueryEmbedder{}
	store, err := NewStore(Config{
		DBPath:   filepath.Join(dir, "wiki.db"),
		Embedder: embedder,
	})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	if err := store.AddCollection(ctx, Collection{Name: "docs", Path: docs, Include: []string{"**/*.md"}}); err != nil {
		t.Fatalf("AddCollection: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "docs", UpdateOptions{}); err != nil {
		t.Fatalf("UpdateCollection: %v", err)
	}

	result, err := store.Query(ctx, "docs", "alpha", QueryOptions{Limit: 5, SkipRerank: true})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(result.Results) == 0 || result.Results[0].Path != "alpha.md" {
		t.Fatalf("results = %#v, want FTS result without embeddings", result.Results)
	}
	if len(embedder.texts) != 0 || len(embedder.batches) != 0 {
		t.Fatalf("embed calls texts=%#v batches=%#v, want vector path skipped when no embeddings exist", embedder.texts, embedder.batches)
	}
}

func TestQueryRejectsStructuredQueryNewlineLikeQMD(t *testing.T) {
	store := newReferenceQueryFixture(t, Config{ChunkSize: 50, Embedder: queryContractEmbedder{}})
	defer store.Close()

	_, err := store.Query(context.Background(), "docs", "", QueryOptions{
		Queries: []QueryExpansion{{Type: QueryExpansionLex, Query: "alpha\nbeta", Line: 7}},
	})
	if err == nil || !strings.Contains(err.Error(), "single-line") || !strings.Contains(err.Error(), "Line 7") {
		t.Fatalf("Query error = %v, want line-aware single-line validation", err)
	}
}

func TestQueryRejectsStructuredLexUnmatchedQuoteLikeQMD(t *testing.T) {
	store := newReferenceQueryFixture(t, Config{ChunkSize: 50, Embedder: queryContractEmbedder{}})
	defer store.Close()

	_, err := store.Query(context.Background(), "docs", "", QueryOptions{
		Queries: []QueryExpansion{{Type: QueryExpansionLex, Query: `"alpha`}},
	})
	if err == nil || !strings.Contains(err.Error(), "unmatched double quote") {
		t.Fatalf("Query error = %v, want unmatched quote validation", err)
	}
}

func TestQueryRejectsStructuredSemanticNegationLikeQMD(t *testing.T) {
	store := newReferenceQueryFixture(t, Config{ChunkSize: 50, Embedder: queryContractEmbedder{}})
	defer store.Close()

	_, err := store.Query(context.Background(), "docs", "", QueryOptions{
		Queries: []QueryExpansion{{Type: QueryExpansionVec, Query: "alpha -beta"}},
	})
	if err == nil || !strings.Contains(err.Error(), "Negation") {
		t.Fatalf("Query error = %v, want semantic negation validation", err)
	}
}

func TestQueryAcceptsIndependentSearchQueries(t *testing.T) {
	ctx := context.Background()
	expander := &queryContractExpander{items: []QueryExpansion{
		{Type: QueryExpansionLex, Text: "unused-expansion"},
	}}
	embedder := &countingQueryEmbedder{}
	store := newReferenceQueryFixture(t, Config{
		ChunkSize:     50,
		Embedder:      embedder,
		QueryExpander: expander,
	})
	defer store.Close()
	embedder.reset()

	result, err := store.Query(ctx, "docs", "combined retrieval", QueryOptions{
		Limit:         5,
		SkipRerank:    true,
		Explain:       true,
		SearchQueries: []string{"alpha", "lex-only"},
	})
	if err != nil {
		t.Fatalf("Query with independent search queries: %v", err)
	}
	if expander.calls != 0 {
		t.Fatalf("QueryExpander calls = %d, want search queries to skip auto expansion", expander.calls)
	}
	if len(embedder.batches) != 1 || len(embedder.batches[0]) != 1 || !strings.Contains(embedder.batches[0][0], "combined retrieval") {
		t.Fatalf("embed batches = %#v, want only the primary question embedded once", embedder.batches)
	}
	if len(embedder.texts) != 0 {
		t.Fatalf("single embeds = %#v, want search query vector path to use batch embedder", embedder.texts)
	}
	paths := resultPaths(result.Results)
	if indexOfPath(paths, "alpha.md") < 0 || indexOfPath(paths, "lex.md") < 0 {
		t.Fatalf("paths = %#v, want results from both independent search queries", paths)
	}
	lex := findResultPath(t, result.Results, "lex.md")
	if lex.Explain == nil || len(lex.Explain.RRF.Contributions) == 0 {
		t.Fatalf("lex explain = %#v, want RRF contributions", lex.Explain)
	}
	if !hasContributionQuery(lex.Explain.RRF.Contributions, "lex-only") {
		t.Fatalf("lex contributions = %#v, want lex-only contribution", lex.Explain.RRF.Contributions)
	}
}

func newReferenceQueryFixture(t testingFatalHelper, cfg Config) *Store {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(filepath.Join(docs, "topics"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	files := map[string]string{
		"alpha.md":        "# Alpha\n\nintro chunk without the term.\n\n## Details\n\nalpha exact retrieval chunk.\n",
		"lex.md":          "# Lex\n\nlex-only expansion document.\n",
		"beta.md":         "# Beta\n\nbeta-only vector document.\n",
		"rerank.md":       "# Rerank\n\nalpha rerank candidate.\n",
		"topics/noise.md": "# Noise\n\nunrelated content.\n",
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
	cfg.DBPath = filepath.Join(dir, "wiki.db")
	store, err := NewStore(cfg)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.AddCollection(ctx, Collection{
		Name:    "docs",
		Path:    docs,
		Include: []string{"**/*.md"},
		Context: map[string]string{
			"/":        "Docs context",
			"alpha.md": "Alpha context",
		},
	}); err != nil {
		t.Fatalf("AddCollection: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "docs", UpdateOptions{}); err != nil {
		t.Fatalf("UpdateCollection: %v", err)
	}
	if _, err := store.EmbedCollection(ctx, "docs", EmbedOptions{}); err != nil {
		t.Fatalf("EmbedCollection: %v", err)
	}
	return store
}

func resultPaths(results []SearchResult) []string {
	paths := make([]string, 0, len(results))
	for _, result := range results {
		paths = append(paths, result.Path)
	}
	return paths
}

func indexOfPath(paths []string, want string) int {
	for i, path := range paths {
		if path == want {
			return i
		}
	}
	return -1
}

func findResultPath(t *testing.T, results []SearchResult, path string) SearchResult {
	t.Helper()
	for _, result := range results {
		if result.Path == path {
			return result
		}
	}
	t.Fatalf("result path %s not found in %#v", path, resultPaths(results))
	return SearchResult{}
}

func hasOriginalDoubleWeight(contributions []QueryContributionTrace) bool {
	for _, contribution := range contributions {
		if contribution.QueryType == "original" && contribution.Weight == 2.0 {
			return true
		}
	}
	return false
}

func hasContributionQuery(contributions []QueryContributionTrace, query string) bool {
	for _, contribution := range contributions {
		if contribution.Query == query {
			return true
		}
	}
	return false
}
