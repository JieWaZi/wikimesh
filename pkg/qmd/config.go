package qmd

import (
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// FileConfig 是测试 CLI 使用的配置文件结构。
type FileConfig struct {
	// DBPath 是 SQLite 数据库路径。
	DBPath string `yaml:"db_path"`

	// ChunkSize 是 chunk 大小，单位近似 token；不配置时使用默认值 900。
	ChunkSize int `yaml:"chunk_size"`

	// ChunkOverlap 是相邻 chunk 的重叠比例；小于等于 0 时使用 VSearch 默认值 0.15。
	ChunkOverlap float64 `yaml:"chunk_overlap"`

	// Embedding 是向量模型配置。
	Embedding EmbeddingConfig `yaml:"embedding"`

	// Models 保存 qmd 风格的默认模型路径。
	Models ModelsConfig `yaml:"models"`

	// QueryExpansion 配置 query expansion 模型。
	QueryExpansion LlamaCppTextModelConfig `yaml:"query_expansion"`

	// Reranker 配置 query reranker 模型。
	Reranker LlamaCppTextModelConfig `yaml:"reranker"`

	// GlobalContext 是 qmd 的全局上下文。
	GlobalContext string `yaml:"global_context"`

	// Collections 是配置文件里预置的 collection 列表。
	Collections []Collection `yaml:"collections"`
}

type yamlFileConfig struct {
	DBPath         string                    `yaml:"db_path,omitempty"`
	ChunkSize      int                       `yaml:"chunk_size,omitempty"`
	ChunkOverlap   float64                   `yaml:"chunk_overlap,omitempty"`
	Embedding      EmbeddingConfig           `yaml:"embedding,omitempty"`
	Models         ModelsConfig              `yaml:"models,omitempty"`
	QueryExpansion LlamaCppTextModelConfig   `yaml:"query_expansion,omitempty"`
	Reranker       LlamaCppTextModelConfig   `yaml:"reranker,omitempty"`
	GlobalContext  string                    `yaml:"global_context,omitempty"`
	Collections    map[string]yamlCollection `yaml:"collections"`
}

type yamlCollection struct {
	Path             string            `yaml:"path"`
	Pattern          string            `yaml:"pattern,omitempty"`
	Include          []string          `yaml:"include,omitempty"`
	Ignore           []string          `yaml:"ignore,omitempty"`
	Update           string            `yaml:"update,omitempty"`
	IncludeByDefault *bool             `yaml:"includeByDefault,omitempty"`
	Context          map[string]string `yaml:"context,omitempty"`
}

// UnmarshalYAML 同时支持本地旧格式 collections: []Collection 和 qmd 格式 collections: map[name]Collection。
func (c *FileConfig) UnmarshalYAML(value *yaml.Node) error {
	type rawConfig struct {
		DBPath         string                  `yaml:"db_path"`
		ChunkSize      int                     `yaml:"chunk_size"`
		ChunkOverlap   float64                 `yaml:"chunk_overlap"`
		Embedding      EmbeddingConfig         `yaml:"embedding"`
		Models         ModelsConfig            `yaml:"models"`
		QueryExpansion LlamaCppTextModelConfig `yaml:"query_expansion"`
		Reranker       LlamaCppTextModelConfig `yaml:"reranker"`
		GlobalContext  string                  `yaml:"global_context"`
		Collections    yaml.Node               `yaml:"collections"`
	}
	var raw rawConfig
	if err := value.Decode(&raw); err != nil {
		return err
	}
	c.DBPath = raw.DBPath
	c.ChunkSize = raw.ChunkSize
	c.ChunkOverlap = raw.ChunkOverlap
	c.Embedding = raw.Embedding
	c.Models = raw.Models
	c.QueryExpansion = raw.QueryExpansion
	c.Reranker = raw.Reranker
	c.GlobalContext = raw.GlobalContext
	c.Collections = nil

	switch raw.Collections.Kind {
	case 0:
		return nil
	case yaml.SequenceNode:
		return raw.Collections.Decode(&c.Collections)
	case yaml.MappingNode:
		byName := make(map[string]Collection, len(raw.Collections.Content)/2)
		var names []string
		for i := 0; i+1 < len(raw.Collections.Content); i += 2 {
			name := raw.Collections.Content[i].Value
			var collection Collection
			if err := raw.Collections.Content[i+1].Decode(&collection); err != nil {
				return err
			}
			if collection.Name == "" {
				collection.Name = name
			}
			byName[name] = collection
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			c.Collections = append(c.Collections, byName[name])
		}
		return nil
	default:
		return nil
	}
}

// DefaultFileConfig 返回默认落盘配置。
// 默认配置使用 `.wikimesh/wiki.db`、qmd 默认模型来源和本地模型路径，不预置 collection。
func DefaultFileConfig() FileConfig {
	return FileConfig{
		DBPath:       ".wikimesh/wiki.db",
		ChunkSize:    900,
		ChunkOverlap: 0.15,
		Models: ModelsConfig{
			Embed:    DefaultEmbedModel,
			Rerank:   DefaultRerankModel,
			Generate: DefaultGenerateModel,
		},
		Embedding: EmbeddingConfig{
			Provider: "local",
			Model:    LocalModelPath(DefaultEmbedModel),
		},
		QueryExpansion: LlamaCppTextModelConfig{
			Provider: "local",
			Model:    LocalModelPath(DefaultGenerateModel),
		},
		Reranker: LlamaCppTextModelConfig{
			Provider: "local",
			Model:    LocalModelPath(DefaultRerankModel),
		},
		Collections: []Collection{},
	}
}

// LoadConfigFile 从 YAML 文件加载配置。
func LoadConfigFile(path string) (FileConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return FileConfig{}, err
	}
	var cfg FileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return FileConfig{}, err
	}
	return cfg, nil
}

