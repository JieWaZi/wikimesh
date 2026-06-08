package qmdapp

import (
	"context"
	"path/filepath"
	"testing"

	qmd "github.com/JieWaZi/wikimesh/pkg/qmd"
)

func TestSyncConfigFileRelativizesWorkspaceDBPath(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	configPath := filepath.Join(root, DefaultConfigPath)
	cfg := qmd.DefaultFileConfig()
	cfg.DBPath = filepath.Join(root, ".wikimesh/wiki.db")

	store, err := qmd.NewStore(cfg.StoreConfig())
	if err != nil {
		t.Fatalf("NewStore error = %v", err)
	}
	defer store.Close()

	if err := SyncConfigFile(ctx, cfg, configPath, store); err != nil {
		t.Fatalf("SyncConfigFile error = %v", err)
	}
	got, err := qmd.LoadConfigFile(configPath)
	if err != nil {
		t.Fatalf("LoadConfigFile error = %v", err)
	}
	if got.DBPath != ".wikimesh/wiki.db" {
		t.Fatalf("DBPath = %q, want relative workspace path", got.DBPath)
	}
}
