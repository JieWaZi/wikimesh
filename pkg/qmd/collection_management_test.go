package qmd_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	qmd "github.com/JieWaZi/wikimesh/pkg/qmd"
)

func TestCollectionMetadataMatchesQMDShape(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}

	store, err := qmd.NewStore(qmd.Config{DBPath: filepath.Join(dir, "wiki.db")})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	if err := store.AddCollection(ctx, qmd.Collection{
		Name:             "docs",
		Path:             docs,
		Pattern:          "**/*.md",
		Ignore:           []string{"tmp/**"},
		Update:           "git pull",
		IncludeByDefault: qmd.BoolPtr(false),
		Context:          map[string]string{"/": "Project docs"},
	}); err != nil {
		t.Fatalf("AddCollection: %v", err)
	}

	collections, err := store.ListCollections(ctx)
	if err != nil {
		t.Fatalf("ListCollections: %v", err)
	}
	if len(collections) != 1 {
		t.Fatalf("len(collections) = %d, want 1", len(collections))
	}
	got := collections[0]
	if got.Name != "docs" || got.Path != docs || got.Pattern != "**/*.md" {
		t.Fatalf("collection identity = %#v, want qmd-style name/path/pattern", got)
	}
	if got.IncludeByDefault == nil || *got.IncludeByDefault {
		t.Fatalf("IncludeByDefault = %#v, want explicit false", got.IncludeByDefault)
	}
	if got.Update != "git pull" {
		t.Fatalf("Update = %q, want git pull", got.Update)
	}
	if got.Context["/"] != "Project docs" {
		t.Fatalf("Context = %#v, want root context", got.Context)
	}
	if len(got.Ignore) != 1 || got.Ignore[0] != "tmp/**" {
		t.Fatalf("Ignore = %#v, want tmp/**", got.Ignore)
	}
}

func TestDefaultCollectionNamesUseIncludeByDefault(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	archive := filepath.Join(dir, "archive")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(archive, 0o755); err != nil {
		t.Fatal(err)
	}

	store, err := qmd.NewStore(qmd.Config{DBPath: filepath.Join(dir, "wiki.db")})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	if err := store.AddCollection(ctx, qmd.Collection{Name: "docs", Path: docs, Pattern: "**/*.md"}); err != nil {
		t.Fatalf("Add docs: %v", err)
	}
	if err := store.AddCollection(ctx, qmd.Collection{Name: "archive", Path: archive, Pattern: "**/*.md", IncludeByDefault: qmd.BoolPtr(false)}); err != nil {
		t.Fatalf("Add archive: %v", err)
	}

	names, err := store.DefaultCollectionNames(ctx)
	if err != nil {
		t.Fatalf("DefaultCollectionNames: %v", err)
	}
	if len(names) != 1 || names[0] != "docs" {
		t.Fatalf("DefaultCollectionNames = %#v, want docs only", names)
	}

	if ok, err := store.UpdateCollectionSettings(ctx, "archive", qmd.CollectionSettings{IncludeByDefault: qmd.BoolPtr(true)}); err != nil || !ok {
		t.Fatalf("include archive ok=%v err=%v", ok, err)
	}
	names, err = store.DefaultCollectionNames(ctx)
	if err != nil {
		t.Fatalf("DefaultCollectionNames after include: %v", err)
	}
	if len(names) != 2 || names[0] != "archive" || names[1] != "docs" {
		t.Fatalf("DefaultCollectionNames after include = %#v, want archive/docs", names)
	}
}

func TestRemoveCollectionDeletesIndexedDocuments(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "guide.md"), []byte("# Guide\n\ncollection removal marker\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := qmd.NewStore(qmd.Config{DBPath: filepath.Join(dir, "wiki.db")})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	if err := store.AddCollection(ctx, qmd.Collection{Name: "docs", Path: docs, Pattern: "**/*.md"}); err != nil {
		t.Fatalf("AddCollection: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "docs", qmd.UpdateOptions{}); err != nil {
		t.Fatalf("UpdateCollection: %v", err)
	}

	removed, err := store.RemoveCollection(ctx, "docs")
	if err != nil || !removed {
		t.Fatalf("RemoveCollection removed=%v err=%v", removed, err)
	}
	hits, err := store.Search(ctx, "", "removal marker", qmd.SearchOptions{Limit: 5})
	if err != nil {
		t.Fatalf("Search after remove: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("Search after remove = %#v, want no hits", hits)
	}
	removed, err = store.RemoveCollection(ctx, "docs")
	if err != nil {
		t.Fatalf("RemoveCollection missing: %v", err)
	}
	if removed {
		t.Fatalf("RemoveCollection missing returned true, want false")
	}
}

func TestRenameCollectionMovesIndexedDocuments(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "guide.md"), []byte("# Guide\n\nrename marker\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := qmd.NewStore(qmd.Config{DBPath: filepath.Join(dir, "wiki.db")})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	if err := store.AddCollection(ctx, qmd.Collection{Name: "old-name", Path: docs, Pattern: "**/*.md"}); err != nil {
		t.Fatalf("AddCollection: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "old-name", qmd.UpdateOptions{}); err != nil {
		t.Fatalf("UpdateCollection: %v", err)
	}

	renamed, err := store.RenameCollection(ctx, "old-name", "new-name")
	if err != nil || !renamed {
		t.Fatalf("RenameCollection renamed=%v err=%v", renamed, err)
	}
	hits, err := store.Search(ctx, "new-name", "rename marker", qmd.SearchOptions{Limit: 5})
	if err != nil {
		t.Fatalf("Search new-name: %v", err)
	}
	if len(hits) != 1 || hits[0].Collection != "new-name" || hits[0].Path != "guide.md" {
		t.Fatalf("Search new-name = %#v, want renamed hit", hits)
	}
	oldHits, err := store.Search(ctx, "old-name", "rename marker", qmd.SearchOptions{Limit: 5})
	if err != nil {
		t.Fatalf("Search old-name: %v", err)
	}
	if len(oldHits) != 0 {
		t.Fatalf("Search old-name = %#v, want no hits", oldHits)
	}
}

