package readcmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"io"
	"path/filepath"
	"strings"

	"github.com/JieWaZi/wikimesh/internal/cli/common"
	"github.com/JieWaZi/wikimesh/internal/ui"
)

// NewCommand 构造 `wikimesh read` 命令。
func NewCommand() *cobra.Command {
	copy := ui.Messages()
	var root, project, view, format string
	cmd := &cobra.Command{
		Use:   "read <topic|workflow> <slug>",
		Short: copy.WikiReadShort,
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWikiRead(cmd.OutOrStdout(), root, project, args[0], args[1], view, format)
		},
	}
	cmd.Flags().StringVar(&root, "root", ".", copy.FlagRoot)
	cmd.Flags().StringVar(&project, "project", "", copy.FlagProject)
	cmd.Flags().StringVar(&view, "view", "card", copy.FlagView)
	cmd.Flags().StringVar(&format, "format", "text", copy.FlagFormat)
	return cmd
}

// runWikiRead 按 kind/slug/view 读取 Topic 或 Workflow 的指定 section。
func runWikiRead(out io.Writer, root, project, kind, slug, view, format string) error {
	if format != "" && format != "text" {
		return fmt.Errorf("unsupported read format %q", format)
	}
	resolvedRoot, err := common.ResolveWikiRoot(root, project)
	if err != nil {
		return err
	}
	page, err := loadWikiPage(resolvedRoot, kind, slug)
	if err != nil {
		return err
	}
	if view == "" {
		view = "card"
	}
	section, ok := page.Sections[view]
	if !ok {
		return fmt.Errorf("%s: missing section %q", page.Rel, view)
	}
	_, err = fmt.Fprint(out, strings.TrimRight(section, "\n")+"\n")
	return err
}

// loadWikiPage 按约定路径加载一个 topic/workflow 页面。
func loadWikiPage(root, kind, slug string) (common.WikiPage, error) {
	if kind != "topic" && kind != "workflow" {
		return common.WikiPage{}, fmt.Errorf("unsupported wiki page kind %q", kind)
	}
	rel := filepath.ToSlash(filepath.Join("wiki", kind+"s", slug+".md"))
	return common.ParseWikiPage(root, rel)
}
