package common

import (
	"context"
	"path/filepath"
	"testing"

	qmd "github.com/JieWaZi/wikimesh/pkg/qmd"
)

func TestSyncQMDConfigFileRelativizesWorkspaceDBPath(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	configPath := filepath.Join(root, DefaultQMDConfigPath)
	cfg := qmd.DefaultFileConfig()
	cfg.DBPath = filepath.Join(root, ".wikimesh/wiki.db")

	store, err := qmd.NewStore(cfg.StoreConfig())
	if err != nil {
		t.Fatalf("NewStore error = %v", err)
	}
	defer store.Close()

	if err := SyncQMDConfigFile(ctx, cfg, configPath, store); err != nil {
		t.Fatalf("SyncQMDConfigFile error = %v", err)
	}
	got, err := qmd.LoadConfigFile(configPath)
	if err != nil {
		t.Fatalf("LoadConfigFile error = %v", err)
	}
	if got.DBPath != ".wikimesh/wiki.db" {
		t.Fatalf("DBPath = %q, want relative workspace path", got.DBPath)
	}
}
