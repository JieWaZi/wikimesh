package wikiapp

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/JieWaZi/wikimesh/internal/app/qmdapp"
	qmd "github.com/JieWaZi/wikimesh/pkg/qmd"
)

// SearchPagesWithQMD 尝试使用工作区 qmd 索引搜索 Topic/Workflow 页面。
func SearchPagesWithQMD(ctx context.Context, out io.Writer, root, kind string, queries []string) (bool, error) {
	cfgPath := qmdapp.ConfigPathForRoot(root)
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
	if err := AbsolutizeQMDConfig(root, &cfg); err != nil {
		return false, err
	}
	store, err := qmdapp.OpenStoreFromConfig(ctx, cfg)
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

// AbsolutizeQMDConfig 把工作区相对 qmd 路径转为绝对路径，支持 --root 从任意目录搜索。
func AbsolutizeQMDConfig(root string, cfg *qmd.FileConfig) error {
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
	if page, err := ParsePage(root, rel); err == nil {
		return filepath.Base(page.Rel), page.Slug, page.Title
	}
	slug := strings.TrimSuffix(filepath.Base(hit.Path), filepath.Ext(hit.Path))
	title := hit.Title
	if title == "" {
		title = slug
	}
	return filepath.Base(hit.Path), slug, title
}
