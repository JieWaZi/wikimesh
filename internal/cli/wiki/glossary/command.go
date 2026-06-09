package glossarycmd

import (
	"fmt"
	"github.com/JieWaZi/wikimesh/internal/app/wikiapp"
	"github.com/spf13/cobra"
	"io"
	"path/filepath"
	"strings"

	"github.com/JieWaZi/wikimesh/internal/ui"
)

// NewCommand 构造 `wikimesh glossary` 命令。
func NewCommand() *cobra.Command {
	copy := ui.Messages()
	cmd := &cobra.Command{
		Use:   "glossary",
		Short: copy.WikiGlossaryShort,
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}
	cmd.AddCommand(newKeywordsCommand())
	return cmd
}

func newKeywordsCommand() *cobra.Command {
	copy := ui.Messages()
	var root, project string
	cmd := &cobra.Command{
		Use:   "keywords",
		Short: copy.WikiGlossaryKeywordsShort,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWikiGlossaryKeywords(cmd.OutOrStdout(), root, project)
		},
	}
	cmd.Flags().StringVar(&root, "root", ".", copy.FlagRoot)
	cmd.Flags().StringVar(&project, "project", "", copy.FlagProject)
	return cmd
}

// runWikiGlossaryKeywords 从 wiki/glossary.md 的第一列提取术语关键词。
func runWikiGlossaryKeywords(out io.Writer, root string, project string) error {
	resolvedRoot, err := wikiapp.ResolveRoot(root, project)
	if err != nil {
		return err
	}
	rows, err := wikiapp.ReadMarkdownTable(filepath.Join(resolvedRoot, "wiki/glossary.md"))
	if err != nil {
		return err
	}
	for _, row := range rows {
		if len(row) > 0 && strings.TrimSpace(row[0]) != "" {
			fmt.Fprintln(out, strings.TrimSpace(row[0]))
		}
	}
	return nil
}
