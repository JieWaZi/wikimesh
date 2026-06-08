package repocmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"io"

	"github.com/JieWaZi/wikimesh/internal/cli/common"
	"github.com/JieWaZi/wikimesh/internal/ui"
)

func newUseCommand() *cobra.Command {
	msg := ui.Messages()
	return &cobra.Command{
		Use:   "use <project> <local|remote>",
		Short: msg.WikiRepoUseShort,
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWikiRepoUse(cmd.OutOrStdout(), args[0], args[1])
		},
	}
}

// runWikiRepoUse 切换项目当前激活的知识来源。
func runWikiRepoUse(out io.Writer, project, sourceType string) error {
	cfg, err := common.LoadWikiRepoConfig(common.WikiSlug(project))
	if err != nil {
		return err
	}
	switch sourceType {
	case common.WikiRepoSourceLocal:
		if cfg.Sources.Local == nil {
			return fmt.Errorf("local source is not configured")
		}
	case common.WikiRepoSourceRemote:
		if cfg.Sources.Remote == nil {
			return fmt.Errorf("remote source is not configured")
		}
	default:
		return fmt.Errorf("unsupported source type %q", sourceType)
	}
	cfg.ActiveSource = sourceType
	if err := common.SaveWikiRepoConfig(cfg); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, ui.Messages().OutputWikiRepoActiveFmt, sourceType)
	return err
}
