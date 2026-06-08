package searchcmd

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"io"
	"path/filepath"
	"strings"

	"github.com/JieWaZi/wikimesh/internal/cli/common"
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
	resolvedRoot, err := common.ResolveWikiRoot(root, project)
	if err != nil {
		return err
	}
	query := strings.ToLower(strings.Join(queries, " "))
	switch kind {
	case "index":
		return searchWikiTable(out, filepath.Join(resolvedRoot, "wiki/index.md"), []string{"type", "description", "slug"}, query)
	case "glossary":
		return searchWikiTable(out, filepath.Join(resolvedRoot, "wiki/glossary.md"), []string{"glossary", "type", "description", "slug"}, query)
	case "topic", "workflow":
		if ok, err := common.SearchWikiPagesWithQMD(ctx, out, resolvedRoot, kind, strings.Join(queries, " ")); ok || err != nil {
			return err
		}
		return searchWikiPagesLocal(out, resolvedRoot, kind, query)
	default:
		return fmt.Errorf("unsupported wiki search kind %q", kind)
	}
}

// searchWikiTable 对 index/glossary 的 Markdown 表格执行包含匹配。
func searchWikiTable(out io.Writer, path string, headers []string, query string) error {
	rows, err := common.ReadMarkdownTable(path)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "|%s|\n", strings.Join(headers, "|"))
	for _, row := range rows {
		if query != "" && !strings.Contains(strings.ToLower(strings.Join(row, " ")), query) {
			continue
		}
		for len(row) < len(headers) {
			row = append(row, "")
		}
		for i := range row {
			row[i] = pipeCell(row[i])
		}
		fmt.Fprintf(out, "|%s|\n", strings.Join(row[:len(headers)], "|"))
	}
	return nil
}

// searchWikiPagesLocal 在 qmd 索引不可用时扫描本地 Markdown 页面。
func searchWikiPagesLocal(out io.Writer, root, kind, query string) error {
	pages, err := common.ListWikiPages(root, kind)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "|file|slug|title|score|")
	for _, page := range pages {
		haystack := strings.ToLower(page.Title + "\n" + page.Slug + "\n" + page.Summary + "\n" + page.Text)
		if query != "" && !strings.Contains(haystack, query) {
			continue
		}
		fmt.Fprintf(out, "|%s|%s|%s|%s|\n", pipeCell(filepath.Base(page.Rel)), pipeCell(page.Slug), pipeCell(page.Title), "100%")
	}
	return nil
}

// pipeCell 转义 pipe table 单元格，保证输出仍是合法表格。
func pipeCell(value string) string {
	value = strings.ReplaceAll(value, "\r\n", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "|", "\\|")
	return strings.TrimSpace(value)
}
