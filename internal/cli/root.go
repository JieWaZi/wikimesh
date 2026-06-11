package cli

import (
	"github.com/spf13/cobra"

	agentcmd "github.com/JieWaZi/wikimesh/internal/cli/agent"
	daemoncmd "github.com/JieWaZi/wikimesh/internal/cli/daemon"
	qmdcmd "github.com/JieWaZi/wikimesh/internal/cli/qmd"
	skillcmd "github.com/JieWaZi/wikimesh/internal/cli/skill"
	updatecmd "github.com/JieWaZi/wikimesh/internal/cli/update"
	"github.com/JieWaZi/wikimesh/internal/cli/wiki"
	"github.com/JieWaZi/wikimesh/internal/ui"
)

// NewRootCmd 构建 Wikimesh 根命令，并直接挂载所有一级命令。
func NewRootCmd() *cobra.Command {
	msg := ui.Messages()
	root := &cobra.Command{
		Use:           "wikimesh",
		Short:         msg.RootShort,
		SilenceUsage:  true,
		SilenceErrors: true,
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.AddCommand(wiki.Commands()...)
	root.AddCommand(agentcmd.NewCommand())
	root.AddCommand(daemoncmd.NewCommand())
	root.AddCommand(updatecmd.NewCommand())
	root.AddCommand(skillcmd.NewCommand())
	root.AddCommand(qmdcmd.NewCommand())
	ui.ApplyLocalizedHelp(root)
	return root
}
