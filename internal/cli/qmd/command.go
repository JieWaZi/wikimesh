package qmdcmd

import (
	"github.com/spf13/cobra"

	"github.com/JieWaZi/wikimesh/internal/ui"
)

// NewCommand 构造 `wikimesh qmd` 命令组。
func NewCommand() *cobra.Command {
	msg := ui.Messages()
	cmd := &cobra.Command{
		Use:   "qmd",
		Short: msg.QMDShort,
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}
	cmd.AddCommand(newCollectionCommand())
	cmd.AddCommand(newStatusCommand())
	cmd.AddCommand(newUpdateCommand())
	cmd.AddCommand(newSearchCommand(false))
	cmd.AddCommand(newSearchCommand(true))
	cmd.AddCommand(newQueryCommand())
	cmd.AddCommand(newEmbedCommand())
	cmd.AddCommand(newModelCommand())
	return cmd
}
