package common

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	qmd "github.com/JieWaZi/wikimesh/pkg/qmd"
)

const DefaultQMDConfigPath = ".wikimesh/qmd.yaml"

func LoadQMDConfig(path string) (qmd.FileConfig, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			cfg := qmd.DefaultFileConfig()
			if err := qmd.SaveConfigFile(path, cfg); err != nil {
				return qmd.FileConfig{}, err
			}
			return cfg, nil
		}
		return qmd.FileConfig{}, err
	}
	return qmd.LoadConfigFile(path)
}

func OpenQMDStoreFromConfig(ctx context.Context, cfg qmd.FileConfig) (*qmd.Store, error) {
	store, err := qmd.NewStore(cfg.StoreConfig())
	if err != nil {
		return nil, err
	}
	for _, c := range cfg.Collections {
		if err := store.AddCollection(ctx, c); err != nil {
			store.Close()
			return nil, err
		}
	}
	if strings.TrimSpace(cfg.GlobalContext) != "" {
		if err := store.SetGlobalContext(ctx, cfg.GlobalContext); err != nil {
			store.Close()
			return nil, err
		}
	}
	return store, nil
}

func OpenDefaultQMDStore(ctx context.Context) (*qmd.Store, qmd.FileConfig, error) {
	cfg, err := LoadQMDConfig(DefaultQMDConfigPath)
	if err != nil {
		return nil, qmd.FileConfig{}, err
	}
	store, err := OpenQMDStoreFromConfig(ctx, cfg)
	if err != nil {
		return nil, qmd.FileConfig{}, err
	}
	return store, cfg, nil
}

func AddQMDCollectionAndSync(ctx context.Context, cfg qmd.FileConfig, configPath string, store *qmd.Store, collection qmd.Collection) error {
	if err := store.AddCollection(ctx, collection); err != nil {
		return err
	}
	return SyncQMDConfigFile(ctx, cfg, configPath, store)
}

func SyncQMDConfigFile(ctx context.Context, cfg qmd.FileConfig, configPath string, store *qmd.Store) error {
	collections, err := store.ListCollections(ctx)
	if err != nil {
		return err
	}
	cfg.DBPath = relativizeWorkspacePath(configPath, cfg.DBPath)
	cfg.Collections = relativizeCollectionPaths(configPath, collections)
	global, err := store.GlobalContext(ctx)
	if err != nil {
		return err
	}
	cfg.GlobalContext = global
	return qmd.SaveConfigFile(configPath, cfg)
}

func QMDConfigPathForRoot(root string) string {
	return filepath.Join(root, DefaultQMDConfigPath)
}

func relativizeCollectionPaths(configPath string, collections []qmd.Collection) []qmd.Collection {
	normalized := make([]qmd.Collection, 0, len(collections))
	for _, collection := range collections {
		collection.Path = relativizeWorkspacePath(configPath, collection.Path)
		normalized = append(normalized, collection)
	}
	return normalized
}

func relativizeWorkspacePath(configPath, path string) string {
	if !filepath.IsAbs(path) {
		return path
	}
	root := filepath.Dir(filepath.Dir(configPath))
	if rel, err := filepath.Rel(root, path); err == nil && isWorkspaceRelativePath(rel) {
		return filepath.ToSlash(rel)
	}
	return path
}

func isWorkspaceRelativePath(path string) bool {
	return path != "." && path != ".." && !strings.HasPrefix(path, ".."+string(filepath.Separator))
}
