package searchcmd

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/JieWaZi/wikimesh/internal/app/wikiapp"
	"github.com/JieWaZi/wikimesh/internal/ui"
)

// NewCommand 构造 `wikimesh search` 命令。
func NewCommand() *cobra.Command {
	msg := ui.Messages()
	var root, project string
	cmd := &cobra.Command{
		Use:   "search <index|glossary|topic|workflow> <query...>",
		Short: msg.WikiSearchShort,
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWikiSearch(cmd.Context(), cmd.OutOrStdout(), root, project, args[0], args[1:])
		},
	}
	cmd.Flags().StringVar(&root, "root", ".", msg.FlagRoot)
	cmd.Flags().StringVar(&project, "project", "", msg.FlagProject)
	return cmd
}

// runWikiSearch 执行 Wikimesh 检索；Topic/Workflow 优先使用 qmd 索引，未命中时回退本地扫描。
func runWikiSearch(ctx context.Context, out io.Writer, root, project, kind string, queries []string) error {
	resolvedRoot, err := wikiapp.ResolveRoot(root, project)
	if err != nil {
		return err
	}
	normalizedQueries := normalizeSearchQueries(queries)
	switch kind {
	case "index":
		return searchWikiTable(out, filepath.Join(resolvedRoot, "wiki/index.md"), []string{"slug", "type", "description"}, []int{2, 0, 1}, normalizedQueries)
	case "glossary":
		return searchWikiTable(out, filepath.Join(resolvedRoot, "wiki/glossary.md"), []string{"slug", "glossary", "type", "description"}, []int{3, 0, 1, 2}, normalizedQueries)
	case "topic", "workflow":
		if ok, err := wikiapp.SearchPagesWithQMD(ctx, out, resolvedRoot, kind, queries); ok || err != nil {
			return err
		}
		return searchPagesLocal(out, resolvedRoot, kind, normalizedQueries)
	default:
		return fmt.Errorf("unsupported wiki search kind %q", kind)
	}
}

// searchWikiTable 对 index/glossary 的 Markdown 表格执行包含匹配。
func searchWikiTable(out io.Writer, path string, headers []string, columns []int, queries []string) error {
	rows, err := wikiapp.ReadMarkdownTable(path)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "|%s|\n", strings.Join(headers, "|"))
	for _, row := range rows {
		if !searchRowMatches(queries, row...) {
			continue
		}
		for len(row) < len(columns) {
			row = append(row, "")
		}
		values := make([]string, 0, len(columns))
		for _, column := range columns {
			value := ""
			if column >= 0 && column < len(row) {
				value = row[column]
			}
			values = append(values, pipeCell(value))
		}
		fmt.Fprintf(out, "|%s|\n", strings.Join(values, "|"))
	}
	return nil
}

// searchPagesLocal 在 qmd 索引不可用时扫描本地 Markdown 页面。
func searchPagesLocal(out io.Writer, root, kind string, queries []string) error {
	pages, err := wikiapp.ListPages(root, kind)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "|slug|title|score|")
	for _, page := range pages {
		haystack := strings.ToLower(page.Title + "\n" + page.Slug + "\n" + page.Summary + "\n" + page.Text)
		if !searchHaystackMatches(queries, haystack) {
			continue
		}
		fmt.Fprintf(out, "|%s|%s|%s|\n", pipeCell(page.Slug), pipeCell(page.Title), "100%")
	}
	return nil
}

func normalizeSearchQueries(raw []string) []string {
	queries := make([]string, 0, len(raw))
	for _, query := range raw {
		query = strings.ToLower(strings.TrimSpace(query))
		if query == "" {
			continue
		}
		queries = append(queries, query)
	}
	return queries
}

func searchRowMatches(queries []string, values ...string) bool {
	return searchHaystackMatches(queries, strings.ToLower(strings.Join(values, " ")))
}

func searchHaystackMatches(queries []string, haystack string) bool {
	if len(queries) == 0 {
		return true
	}
	for _, query := range queries {
		if strings.Contains(haystack, query) {
			return true
		}
	}
	return false
}

// pipeCell 转义 pipe table 单元格，保证输出仍是合法表格。
func pipeCell(value string) string {
	value = strings.ReplaceAll(value, "\r\n", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "|", "\\|")
	return strings.TrimSpace(value)
}
