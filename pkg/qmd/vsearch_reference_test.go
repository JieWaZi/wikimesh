package qmd_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

import qmd "github.com/JieWaZi/wikimesh/pkg/qmd"

type qmdContractEmbedder struct{}

func (qmdContractEmbedder) Name() string { return "qmd-contract/fake-embedding" }

func (qmdContractEmbedder) Dimensions() int { return 2 }

func (qmdContractEmbedder) Embed(text string) ([]float32, error) {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "shared expansion"):
		return []float32{0, 1}, nil
	case strings.Contains(lower, "strong-target"):
		return []float32{1, 0}, nil
	case strings.Contains(lower, "weak-shared"):
		return []float32{0.2, 0.98}, nil
	case strings.Contains(lower, "filler-shared"):
		return []float32{0, 1}, nil
	case strings.Contains(lower, "target"):
		return []float32{1, 0}, nil
	default:
		return []float32{0, 0}, nil
	}
}

func TestVSearchKeepsBestChunkPerDocument(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(docs, "multi.md"), []byte(strings.Join([]string{
		"weak-shared passage that appears in several expanded searches",
		"",
		"strong-target passage that is the best chunk for the original query",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		name := filepath.Join(docs, "filler-"+string(rune('a'+i))+".md")
		if err := os.WriteFile(name, []byte("filler-shared passage for expansion ranking\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	store, err := qmd.NewStore(qmd.Config{
		DBPath:    filepath.Join(dir, "wiki.db"),
		ChunkSize: 4,
		Embedder:  qmdContractEmbedder{},
		QueryExpander: fakeQueryExpander{items: []qmd.QueryExpansion{
			{Type: qmd.QueryExpansionVec, Text: "shared expansion"},
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
	if _, err := store.EmbedCollection(ctx, "docs", qmd.EmbedOptions{}); err != nil {
		t.Fatalf("EmbedCollection: %v", err)
	}

	hits, err := store.VSearch(ctx, "docs", "target", qmd.SearchOptions{Limit: 10, MinScore: -1})
	if err != nil {
		t.Fatalf("VSearch: %v", err)
	}
	var multi *qmd.SearchResult
	for i := range hits {
		if hits[i].Path == "multi.md" {
			multi = &hits[i]
			break
		}
	}
	if multi == nil {
		t.Fatalf("hits = %#v, want multi.md present", hits)
	}
	if !strings.Contains(multi.Snippet, "strong-target") {
		t.Fatalf("multi.md snippet = %q, want best cosine chunk strong-target", multi.Snippet)
	}
	if multi.Score < 0.99 {
		t.Fatalf("multi.md score = %.6f, want best chunk cosine near 1.0", multi.Score)
	}
}

type qmdVariantMergeEmbedder struct{}

func (qmdVariantMergeEmbedder) Name() string { return "qmd-contract/variant-merge" }

func (qmdVariantMergeEmbedder) Dimensions() int { return 2 }

func (qmdVariantMergeEmbedder) Embed(text string) ([]float32, error) {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "expansion-only"):
		return []float32{0.6, 0.8}, nil
	case strings.Contains(lower, "expansion-noise"):
		return []float32{0, 1}, nil
	case strings.Contains(lower, "target-best"):
		return []float32{1, 0}, nil
	case strings.Contains(lower, "target"):
		return []float32{1, 0}, nil
	default:
		return []float32{0, 0}, nil
	}
}

func TestVSearchMergesQueryVariantsByMaxScore(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(docs, "target.md"), []byte("target-best exact answer\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		name := filepath.Join(docs, "noise-"+string(rune('a'+i))+".md")
		if err := os.WriteFile(name, []byte("expansion-noise semantic neighbor\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	store, err := qmd.NewStore(qmd.Config{
		DBPath:    filepath.Join(dir, "wiki.db"),
		ChunkSize: 20,
		Embedder:  qmdVariantMergeEmbedder{},
		QueryExpander: fakeQueryExpander{items: []qmd.QueryExpansion{
			{Type: qmd.QueryExpansionVec, Text: "expansion-only"},
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
	if _, err := store.EmbedCollection(ctx, "docs", qmd.EmbedOptions{}); err != nil {
		t.Fatalf("EmbedCollection: %v", err)
	}

	hits, err := store.VSearch(ctx, "docs", "target", qmd.SearchOptions{Limit: 3, MinScore: -1})
	if err != nil {
		t.Fatalf("VSearch: %v", err)
	}
	if len(hits) == 0 || hits[0].Path != "target.md" {
		t.Fatalf("hits = %#v, want max-score merge to keep target.md first", hits)
	}
	if hits[0].Score < 0.99 {
		t.Fatalf("target score = %.6f, want original-query cosine score preserved", hits[0].Score)
	}
}

type qmdCollectionEmbedder struct{}

func (qmdCollectionEmbedder) Name() string { return "qmd-contract/collection-filter" }

func (qmdCollectionEmbedder) Dimensions() int { return 2 }

func (qmdCollectionEmbedder) Embed(text string) ([]float32, error) {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "other-high"):
		return []float32{1, 0}, nil
	case strings.Contains(lower, "target-medium"):
		return []float32{0.8, 0.6}, nil
	case strings.Contains(lower, "needle"):
		return []float32{1, 0}, nil
	default:
		return []float32{0, 1}, nil
	}
}

func TestVSearchAppliesCollectionFilterBeforeFinalLimit(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	other := filepath.Join(dir, "other")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "target.md"), []byte("target-medium answer\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		name := filepath.Join(other, "other-"+string(rune('a'+i))+".md")
		if err := os.WriteFile(name, []byte("other-high unrelated collection\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	store, err := qmd.NewStore(qmd.Config{
		DBPath:    filepath.Join(dir, "wiki.db"),
		ChunkSize: 20,
		Embedder:  qmdCollectionEmbedder{},
	})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	if err := store.AddCollection(ctx, qmd.Collection{Name: "docs", Path: docs, Include: []string{"**/*.md"}}); err != nil {
		t.Fatalf("AddCollection docs: %v", err)
	}
	if err := store.AddCollection(ctx, qmd.Collection{Name: "other", Path: other, Include: []string{"**/*.md"}}); err != nil {
		t.Fatalf("AddCollection other: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "docs", qmd.UpdateOptions{}); err != nil {
		t.Fatalf("UpdateCollection docs: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "other", qmd.UpdateOptions{}); err != nil {
		t.Fatalf("UpdateCollection other: %v", err)
	}
	if _, err := store.EmbedCollection(ctx, "docs", qmd.EmbedOptions{}); err != nil {
		t.Fatalf("EmbedCollection docs: %v", err)
	}
	if _, err := store.EmbedCollection(ctx, "other", qmd.EmbedOptions{}); err != nil {
		t.Fatalf("EmbedCollection other: %v", err)
	}

	hits, err := store.VSearch(ctx, "docs", "needle", qmd.SearchOptions{Limit: 1, MinScore: -1})
	if err != nil {
		t.Fatalf("VSearch: %v", err)
	}
	if len(hits) != 1 || hits[0].Path != "target.md" {
		t.Fatalf("hits = %#v, want target collection result before final limit is applied", hits)
	}
}

type qmdBenchmarkEmbedder struct{}

func (qmdBenchmarkEmbedder) Name() string { return "qmd-contract/benchmark" }

func (qmdBenchmarkEmbedder) Dimensions() int { return 2 }

func (qmdBenchmarkEmbedder) Embed(text string) ([]float32, error) {
	lower := strings.ToLower(text)
	if strings.Contains(lower, "needle") || strings.Contains(lower, "target") {
		return []float32{1, 0}, nil
	}
	return []float32{0, 1}, nil
}

func BenchmarkVSearchQMDContract(b *testing.B) {
	ctx := context.Background()
	dir := b.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		b.Fatal(err)
	}
	for i := 0; i < 200; i++ {
		body := "background document for vector benchmark\n"
		if i%20 == 0 {
			body = "target needle document for vector benchmark\n"
		}
		name := filepath.Join(docs, fmt.Sprintf("doc-%03d.md", i))
		if err := os.WriteFile(name, []byte(body), 0o644); err != nil {
			b.Fatal(err)
		}
	}

	store, err := qmd.NewStore(qmd.Config{
		DBPath:    filepath.Join(dir, "wiki.db"),
		ChunkSize: 20,
		Embedder:  qmdBenchmarkEmbedder{},
	})
	if err != nil {
		b.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	if err := store.AddCollection(ctx, qmd.Collection{Name: "docs", Path: docs, Include: []string{"**/*.md"}}); err != nil {
		b.Fatalf("AddCollection: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "docs", qmd.UpdateOptions{}); err != nil {
		b.Fatalf("UpdateCollection: %v", err)
	}
	if _, err := store.EmbedCollection(ctx, "docs", qmd.EmbedOptions{}); err != nil {
		b.Fatalf("EmbedCollection: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hits, err := store.VSearch(ctx, "docs", "needle", qmd.SearchOptions{Limit: 10})
		if err != nil {
			b.Fatalf("VSearch: %v", err)
		}
		if len(hits) == 0 {
			b.Fatal("VSearch returned no hits")
		}
	}
}