func TestGlobalAndCollectionContexts(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}

	store, err := qmd.NewStore(qmd.Config{DBPath: filepath.Join(dir, "wiki.db")})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	if err := store.AddCollection(ctx, qmd.Collection{Name: "docs", Path: docs, Pattern: "**/*.md"}); err != nil {
		t.Fatalf("AddCollection: %v", err)
	}

	if err := store.SetGlobalContext(ctx, "All docs"); err != nil {
		t.Fatalf("SetGlobalContext: %v", err)
	}
	if ok, err := store.AddContext(ctx, "docs", "/api", "API docs"); err != nil || !ok {
		t.Fatalf("AddContext ok=%v err=%v", ok, err)
	}
	contexts, err := store.ListContexts(ctx)
	if err != nil {
		t.Fatalf("ListContexts: %v", err)
	}
	if len(contexts) != 2 {
		t.Fatalf("ListContexts = %#v, want global and collection context", contexts)
	}
	if got, err := store.ContextForPath(ctx, "docs", "api/auth.md"); err != nil || got != "API docs" {
		t.Fatalf("ContextForPath api got=%q err=%v, want API docs", got, err)
	}
	if got, err := store.ContextForPath(ctx, "docs", "other.md"); err != nil || got != "All docs" {
		t.Fatalf("ContextForPath fallback got=%q err=%v, want All docs", got, err)
	}
}

func TestAddCollectionUpdatePreservesExistingContext(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}

	store, err := qmd.NewStore(qmd.Config{DBPath: filepath.Join(dir, "wiki.db")})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	if err := store.AddCollection(ctx, qmd.Collection{Name: "docs", Path: docs, Pattern: "**/*.md", Context: map[string]string{"/api": "API docs"}}); err != nil {
		t.Fatalf("AddCollection first: %v", err)
	}
	if err := store.AddCollection(ctx, qmd.Collection{Name: "docs", Path: docs, Pattern: "**/*.txt"}); err != nil {
		t.Fatalf("AddCollection update: %v", err)
	}
	collections, err := store.ListCollections(ctx)
	if err != nil {
		t.Fatalf("ListCollections: %v", err)
	}
	if len(collections) != 1 {
		t.Fatalf("len(collections) = %d, want 1", len(collections))
	}
	if collections[0].Pattern != "**/*.txt" {
		t.Fatalf("Pattern = %q, want updated pattern", collections[0].Pattern)
	}
	if collections[0].Context["/api"] != "API docs" {
		t.Fatalf("Context = %#v, want existing context preserved", collections[0].Context)
	}
}

