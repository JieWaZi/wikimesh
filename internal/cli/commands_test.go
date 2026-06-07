package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	qmd "github.com/JieWaZi/wikimesh/pkg/qmd"
)

func TestCobraHelpCommands(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "root",
			args: []string{"--help"},
			want: []string{"Usage:", "wikimesh", "collection", "embed"},
		},
		{
			name: "collection",
			args: []string{"collection", "--help"},
			want: []string{"Usage:", "collection", "add", "remove", "update"},
		},
		{
			name: "collection add",
			args: []string{"collection", "add", "--help"},
			want: []string{"Usage:", "add [path]", "--name", "--mask"},
		},
		{
			name: "model lib install",
			args: []string{"model", "lib", "install", "--help"},
			want: []string{"Usage:", "install", "--processor", "--lib"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := runCLI(t, context.Background(), tc.args)
			for _, want := range tc.want {
				if !strings.Contains(output, want) {
					t.Fatalf("help output missing %q:\n%s", want, output)
				}
			}
		})
	}
}

func TestCollectionHelpHidesSearchAndConfigEditingCommands(t *testing.T) {
	output := runCLI(t, context.Background(), []string{"collection", "--help"})
	for _, hidden := range []string{"search", "vsearch", "query", "show", "rename", "include", "exclude", "update-cmd"} {
		if strings.Contains(output, hidden) {
			t.Fatalf("collection help contains removed command %q:\n%s", hidden, output)
		}
	}
}

