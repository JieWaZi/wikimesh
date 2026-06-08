package wikiapp

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// SourceLocal 表示本地 Wikimesh 文档工作区来源。
	SourceLocal = "local"
	// SourceRemote 表示远端 Wikimesh HTTP API 来源。
	SourceRemote = "remote"
)

// RepoConfig 是用户级 Wikimesh 项目来源配置。
type RepoConfig struct {
	// ProjectName 是项目展示名称。
	ProjectName string `json:"project_name" yaml:"project_name"`
	// ProjectSlug 是命令行使用的稳定项目标识。
	ProjectSlug string `json:"project_slug" yaml:"project_slug"`
	// Language 是项目默认语言。
	Language string `json:"language" yaml:"language"`
	// ActiveSource 是当前读取/搜索使用的来源类型。
	ActiveSource string `json:"active_source" yaml:"active_source"`
	// Sources 保存本地和远端两类可切换来源。
	Sources RepoSources `json:"sources" yaml:"sources"`
	// CodeRepos 保存与该知识库关联的代码仓。
	CodeRepos []CodeRepo `json:"code_repos,omitempty" yaml:"code_repos,omitempty"`
}

// RepoSources 聚合一个项目可配置的知识来源。
type RepoSources struct {
	// Local 是本地 Wikimesh 工作区来源。
	Local *RepoSource `json:"local,omitempty" yaml:"local,omitempty"`
	// Remote 是远端 Wikimesh API 来源。
	Remote *RepoSource `json:"remote,omitempty" yaml:"remote,omitempty"`
}

// RepoSource 描述单个本地或远端来源。
type RepoSource struct {
	// Type 是来源类型，取值为 local 或 remote。
	Type string `json:"type" yaml:"type"`
	// Path 是本地来源的工作区路径。
	Path string `json:"path,omitempty" yaml:"path,omitempty"`
	// URL 是远端来源的 API 地址。
	URL string `json:"url,omitempty" yaml:"url,omitempty"`
}

// CodeRepo 描述与 Wikimesh 项目关联的代码仓。
type CodeRepo struct {
	// Name 是代码仓展示名称。
	Name string `json:"name" yaml:"name"`
	// Slug 是代码仓稳定标识。
	Slug string `json:"slug" yaml:"slug"`
	// Path 是代码仓本地路径。
	Path string `json:"path" yaml:"path"`
	// Default 表示该代码仓是项目默认代码仓。
	Default bool `json:"default" yaml:"default"`
}

// ConfigRoot 返回用户级 Wikimesh 配置根目录。
func ConfigRoot() (string, error) {
	root := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME"))
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(home, ".config")
	}
	return filepath.Join(root, "wikimesh"), nil
}

// wikiRepoConfigPath 返回指定项目 slug 的配置文件路径。
func wikiRepoConfigPath(slug string) (string, error) {
	root, err := ConfigRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, slug, "config.yaml"), nil
}

// LoadRepoConfig 读取并解析项目来源配置。
func LoadRepoConfig(slug string) (RepoConfig, error) {
	path, err := wikiRepoConfigPath(slug)
	if err != nil {
		return RepoConfig{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return RepoConfig{}, err
	}
	var cfg RepoConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return RepoConfig{}, err
	}
	normalizeRepoConfig(&cfg)
	if err := validateRepoConfig(cfg); err != nil {
		return RepoConfig{}, err
	}
	return cfg, nil
}

// SaveRepoConfig 将项目来源配置写入用户级配置目录。
func SaveRepoConfig(cfg RepoConfig) error {
	normalizeRepoConfig(&cfg)
	if err := validateRepoConfig(cfg); err != nil {
		return err
	}
	path, err := wikiRepoConfigPath(cfg.ProjectSlug)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// validateRepoConfig 校验用户级 Wikimesh 项目配置的稳定结构。
func validateRepoConfig(cfg RepoConfig) error {
	if strings.TrimSpace(cfg.ProjectSlug) == "" {
		return fmt.Errorf("wikimesh project slug is required")
	}
	if strings.TrimSpace(cfg.ProjectName) == "" {
		return fmt.Errorf("wikimesh project name is required")
	}
	if strings.TrimSpace(cfg.Language) == "" {
		return fmt.Errorf("wikimesh project language is required")
	}
	active := strings.TrimSpace(cfg.ActiveSource)
	if active == "" {
		return fmt.Errorf("active wikimesh source is required")
	}
	switch active {
	case SourceLocal:
		if cfg.Sources.Local == nil {
			return fmt.Errorf("active local wikimesh source is not configured")
		}
	case SourceRemote:
		if cfg.Sources.Remote == nil {
			return fmt.Errorf("active remote wikimesh source is not configured")
		}
	default:
		return fmt.Errorf("unsupported active wikimesh source %q", active)
	}
	if cfg.Sources.Local != nil {
		if cfg.Sources.Local.Type != SourceLocal {
			return fmt.Errorf("local wikimesh source must have type %q", SourceLocal)
		}
		if strings.TrimSpace(cfg.Sources.Local.Path) == "" {
			return fmt.Errorf("local wikimesh source requires path")
		}
		if strings.TrimSpace(cfg.Sources.Local.URL) != "" {
			return fmt.Errorf("local wikimesh source must not set url")
		}
	}
	if cfg.Sources.Remote != nil {
		if cfg.Sources.Remote.Type != SourceRemote {
			return fmt.Errorf("remote wikimesh source must have type %q", SourceRemote)
		}
		if strings.TrimSpace(cfg.Sources.Remote.URL) == "" {
			return fmt.Errorf("remote wikimesh source requires url")
		}
		if strings.TrimSpace(cfg.Sources.Remote.Path) != "" {
			return fmt.Errorf("remote wikimesh source must not set path")
		}
	}
	for _, repo := range cfg.CodeRepos {
		if strings.TrimSpace(repo.Slug) == "" {
			return fmt.Errorf("code repo slug is required")
		}
		if strings.TrimSpace(repo.Path) == "" {
			return fmt.Errorf("code repo path is required")
		}
	}
	return nil
}

// normalizeRepoConfig 规范化用户级项目配置中的可修剪字段。
func normalizeRepoConfig(cfg *RepoConfig) {
	cfg.ActiveSource = strings.TrimSpace(cfg.ActiveSource)
}

// ListRepoSlugs 列出已经登记的 Wikimesh 项目 slug。
func ListRepoSlugs() ([]string, error) {
	root, err := ConfigRoot()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	var slugs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		slug := strings.TrimSpace(entry.Name())
		if slug == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, slug, "config.yaml")); err == nil {
			slugs = append(slugs, slug)
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	sort.Strings(slugs)
	return slugs, nil
}

// ResolveRoot 返回 project 对应的本地文档库根目录；未指定 project 时使用显式 root。
func ResolveRoot(root string, project string) (string, error) {
	project = strings.TrimSpace(project)
	if project == "" {
		if strings.TrimSpace(root) == "" {
			return ".", nil
		}
		return root, nil
	}
	cfg, err := LoadRepoConfig(Slug(project))
	if err != nil {
		return "", err
	}
	if cfg.ActiveSource != SourceLocal || cfg.Sources.Local == nil {
		return "", fmt.Errorf("wikimesh project %q active source is not local", project)
	}
	if strings.TrimSpace(cfg.Sources.Local.Path) == "" {
		return "", fmt.Errorf("wikimesh project %q local source path is empty", project)
	}
	return cfg.Sources.Local.Path, nil
}