func TestListCollectionsIncludesDocumentStats(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "one.md"), []byte("# One\n\nfirst doc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "two.md"), []byte("# Two\n\nsecond doc\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := qmd.NewStore(qmd.Config{DBPath: filepath.Join(dir, "wiki.db")})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	if err := store.AddCollection(ctx, qmd.Collection{Name: "docs", Path: docs, Pattern: "**/*.md"}); err != nil {
		t.Fatalf("AddCollection: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "docs", qmd.UpdateOptions{}); err != nil {
		t.Fatalf("UpdateCollection first: %v", err)
	}
	if err := os.Remove(filepath.Join(docs, "two.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateCollection(ctx, "docs", qmd.UpdateOptions{}); err != nil {
		t.Fatalf("UpdateCollection second: %v", err)
	}

	collections, err := store.ListCollections(ctx)
	if err != nil {
		t.Fatalf("ListCollections: %v", err)
	}
	if len(collections) != 1 {
		t.Fatalf("len(collections) = %d, want 1", len(collections))
	}
	got := collections[0]
	if got.DocCount != 2 || got.ActiveCount != 1 {
		t.Fatalf("collection stats = doc_count %d active_count %d, want 2 and 1", got.DocCount, got.ActiveCount)
	}
	if got.LastModified == "" {
		t.Fatalf("LastModified is empty, want qmd-style last modified timestamp")
	}
}

func TestAddCollectionDefaultsToQMDMarkdownPattern(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "note.md"), []byte("# Note\n\nmarkdown marker\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "note.txt"), []byte("text marker\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := qmd.NewStore(qmd.Config{DBPath: filepath.Join(dir, "wiki.db")})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	if err := store.AddCollection(ctx, qmd.Collection{Name: "docs", Path: docs}); err != nil {
		t.Fatalf("AddCollection: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "docs", qmd.UpdateOptions{}); err != nil {
		t.Fatalf("UpdateCollection: %v", err)
	}

	collections, err := store.ListCollections(ctx)
	if err != nil {
		t.Fatalf("ListCollections: %v", err)
	}
	if len(collections) != 1 || collections[0].Pattern != "**/*.md" {
		t.Fatalf("collections = %#v, want default pattern **/*.md", collections)
	}
	txtHits, err := store.Search(ctx, "docs", "text marker", qmd.SearchOptions{Limit: 5})
	if err != nil {
		t.Fatalf("Search txt: %v", err)
	}
	if len(txtHits) != 0 {
		t.Fatalf("txt hits = %#v, want no hits under qmd default markdown pattern", txtHits)
	}
	mdHits, err := store.Search(ctx, "docs", "markdown marker", qmd.SearchOptions{Limit: 5})
	if err != nil {
		t.Fatalf("Search md: %v", err)
	}
	if len(mdHits) != 1 || mdHits[0].Path != "note.md" {
		t.Fatalf("md hits = %#v, want note.md", mdHits)
	}
}

func TestUpdateCollectionSkipsUpdateCommandByDefault(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	command := `printf '# Generated\n\nupdate command marker\n' > generated.md`

	store, err := qmd.NewStore(qmd.Config{DBPath: filepath.Join(dir, "wiki.db")})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	if err := store.AddCollection(ctx, qmd.Collection{Name: "docs", Path: docs, Pattern: "**/*.md", Update: command}); err != nil {
		t.Fatalf("AddCollection: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "docs", qmd.UpdateOptions{}); err != nil {
		t.Fatalf("UpdateCollection: %v", err)
	}

	hits, err := store.Search(ctx, "docs", "update command marker", qmd.SearchOptions{Limit: 5})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("hits = %#v, want no file created without pull/update command", hits)
	}
}

func TestUpdateCollectionRunsUpdateCommandWhenPullEnabled(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	command := `printf '# Generated\n\nupdate command marker\n' > generated.md`

	store, err := qmd.NewStore(qmd.Config{DBPath: filepath.Join(dir, "wiki.db")})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	if err := store.AddCollection(ctx, qmd.Collection{Name: "docs", Path: docs, Pattern: "**/*.md", Update: command}); err != nil {
		t.Fatalf("AddCollection: %v", err)
	}
	if _, err := store.UpdateCollection(ctx, "docs", qmd.UpdateOptions{RunUpdateCommand: true}); err != nil {
		t.Fatalf("UpdateCollection: %v", err)
	}

	hits, err := store.Search(ctx, "docs", "update command marker", qmd.SearchOptions{Limit: 5})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 || hits[0].Path != "generated.md" {
		t.Fatalf("hits = %#v, want generated.md created by update command", hits)
	}
}

func TestLoadConfigFileAcceptsQMDCollectionMap(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "wikimesh.yaml")
	docs := filepath.Join(dir, "docs")
	if err := os.WriteFile(configPath, []byte(`
global_context: All docs
collections:
  docs:
    path: `+docs+`
    pattern: "**/*.md"
    ignore:
      - tmp/**
    update: git pull
    includeByDefault: false
    context:
      /: Project docs
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := qmd.LoadConfigFile(configPath)
	if err != nil {
		t.Fatalf("LoadConfigFile: %v", err)
	}
	if cfg.GlobalContext != "All docs" {
		t.Fatalf("GlobalContext = %q, want All docs", cfg.GlobalContext)
	}
	if len(cfg.Collections) != 1 {
		t.Fatalf("len(Collections) = %d, want 1", len(cfg.Collections))
	}
	got := cfg.Collections[0]
	if got.Name != "docs" || got.Path != docs || got.Pattern != "**/*.md" {
		t.Fatalf("collection = %#v, want qmd map collection", got)
	}
	if got.IncludeByDefault == nil || *got.IncludeByDefault {
		t.Fatalf("IncludeByDefault = %#v, want explicit false", got.IncludeByDefault)
	}
	if got.Update != "git pull" || got.Context["/"] != "Project docs" {
		t.Fatalf("metadata = update %q context %#v, want qmd metadata", got.Update, got.Context)
	}
}
