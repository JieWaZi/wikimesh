package qmdcmd

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/JieWaZi/wikimesh/internal/cli/common"
	qmd "github.com/JieWaZi/wikimesh/pkg/qmd"
)

func TestResolveProjectQMDConfigPathUsesProjectLocalSource(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	saveQMDProjectConfig(t, "sample", root)

	gotRoot, gotPath, err := resolveProjectQMDConfigPath("sample")
	if err != nil {
		t.Fatalf("resolveProjectQMDConfigPath error = %v", err)
	}
	if gotRoot != root {
		t.Fatalf("root = %q, want %q", gotRoot, root)
	}
	wantPath := filepath.Join(root, common.DefaultQMDConfigPath)
	if gotPath != wantPath {
		t.Fatalf("path = %q, want %q", gotPath, wantPath)
	}
}

func TestOpenStoreForProjectAnchorsRelativeQMDPathsToProjectRoot(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	saveQMDProjectConfig(t, "sample", root)

	cfg := qmd.DefaultFileConfig()
	cfg.DBPath = ".wikimesh/wiki.db"
	cfg.Collections = []qmd.Collection{{Name: "docs", Path: "wiki"}}
	configPath := filepath.Join(root, common.DefaultQMDConfigPath)
	if err := qmd.SaveConfigFile(configPath, cfg); err != nil {
		t.Fatalf("SaveConfigFile error = %v", err)
	}

	store, gotCfg, gotPath, err := openStoreForProject(context.Background(), "sample")
	if err != nil {
		t.Fatalf("openStoreForProject error = %v", err)
	}
	defer store.Close()

	if gotPath != configPath {
		t.Fatalf("configPath = %q, want %q", gotPath, configPath)
	}
	wantDB := filepath.Join(root, ".wikimesh/wiki.db")
	if gotCfg.DBPath != wantDB {
		t.Fatalf("DBPath = %q, want %q", gotCfg.DBPath, wantDB)
	}
	if len(gotCfg.Collections) != 1 {
		t.Fatalf("collections = %#v, want one collection", gotCfg.Collections)
	}
	wantCollectionPath := filepath.Join(root, "wiki")
	if gotCfg.Collections[0].Path != wantCollectionPath {
		t.Fatalf("collection path = %q, want %q", gotCfg.Collections[0].Path, wantCollectionPath)
	}
}

func saveQMDProjectConfig(t *testing.T, slug, root string) {
	t.Helper()
	if err := common.SaveWikiRepoConfig(common.WikiRepoConfig{
		ProjectName:  slug,
		ProjectSlug:  slug,
		Language:     "zh",
		ActiveSource: common.WikiRepoSourceLocal,
		Sources: common.WikiRepoSources{
			Local: &common.WikiRepoSource{Type: common.WikiRepoSourceLocal, Path: root},
		},
	}); err != nil {
		t.Fatalf("SaveWikiRepoConfig error = %v", err)
	}
}
