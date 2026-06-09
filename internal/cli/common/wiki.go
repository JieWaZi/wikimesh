package common

import (
	"bytes"
	"context"
	"fmt"
	qmd "github.com/JieWaZi/wikimesh/pkg/qmd"
	"gopkg.in/yaml.v3"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// WikiPage 是从 Topic/Workflow Markdown 页面解析出的轻量页面模型。
type WikiPage struct {
	// Rel 是页面相对工作区根目录的路径。
	Rel string
	// Title 是页面 frontmatter 中的标题。
	Title string
	// Slug 是页面稳定标识，默认来自文件名。
	Slug string
	// Kind 是页面类型，例如 topic 或 workflow。
	Kind string
	// Status 是页面状态字段。
	Status string
	// Summary 是页面摘要，用于搜索召回。
	Summary string
	// Sections 保存 card/core/explain 等 section 内容。
	Sections map[string]string
	// Text 是页面正文原文，用于轻量搜索。
	Text string
}

// WikiRepoConfig 是用户级 Wikimesh 项目来源配置。
type WikiRepoConfig struct {
	// ProjectName 是项目展示名称。
	ProjectName string `json:"project_name" yaml:"project_name"`
	// ProjectSlug 是命令行使用的稳定项目标识。
	ProjectSlug string `json:"project_slug" yaml:"project_slug"`
	// Language 是项目默认语言。
	Language string `json:"language" yaml:"language"`
	// ActiveSource 是当前读取/搜索使用的来源类型。
	ActiveSource string `json:"active_source" yaml:"active_source"`
	// Sources 保存本地和远端两类可切换来源。
	Sources WikiRepoSources `json:"sources" yaml:"sources"`
	// CodeRepos 保存与该知识库关联的代码仓。
	CodeRepos []WikiCodeRepo `json:"code_repos,omitempty" yaml:"code_repos,omitempty"`
}

// WikiRepoSources 聚合一个项目可配置的知识来源。
type WikiRepoSources struct {
	// Local 是本地 Wikimesh 工作区来源。
	Local *WikiRepoSource `json:"local,omitempty" yaml:"local,omitempty"`
	// Remote 是远端 Wikimesh API 来源。
	Remote *WikiRepoSource `json:"remote,omitempty" yaml:"remote,omitempty"`
}

// WikiRepoSource 描述单个本地或远端来源。
type WikiRepoSource struct {
	// Type 是来源类型，取值为 local 或 remote。
	Type string `json:"type" yaml:"type"`
	// Path 是本地来源的工作区路径。
	Path string `json:"path,omitempty" yaml:"path,omitempty"`
	// URL 是远端来源的 API 地址。
	URL string `json:"url,omitempty" yaml:"url,omitempty"`
}

// WikiCodeRepo 描述与 Wikimesh 项目关联的代码仓。
type WikiCodeRepo struct {
	// Name 是代码仓展示名称。
	Name string `json:"name" yaml:"name"`
	// Slug 是代码仓稳定标识。
	Slug string `json:"slug" yaml:"slug"`
	// Path 是代码仓本地路径。
	Path string `json:"path" yaml:"path"`
	// Default 表示该代码仓是项目默认代码仓。
	Default bool `json:"default" yaml:"default"`
}

var wikiSectionStart = regexp.MustCompile(`^<!--\s*wikimesh:section\s+id=([A-Za-z0-9_-]+)\s*-->\s*$`)

type wikiPageMeta struct {
	Title   string `yaml:"title"`
	Slug    string `yaml:"slug"`
	Kind    string `yaml:"kind"`
	Status  string `yaml:"status"`
	Summary string `yaml:"summary"`
}

const (
	// WikiRepoSourceLocal 表示本地 Wikimesh 文档工作区来源。
	WikiRepoSourceLocal = "local"
	// WikiRepoSourceRemote 表示远端 Wikimesh HTTP API 来源。
	WikiRepoSourceRemote = "remote"
)

// SearchWikiPagesWithQMD 尝试使用工作区 qmd 索引搜索 Topic/Workflow 页面。
func SearchWikiPagesWithQMD(ctx context.Context, out io.Writer, root, kind string, queries []string) (bool, error) {
	cfgPath := QMDConfigPathForRoot(root)
	if _, err := os.Stat(cfgPath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	cfg, err := qmd.LoadConfigFile(cfgPath)
	if err != nil {
		return false, err
	}
	if err := AbsolutizeWikiQMDConfig(root, &cfg); err != nil {
		return false, err
	}
	store, err := OpenQMDStoreFromConfig(ctx, cfg)
	if err != nil {
		return false, err
	}
	defer store.Close()

	results, err := store.SearchMany(ctx, "", queries, qmd.SearchOptions{Limit: 50})
	if err != nil {
		return false, err
	}
	rows := filterWikiQMDResults(kind, results)
	if len(rows) == 0 {
		return false, nil
	}
	fmt.Fprintln(out, "|slug|title|score|")
	for _, hit := range rows {
		_, slug, title := wikiQMDHitDisplay(root, kind, hit)
		fmt.Fprintf(out, "|%s|%s|%.2f|\n", pipeCell(slug), pipeCell(title), hit.Score)
	}
	return true, nil
}

// pipeCell 转义 pipe table 单元格，保证输出仍是合法表格。
func pipeCell(value string) string {
	value = strings.ReplaceAll(value, "\r\n", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "|", "\\|")
	return strings.TrimSpace(value)
}

// AbsolutizeWikiQMDConfig 把工作区相对 qmd 路径转为绝对路径，支持 --root 从任意目录搜索。
func AbsolutizeWikiQMDConfig(root string, cfg *qmd.FileConfig) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	if cfg.DBPath != "" && !filepath.IsAbs(cfg.DBPath) {
		cfg.DBPath = filepath.Join(absRoot, cfg.DBPath)
	}
	for i := range cfg.Collections {
		if cfg.Collections[i].Path != "" && !filepath.IsAbs(cfg.Collections[i].Path) {
			cfg.Collections[i].Path = filepath.Join(absRoot, cfg.Collections[i].Path)
		}
	}
	return nil
}

// filterWikiQMDResults 只保留目标页面类型目录下的 qmd 命中。
func filterWikiQMDResults(kind string, results []qmd.SearchResult) []qmd.SearchResult {
	dir := kind + "s"
	var rows []qmd.SearchResult
	for _, hit := range results {
		path := filepath.ToSlash(hit.Path)
		if strings.HasPrefix(path, dir+"/") || strings.Contains(path, "/"+dir+"/") {
			rows = append(rows, hit)
		}
	}
	return rows
}

// wikiQMDHitDisplay 使用 Wikimesh 页面元数据修正 qmd 命中的展示字段。
func wikiQMDHitDisplay(root, kind string, hit qmd.SearchResult) (string, string, string) {
	rel := filepath.ToSlash(hit.Path)
	if !strings.HasPrefix(rel, "wiki/") {
		rel = filepath.ToSlash(filepath.Join("wiki", rel))
	}
	if page, err := ParseWikiPage(root, rel); err == nil {
		return filepath.Base(page.Rel), page.Slug, page.Title
	}
	slug := strings.TrimSuffix(filepath.Base(hit.Path), filepath.Ext(hit.Path))
	title := hit.Title
	if title == "" {
		title = slug
	}
	return filepath.Base(hit.Path), slug, title
}

// WikiRepoConfigRoot 返回用户级 Wikimesh 配置根目录。
func WikiRepoConfigRoot() (string, error) {
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
	root, err := WikiRepoConfigRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, slug, "config.yaml"), nil
}