func TestCollectionCLIQMDManagementCommands(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "wikimesh.yaml")
	dbPath := filepath.Join(dir, "wiki.db")
	if err := os.WriteFile(configPath, []byte("db_path: "+dbPath+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Run(ctx, []string{"--config", configPath, "collection", "add", docs, "--name", "docs", "--mask", "**/*.md"}, io.Discard, io.Discard); err != nil {
		t.Fatalf("collection add: %v", err)
	}
	assertCollectionNames(t, dbPath, []string{"docs"})

	cfg, err := qmd.LoadConfigFile(configPath)
	if err != nil {
		t.Fatalf("LoadConfigFile after add: %v", err)
	}
	if len(cfg.Collections) != 1 || cfg.Collections[0].Name != "docs" {
		t.Fatalf("config after add = %#v, want docs collection", cfg.Collections)
	}

	if err := Run(ctx, []string{"--config", configPath, "collection", "remove", "docs"}, io.Discard, io.Discard); err != nil {
		t.Fatalf("collection remove: %v", err)
	}
	assertCollectionNames(t, dbPath, nil)
	cfg, err = qmd.LoadConfigFile(configPath)
	if err != nil {
		t.Fatalf("LoadConfigFile after remove: %v", err)
	}
	if len(cfg.Collections) != 0 {
		t.Fatalf("config after remove = %#v, want no collections", cfg.Collections)
	}
}

func TestCobraConfigShorthand(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "wikimesh.yaml")
	dbPath := filepath.Join(dir, "wiki.db")
	if err := os.WriteFile(configPath, []byte("db_path: "+dbPath+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Run(ctx, []string{"-c", configPath, "collection", "add", docs, "--name", "docs"}, io.Discard, io.Discard); err != nil {
		t.Fatalf("collection add with -c: %v", err)
	}
	assertCollectionNames(t, dbPath, []string{"docs"})
}

func TestDefaultConfigIsGeneratedUnderDotWikimeshWithDefaultModels(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	t.Chdir(dir)

	output := runCLI(t, ctx, []string{"collection", "list"})
	if !strings.Contains(output, "[]") {
		t.Fatalf("collection list output = %q, want empty JSON list", output)
	}

	configPath := filepath.Join(dir, ".wikimesh", "wikimesh.yaml")
	cfg, err := qmd.LoadConfigFile(configPath)
	if err != nil {
		t.Fatalf("LoadConfigFile generated default: %v", err)
	}
	if cfg.DBPath != ".wikimesh/wiki.db" {
		t.Fatalf("DBPath = %q, want .wikimesh/wiki.db", cfg.DBPath)
	}
	if cfg.Models.Embed != "hf:Qwen/Qwen3-Embedding-0.6B-GGUF/Qwen3-Embedding-0.6B-Q8_0.gguf" {
		t.Fatalf("models.embed = %q", cfg.Models.Embed)
	}
	if cfg.Models.Rerank != "hf:ggml-org/Qwen3-Reranker-0.6B-Q8_0-GGUF/qwen3-reranker-0.6b-q8_0.gguf" {
		t.Fatalf("models.rerank = %q", cfg.Models.Rerank)
	}
	if cfg.Models.Generate != "hf:tobil/qmd-query-expansion-1.7B-gguf/qmd-query-expansion-1.7B-q4_k_m.gguf" {
		t.Fatalf("models.generate = %q", cfg.Models.Generate)
	}
	if cfg.Embedding.Provider != "local" || cfg.Embedding.Model != ".wikimesh/models/Qwen3-Embedding-0.6B-Q8_0.gguf" {
		t.Fatalf("embedding config = %#v, want local default embed model", cfg.Embedding)
	}
	if cfg.Embedding.Command != "" || cfg.Embedding.Dimensions != 0 || cfg.Embedding.RateLimit != 0 {
		t.Fatalf("embedding defaults = %#v, want quiet generated config", cfg.Embedding)
	}
	if cfg.QueryExpansion.Provider != "local" || cfg.QueryExpansion.Model != ".wikimesh/models/qmd-query-expansion-1.7B-q4_k_m.gguf" {
		t.Fatalf("query expansion config = %#v, want local default generate model", cfg.QueryExpansion)
	}
	if cfg.QueryExpansion.Command != "" || cfg.QueryExpansion.MaxTokens != 0 {
		t.Fatalf("query expansion defaults = %#v, want quiet generated config", cfg.QueryExpansion)
	}
	if cfg.Reranker.Provider != "local" || cfg.Reranker.Model != ".wikimesh/models/qwen3-reranker-0.6b-q8_0.gguf" {
		t.Fatalf("reranker config = %#v, want local default rerank model", cfg.Reranker)
	}
	if cfg.Reranker.Command != "" || cfg.Reranker.MaxTokens != 0 {
		t.Fatalf("reranker defaults = %#v, want quiet generated config", cfg.Reranker)
	}
}

func TestModelDownloadCommandDownloadsConfiguredHFModels(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	serverFile := filepath.Join(dir, "remote.gguf")
	if err := os.WriteFile(serverFile, []byte("GGUF fake model"), 0o644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "wikimesh.yaml")
	cfg := qmd.DefaultFileConfig()
	cfg.Models.Embed = "file://" + serverFile
	cfg.Models.Rerank = ""
	cfg.Models.Generate = ""
	cfg.Embedding.Model = filepath.Join(dir, ".wikimesh", "models", "remote.gguf")
	if err := qmd.SaveConfigFile(configPath, cfg); err != nil {
		t.Fatal(err)
	}

	output := runCLI(t, ctx, []string{"--config", configPath, "model", "download", "embed"})
	if !strings.Contains(output, "Downloading:") {
		t.Fatalf("model download output = %q, want start message", output)
	}
	if !strings.Contains(output, "Downloaded:") {
		t.Fatalf("model download output = %q, want download summary", output)
	}
	if data, err := os.ReadFile(cfg.Embedding.Model); err != nil || string(data) != "GGUF fake model" {
		t.Fatalf("downloaded model data=%q err=%v", string(data), err)
	}
}

func TestCollectionCLIAddDefaultsNameFromPath(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "project-docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "wikimesh.yaml")
	dbPath := filepath.Join(dir, "wiki.db")
	if err := os.WriteFile(configPath, []byte("db_path: "+dbPath+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Run(ctx, []string{"--config", configPath, "collection", "add", docs}, io.Discard, io.Discard); err != nil {
		t.Fatalf("collection add: %v", err)
	}
	assertCollectionNames(t, dbPath, []string{"project-docs"})
	cfg, err := qmd.LoadConfigFile(configPath)
	if err != nil {
		t.Fatalf("LoadConfigFile: %v", err)
	}
	if len(cfg.Collections) != 1 || cfg.Collections[0].Name != "project-docs" || cfg.Collections[0].Pattern != "**/*.md" {
		t.Fatalf("config collections = %#v, want basename and default pattern", cfg.Collections)
	}
}

func TestTopLevelSearchUsesQMDJSONDefaultsAndDefaultCollections(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "wikimesh.yaml")
	dbPath := filepath.Join(dir, "wiki.db")
	if err := os.WriteFile(configPath, []byte("db_path: "+dbPath+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	docs := filepath.Join(dir, "docs")
	archive := filepath.Join(dir, "archive")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(archive, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 25; i++ {
		if err := os.WriteFile(filepath.Join(docs, "alpha-"+string(rune('a'+i))+".md"), []byte("# Alpha\n\nAlpha default query result.\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(archive, "alpha-archive.md"), []byte("# Alpha\n\nAlpha excluded archive result.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Run(ctx, []string{"--config", configPath, "collection", "add", docs, "--name", "docs"}, io.Discard, io.Discard); err != nil {
		t.Fatalf("collection add docs: %v", err)
	}
	if err := Run(ctx, []string{"--config", configPath, "collection", "add", archive, "--name", "archive"}, io.Discard, io.Discard); err != nil {
		t.Fatalf("collection add archive: %v", err)
	}
	excludeCollectionInConfig(t, configPath, "archive")
	if err := Run(ctx, []string{"--config", configPath, "collection", "update", "docs"}, io.Discard, io.Discard); err != nil {
		t.Fatalf("collection update docs: %v", err)
	}
	if err := Run(ctx, []string{"--config", configPath, "collection", "update", "archive"}, io.Discard, io.Discard); err != nil {
		t.Fatalf("collection update archive: %v", err)
	}

	output := runCLI(t, ctx, []string{"--config", configPath, "search", "alpha"})
	var hits []qmd.SearchResult
	if err := json.Unmarshal([]byte(output), &hits); err != nil {
		t.Fatalf("search JSON: %v\n%s", err, output)
	}
	if len(hits) != 20 {
		t.Fatalf("top-level search returned %d hits, want qmd JSON default 20", len(hits))
	}
	for _, hit := range hits {
		if hit.Collection != "docs" {
			t.Fatalf("top-level search hit collection = %q, want only default docs: %#v", hit.Collection, hits)
		}
	}

	output = runCLI(t, ctx, []string{"--config", configPath, "search", "-c", "archive", "alpha"})
	if err := json.Unmarshal([]byte(output), &hits); err != nil {
		t.Fatalf("search -c JSON: %v\n%s", err, output)
	}
	if len(hits) != 1 || hits[0].Collection != "archive" {
		t.Fatalf("top-level search -c hits = %#v, want only archive hit", hits)
	}
}

func TestTopLevelSearchEmptyDefaultCollectionsSearchesAllLikeQMD(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "wikimesh.yaml")
	dbPath := filepath.Join(dir, "wiki.db")
	if err := os.WriteFile(configPath, []byte("db_path: "+dbPath+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "alpha.md"), []byte("# Alpha\n\nAlpha still searchable when defaults are empty.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Run(ctx, []string{"--config", configPath, "collection", "add", docs, "--name", "docs"}, io.Discard, io.Discard); err != nil {
		t.Fatalf("collection add docs: %v", err)
	}
	excludeCollectionInConfig(t, configPath, "docs")
	if err := Run(ctx, []string{"--config", configPath, "collection", "update", "docs"}, io.Discard, io.Discard); err != nil {
		t.Fatalf("collection update docs: %v", err)
	}

	output := runCLI(t, ctx, []string{"--config", configPath, "search", "alpha"})
	var hits []qmd.SearchResult
	if err := json.Unmarshal([]byte(output), &hits); err != nil {
		t.Fatalf("search JSON: %v\n%s", err, output)
	}
	if len(hits) != 1 || hits[0].Collection != "docs" {
		t.Fatalf("top-level search with empty defaults hits = %#v, want global docs hit", hits)
	}
}

func TestTopLevelVSearchUsesQMDJSONDefaultLimit(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "wikimesh.yaml")
	dbPath := filepath.Join(dir, "wiki.db")
	modelPath := filepath.Join(dir, "fake.gguf")
	if err := os.WriteFile(modelPath, []byte("fake model"), 0o644); err != nil {
		t.Fatal(err)
	}
	fakeBin := filepath.Join(dir, "llama-embedding")
	if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\nprintf '{\"embedding\":[1,0]}'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+oldPath)
	config := "db_path: " + dbPath + "\nembedding:\n  provider: local\n  model: " + modelPath + "\n  command: " + fakeBin + "\n  dimensions: 2\n"
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 25; i++ {
		if err := os.WriteFile(filepath.Join(docs, "alpha-"+string(rune('a'+i))+".md"), []byte("# Alpha\n\nAlpha vector result.\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := Run(ctx, []string{"--config", configPath, "collection", "add", docs, "--name", "docs"}, io.Discard, io.Discard); err != nil {
		t.Fatalf("collection add docs: %v", err)
	}
	if err := Run(ctx, []string{"--config", configPath, "collection", "update", "docs"}, io.Discard, io.Discard); err != nil {
		t.Fatalf("collection update docs: %v", err)
	}
	if err := Run(ctx, []string{"--config", configPath, "embed"}, io.Discard, io.Discard); err != nil {
		t.Fatalf("embed docs: %v", err)
	}

	output := runCLI(t, ctx, []string{"--config", configPath, "vsearch", "alpha"})
	var hits []qmd.SearchResult
	if err := json.Unmarshal([]byte(output), &hits); err != nil {
		t.Fatalf("vsearch JSON: %v\n%s", err, output)
	}
	if len(hits) != 20 {
		t.Fatalf("top-level vsearch returned %d hits, want qmd JSON default 20", len(hits))
	}
}

func TestTopLevelEmbedGeneratesVectorsFromIndexedDocuments(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "wikimesh.yaml")
	dbPath := filepath.Join(dir, "wiki.db")
	modelPath := filepath.Join(dir, "fake.gguf")
	if err := os.WriteFile(modelPath, []byte("fake model"), 0o644); err != nil {
		t.Fatal(err)
	}
	fakeBin := filepath.Join(dir, "llama-embedding")
	if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\nprintf '{\"embedding\":[1,0]}'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := "db_path: " + dbPath + "\nembedding:\n  provider: none\ncollections:\n  docs:\n    path: " + filepath.Join(dir, "docs") + "\n    pattern: '**/*.md'\n"
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "alpha.md"), []byte("# Alpha\n\nAlpha vector result.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	updateOut := runCLI(t, ctx, []string{"--config", configPath, "collection", "update", "docs"})
	if !strings.Contains(updateOut, "Indexed: 1 new") || !strings.Contains(updateOut, "Run 'wikimesh embed'") {
		t.Fatalf("update output = %q, want qmd-style summary and embed hint", updateOut)
	}

	output := runCLI(t, ctx, []string{
		"--config", configPath,
		"embed",
		"--provider", "local",
		"--model", modelPath,
		"--command", fakeBin,
		"--dimensions", "2",
	})
	for _, want := range []string{"Model: local/", "Embedded:", "chunks", "All embeddings updated."} {
		if !strings.Contains(output, want) {
			t.Fatalf("embed output missing %q:\n%s", want, output)
		}
	}
}

func TestTopLevelQueryUsesDefaultCollections(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "wikimesh.yaml")
	dbPath := filepath.Join(dir, "wiki.db")
	if err := os.WriteFile(configPath, []byte("db_path: "+dbPath+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	docs := filepath.Join(dir, "docs")
	archive := filepath.Join(dir, "archive")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(archive, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "alpha.md"), []byte("# Alpha\n\nAlpha default query result.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(archive, "alpha.md"), []byte("# Alpha\n\nAlpha archive query result.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Run(ctx, []string{"--config", configPath, "collection", "add", docs, "--name", "docs"}, io.Discard, io.Discard); err != nil {
		t.Fatalf("collection add docs: %v", err)
	}
	if err := Run(ctx, []string{"--config", configPath, "collection", "add", archive, "--name", "archive"}, io.Discard, io.Discard); err != nil {
		t.Fatalf("collection add archive: %v", err)
	}
	excludeCollectionInConfig(t, configPath, "archive")
	if err := Run(ctx, []string{"--config", configPath, "collection", "update", "docs"}, io.Discard, io.Discard); err != nil {
		t.Fatalf("collection update docs: %v", err)
	}
	if err := Run(ctx, []string{"--config", configPath, "collection", "update", "archive"}, io.Discard, io.Discard); err != nil {
		t.Fatalf("collection update archive: %v", err)
	}

	output := runCLI(t, ctx, []string{"--config", configPath, "query", "alpha"})
	var result qmd.QueryResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("query JSON: %v\n%s", err, output)
	}
	if len(result.Results) == 0 {
		t.Fatalf("top-level query returned no results")
	}
	for _, hit := range result.Results {
		if hit.Collection != "docs" {
			t.Fatalf("top-level query hit collection = %q, want only default docs: %#v", hit.Collection, result.Results)
		}
	}
}

func TestTopLevelQueryAcceptsPreExpandedQueriesAndOptions(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "wikimesh.yaml")
	dbPath := filepath.Join(dir, "wiki.db")
	if err := os.WriteFile(configPath, []byte("db_path: "+dbPath+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "manual.md"), []byte("# Manual\n\nmanual-only structured query result.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "noise.md"), []byte("# Noise\n\nordinary alpha result.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Run(ctx, []string{"--config", configPath, "collection", "add", docs, "--name", "docs"}, io.Discard, io.Discard); err != nil {
		t.Fatalf("collection add docs: %v", err)
	}
	if err := Run(ctx, []string{"--config", configPath, "collection", "update", "docs"}, io.Discard, io.Discard); err != nil {
		t.Fatalf("collection update docs: %v", err)
	}

	output := runCLI(t, ctx, []string{
		"--config", configPath,
		"query",
		"--queries", `[{"type":"lex","query":"manual-only"}]`,
		"--candidate-limit", "3",
		"--no-rerank",
		"--explain",
		"--intent", "manual structured lookup",
		"alpha",
	})
	var result qmd.QueryResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("query --queries JSON: %v\n%s", err, output)
	}
	if len(result.Results) != 1 || result.Results[0].Path != "manual.md" {
		t.Fatalf("query --queries result = %#v, want manual.md only", result.Results)
	}
	if result.Results[0].Explain == nil {
		t.Fatalf("query --explain result missing explain: %#v", result.Results[0])
	}
}

func assertCollectionNames(t *testing.T, dbPath string, want []string) {
	t.Helper()
	store, err := qmd.NewStore(qmd.Config{DBPath: dbPath})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	collections, err := store.ListCollections(context.Background())
	if err != nil {
		t.Fatalf("ListCollections: %v", err)
	}
	var got []string
	for _, c := range collections {
		got = append(got, c.Name)
	}
	if len(got) != len(want) {
		t.Fatalf("collection names = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("collection names = %#v, want %#v", got, want)
		}
	}
}

func excludeCollectionInConfig(t *testing.T, configPath string, name string) {
	t.Helper()
	cfg, err := qmd.LoadConfigFile(configPath)
	if err != nil {
		t.Fatalf("LoadConfigFile: %v", err)
	}
	for i := range cfg.Collections {
		if cfg.Collections[i].Name == name {
			cfg.Collections[i].IncludeByDefault = qmd.BoolPtr(false)
		}
	}
	if err := qmd.SaveConfigFile(configPath, cfg); err != nil {
		t.Fatalf("SaveConfigFile: %v", err)
	}
}

func runCLI(t *testing.T, ctx context.Context, args []string) string {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := Run(ctx, args, &stdout, &stderr); err != nil {
		t.Fatalf("Run(%#v): %v\nstderr: %s", args, err, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	return stdout.String()
}