// SaveConfigFile 以 qmd map-style collections 写回配置。
func SaveConfigFile(path string, cfg FileConfig) error {
	out := yamlFileConfig{
		DBPath:         cfg.DBPath,
		ChunkSize:      cfg.ChunkSize,
		ChunkOverlap:   cfg.ChunkOverlap,
		Embedding:      cfg.Embedding,
		Models:         cfg.Models,
		QueryExpansion: cfg.QueryExpansion,
		Reranker:       cfg.Reranker,
		GlobalContext:  cfg.GlobalContext,
		Collections:    map[string]yamlCollection{},
	}
	for _, collection := range cfg.Collections {
		item := yamlCollection{
			Path:    collection.Path,
			Pattern: collection.Pattern,
			Include: collection.Include,
			Ignore:  collection.Ignore,
			Update:  collection.Update,
			Context: collection.Context,
		}
		if collection.IncludeByDefault != nil && !*collection.IncludeByDefault {
			item.IncludeByDefault = collection.IncludeByDefault
		}
		out.Collections[collection.Name] = item
	}
	data, err := yaml.Marshal(out)
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0o644)
}

// StoreConfig 转成 Store 初始化配置。
func (c FileConfig) StoreConfig() Config {
	models := c.Models
	if models.Embed == "" && c.Embedding.Model != "" {
		models.Embed = c.Embedding.Model
	}
	embedding := c.Embedding
	if embedding.Model == "" {
		embedding.Model = models.Embed
	}
	queryExpansion := c.QueryExpansion
	if queryExpansion.Model == "" {
		queryExpansion.Model = models.Generate
	}
	reranker := c.Reranker
	if reranker.Model == "" {
		reranker.Model = models.Rerank
	}
	return Config{
		DBPath:        c.DBPath,
		ChunkSize:     c.ChunkSize,
		ChunkOverlap:  c.ChunkOverlap,
		Embedding:     embedding,
		Models:        models,
		QueryExpander: NewLlamaCppQueryExpander(queryExpansion),
		QueryReranker: NewLlamaCppQueryReranker(reranker),
	}
}
