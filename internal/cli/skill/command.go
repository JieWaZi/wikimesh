package skillcmd

import (
	"fmt"
	"github.com/JieWaZi/wikimesh/internal/app/skillapp"
	"github.com/spf13/cobra"
	"io"

	"github.com/JieWaZi/wikimesh/internal/ui"
)

// NewCommand 构造 `wikimesh skill` 命令。
func NewCommand() *cobra.Command {
	msg := ui.Messages()
	cmd := &cobra.Command{
		Use:   "skill",
		Short: msg.WikiSkillShort,
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}
	cmd.AddCommand(newInstallCommand())
	return cmd
}

func newInstallCommand() *cobra.Command {
	msg := ui.Messages()
	var agent string
	var wikiType string
	cmd := &cobra.Command{
		Use:   "install [version]",
		Short: msg.WikiSkillInstallShort,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref := ""
			if len(args) > 0 {
				ref = args[0]
			}
			return runWikiSkillInstall(cmd.OutOrStdout(), agent, wikiType, ref)
		},
	}
	cmd.Flags().StringVar(&agent, "agent", "codex", msg.FlagAgent)
	cmd.Flags().StringVar(&wikiType, "type", skillapp.DefaultType(), msg.FlagWikiType)
	return cmd
}

// runWikiSkillInstall 从 GitHub 安装最新 Wikimesh runtime skills。
func runWikiSkillInstall(out io.Writer, agent string, wikiType string, ref string) error {
	if wikiType == "" {
		wikiType = skillapp.DefaultType()
	}
	bundle, err := skillapp.ResolveWikimeshSkills(wikiType, ref)
	if err != nil {
		return err
	}
	if bundle.Cleanup != nil {
		defer func() { _ = bundle.Cleanup() }()
	}
	if err := skillapp.Install(agent, false, bundle.Skills); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, ui.Messages().OutputWikiSkillsInstalledFmt, agent, wikiType, len(bundle.Skills))
	return err
}
