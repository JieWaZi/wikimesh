package repocmd

import (
	"github.com/spf13/cobra"

	"github.com/JieWaZi/wikimesh/internal/ui"
)

// NewCommand 构造 `wikimesh repo` 命令。
func NewCommand() *cobra.Command {
	msg := ui.Messages()
	cmd := &cobra.Command{
		Use:   "repo",
		Short: msg.WikiRepoShort,
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}
	cmd.AddCommand(newAddCommand())
	cmd.AddCommand(newLinkCommand())
	cmd.AddCommand(newUseCommand())
	cmd.AddCommand(newInfoCommand())
	return cmd
}