// LoadWikiRepoConfig 读取并解析项目来源配置。
func LoadWikiRepoConfig(slug string) (WikiRepoConfig, error) {
	path, err := wikiRepoConfigPath(slug)
	if err != nil {
		return WikiRepoConfig{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return WikiRepoConfig{}, err
	}
	var cfg WikiRepoConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return WikiRepoConfig{}, err
	}
	normalizeWikiRepoConfig(&cfg)
	if err := validateWikiRepoConfig(cfg); err != nil {
		return WikiRepoConfig{}, err
	}
	return cfg, nil
}

// SaveWikiRepoConfig 将项目来源配置写入用户级配置目录。
func SaveWikiRepoConfig(cfg WikiRepoConfig) error {
	normalizeWikiRepoConfig(&cfg)
	if err := validateWikiRepoConfig(cfg); err != nil {
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

// validateWikiRepoConfig 校验用户级 Wikimesh 项目配置的稳定结构。
func validateWikiRepoConfig(cfg WikiRepoConfig) error {
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
	case WikiRepoSourceLocal:
		if cfg.Sources.Local == nil {
			return fmt.Errorf("active local wikimesh source is not configured")
		}
	case WikiRepoSourceRemote:
		if cfg.Sources.Remote == nil {
			return fmt.Errorf("active remote wikimesh source is not configured")
		}
	default:
		return fmt.Errorf("unsupported active wikimesh source %q", active)
	}
	if cfg.Sources.Local != nil {
		if cfg.Sources.Local.Type != WikiRepoSourceLocal {
			return fmt.Errorf("local wikimesh source must have type %q", WikiRepoSourceLocal)
		}
		if strings.TrimSpace(cfg.Sources.Local.Path) == "" {
			return fmt.Errorf("local wikimesh source requires path")
		}
		if strings.TrimSpace(cfg.Sources.Local.URL) != "" {
			return fmt.Errorf("local wikimesh source must not set url")
		}
	}
	if cfg.Sources.Remote != nil {
		if cfg.Sources.Remote.Type != WikiRepoSourceRemote {
			return fmt.Errorf("remote wikimesh source must have type %q", WikiRepoSourceRemote)
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

// normalizeWikiRepoConfig 规范化用户级项目配置中的可修剪字段。
func normalizeWikiRepoConfig(cfg *WikiRepoConfig) {
	cfg.ActiveSource = strings.TrimSpace(cfg.ActiveSource)
}

// ListWikiRepoSlugs 列出已经登记的 Wikimesh 项目 slug。
func ListWikiRepoSlugs() ([]string, error) {
	root, err := WikiRepoConfigRoot()
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

// ResolveWikiRoot returns the local wiki workspace root for a project or the explicit root.
func ResolveWikiRoot(root string, project string) (string, error) {
	project = strings.TrimSpace(project)
	if project == "" {
		if strings.TrimSpace(root) == "" {
			return ".", nil
		}
		return root, nil
	}
	cfg, err := LoadWikiRepoConfig(WikiSlug(project))
	if err != nil {
		return "", err
	}
	if cfg.ActiveSource != WikiRepoSourceLocal || cfg.Sources.Local == nil {
		return "", fmt.Errorf("wikimesh project %q active source is not local", project)
	}
	if strings.TrimSpace(cfg.Sources.Local.Path) == "" {
		return "", fmt.Errorf("wikimesh project %q local source path is empty", project)
	}
	return cfg.Sources.Local.Path, nil
}

// ListWikiPages 扫描指定类型目录下的全部 Markdown 页面。
func ListWikiPages(root, kind string) ([]WikiPage, error) {
	dir := filepath.Join(root, "wiki", kind+"s")
	var pages []WikiPage
	if err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		page, err := ParseWikiPage(root, filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		pages = append(pages, page)
		return nil
	}); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	sort.Slice(pages, func(i, j int) bool { return pages[i].Rel < pages[j].Rel })
	return pages, nil
}

// ParseWikiPage 解析 frontmatter 和 Wikimesh section 标记。
func ParseWikiPage(root, rel string) (WikiPage, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return WikiPage{}, err
	}
	meta, body, ok := splitFrontmatter(data)
	if !ok {
		return WikiPage{}, fmt.Errorf("%s: missing YAML frontmatter", rel)
	}
	page := WikiPage{Rel: rel, Sections: map[string]string{}, Text: string(body)}
	var frontmatter wikiPageMeta
	if err := yaml.Unmarshal(meta, &frontmatter); err != nil {
		return WikiPage{}, fmt.Errorf("%s: parse frontmatter: %w", rel, err)
	}
	page.Title = strings.TrimSpace(frontmatter.Title)
	page.Slug = strings.TrimSpace(frontmatter.Slug)
	page.Kind = strings.TrimSpace(frontmatter.Kind)
	page.Status = strings.TrimSpace(frontmatter.Status)
	page.Summary = strings.TrimSpace(frontmatter.Summary)
	if page.Slug == "" {
		page.Slug = strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
	}
	if page.Title == "" {
		page.Title = page.Slug
	}
	sections, err := parseWikiSections(body)
	if err != nil {
		return WikiPage{}, fmt.Errorf("%s: %w", rel, err)
	}
	page.Sections = sections
	return page, nil
}

// splitFrontmatter 拆分 Markdown YAML frontmatter 和正文。
func splitFrontmatter(data []byte) ([]byte, []byte, bool) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if !bytes.HasPrefix(data, []byte("---\n")) {
		return nil, data, false
	}
	rest := data[len("---\n"):]
	end := bytes.Index(rest, []byte("\n---"))
	if end < 0 {
		return nil, data, false
	}
	bodyStart := end + len("\n---")
	if len(rest) > bodyStart && rest[bodyStart] == '\n' {
		bodyStart++
	}
	return rest[:end], rest[bodyStart:], true
}

// parseWikiSections 解析 card/core/explain 等非嵌套 section。
func parseWikiSections(body []byte) (map[string]string, error) {
	sections := map[string]string{}
	var current string
	var buf strings.Builder
	// 逐行处理开始/结束标记，避免用正则一次性跨段匹配导致错误吞内容。
	for _, line := range strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n") {
		if match := wikiSectionStart.FindStringSubmatch(strings.TrimSpace(line)); match != nil {
			if current != "" {
				return nil, fmt.Errorf("nested section %q", match[1])
			}
			current = match[1]
			buf.Reset()
			continue
		}
		if strings.TrimSpace(line) == "<!-- /wikimesh:section -->" {
			if current == "" {
				return nil, fmt.Errorf("unexpected section end")
			}
			sections[current] = strings.TrimRight(buf.String(), "\n")
			current = ""
			buf.Reset()
			continue
		}
		if current != "" {
			buf.WriteString(line)
			buf.WriteByte('\n')
		}
	}
	if current != "" {
		return nil, fmt.Errorf("unclosed section %q", current)
	}
	return sections, nil
}

// ReadMarkdownTable 读取文件中的第一个 Markdown 表格并返回数据行。
func ReadMarkdownTable(path string) ([][]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rows [][]string
	inTable := false
	for _, line := range strings.Split(string(data), "\n") {
		cells, ok := parseMarkdownTableLine(line)
		if !ok {
			if inTable {
				break
			}
			continue
		}
		if !inTable {
			inTable = true
			continue
		}
		if isMarkdownSeparator(cells) {
			continue
		}
		rows = append(rows, cells)
	}
	return rows, nil
}

// parseMarkdownTableLine 解析一行 pipe table。
func parseMarkdownTableLine(line string) ([]string, bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "|") || !strings.HasSuffix(line, "|") {
		return nil, false
	}
	line = strings.Trim(line, "|")
	raw := strings.Split(line, "|")
	cells := make([]string, len(raw))
	for i, cell := range raw {
		cells[i] = strings.TrimSpace(strings.ReplaceAll(cell, `\|`, "|"))
	}
	return cells, true
}

// isMarkdownSeparator 判断表格行是否为 Markdown 分隔行。
func isMarkdownSeparator(cells []string) bool {
	if len(cells) == 0 {
		return false
	}
	for _, cell := range cells {
		cell = strings.Trim(cell, " :-")
		if cell != "" {
			return false
		}
	}
	return true
}

// WikiSlug 把项目名或标题转为稳定的小写连字符 slug。
func WikiSlug(text string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(text) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
