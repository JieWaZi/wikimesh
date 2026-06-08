package repocmd

import (
	"encoding/json"
	"github.com/spf13/cobra"
	"io"
	"strings"

	"github.com/JieWaZi/wikimesh/internal/cli/common"
	"github.com/JieWaZi/wikimesh/internal/ui"
)

func newInfoCommand() *cobra.Command {
	msg := ui.Messages()
	return &cobra.Command{
		Use:   "info [project]",
		Short: msg.WikiRepoInfoShort,
		Args:  cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			project := ""
			if len(args) > 0 {
				project = args[0]
			}
			return runWikiRepoInfo(cmd.OutOrStdout(), project)
		},
	}
}

// runWikiRepoInfo 输出项目来源配置；无 project 时输出已登记项目列表。
func runWikiRepoInfo(out io.Writer, project string) error {
	if strings.TrimSpace(project) == "" {
		slugs, err := common.ListWikiRepoSlugs()
		if err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(slugs)
	}
	cfg, err := common.LoadWikiRepoConfig(common.WikiSlug(project))
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(cfg)
}
