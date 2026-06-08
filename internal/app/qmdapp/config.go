package qmdapp

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	qmd "github.com/JieWaZi/wikimesh/pkg/qmd"
)

// DefaultConfigPath 是工作区内默认 qmd 配置文件路径。
const DefaultConfigPath = ".wikimesh/qmd.yaml"

// LoadConfig 读取 qmd 配置；配置不存在时写入默认配置并返回。
func LoadConfig(path string) (qmd.FileConfig, error) {
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

// OpenStoreFromConfig 按文件配置打开 qmd Store，并恢复 collection 与全局上下文。
func OpenStoreFromConfig(ctx context.Context, cfg qmd.FileConfig) (*qmd.Store, error) {
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

// OpenDefaultStore 使用当前工作区默认配置打开 qmd Store。
func OpenDefaultStore(ctx context.Context) (*qmd.Store, qmd.FileConfig, error) {
	cfg, err := LoadConfig(DefaultConfigPath)
	if err != nil {
		return nil, qmd.FileConfig{}, err
	}
	store, err := OpenStoreFromConfig(ctx, cfg)
	if err != nil {
		return nil, qmd.FileConfig{}, err
	}
	return store, cfg, nil
}

// AddCollectionAndSync 新增 collection 后同步写回配置文件。
func AddCollectionAndSync(ctx context.Context, cfg qmd.FileConfig, configPath string, store *qmd.Store, collection qmd.Collection) error {
	if err := store.AddCollection(ctx, collection); err != nil {
		return err
	}
	return SyncConfigFile(ctx, cfg, configPath, store)
}

// SyncConfigFile 从 Store 当前状态反写 qmd 配置文件。
func SyncConfigFile(ctx context.Context, cfg qmd.FileConfig, configPath string, store *qmd.Store) error {
	collections, err := store.ListCollections(ctx)
	if err != nil {
		return err
	}
	// 写回前把工作区内路径转成相对路径，避免配置落入机器相关绝对路径。
	cfg.DBPath = relativizeWorkspacePath(configPath, cfg.DBPath)
	cfg.Collections = relativizeCollectionPaths(configPath, collections)
	global, err := store.GlobalContext(ctx)
	if err != nil {
		return err
	}
	cfg.GlobalContext = global
	return qmd.SaveConfigFile(configPath, cfg)
}

// ConfigPathForRoot 返回指定工作区根目录下的 qmd 配置路径。
func ConfigPathForRoot(root string) string {
	return filepath.Join(root, DefaultConfigPath)
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
