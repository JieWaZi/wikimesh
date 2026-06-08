package ui

import (
	"fmt"
	"io"
	"path/filepath"
	"sync"

	"github.com/spf13/cobra"
)

// showLogoOnce 保证同一次进程内多次 help 只输出一次 logo。
var showLogoOnce sync.Once

// ApplyLocalizedHelp 为整棵 Cobra 命令树安装中文 help 渲染器。
func ApplyLocalizedHelp(root *cobra.Command) {
	root.SetHelpCommand(&cobra.Command{Hidden: true})
	applyLocalizedHelpTo(root)
}

// applyLocalizedHelpTo 递归处理子命令，保证新增命令也使用同一套中文 help。
func applyLocalizedHelpTo(cmd *cobra.Command) {
	cmd.InitDefaultHelpFlag()
	if flag := cmd.Flags().Lookup("help"); flag != nil {
		flag.Usage = "显示帮助"
	}
	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		showLogoOnce.Do(func() {
			fmt.Fprint(cmd.OutOrStdout(), Logo())
		})
		writeLocalizedHelp(cmd.OutOrStdout(), cmd)
	})
	for _, child := range cmd.Commands() {
		applyLocalizedHelpTo(child)
	}
}

// writeLocalizedHelp 输出中文化的命令说明、用法、子命令和参数。
func writeLocalizedHelp(out io.Writer, cmd *cobra.Command) {
	copy := Messages()
	if cmd.Short != "" {
		fmt.Fprintln(out, cmd.Short)
		fmt.Fprintln(out)
	}
	fmt.Fprintf(out, "%s:\n  %s\n\n", copy.HelpUsage, cmd.UseLine())
	if children := visibleCommands(cmd); len(children) > 0 {
		fmt.Fprintf(out, "%s:\n", copy.HelpAvailableCommands)
		for _, child := range children {
			fmt.Fprintf(out, "  %-12s %s\n", child.Name(), child.Short)
		}
		fmt.Fprintln(out)
	}
	if cmd.HasLocalFlags() {
		fmt.Fprintf(out, "%s:\n%s\n", copy.HelpFlags, cmd.LocalFlags().FlagUsagesWrapped(80))
	}
	if cmd.HasInheritedFlags() {
		fmt.Fprintf(out, "%s:\n%s\n", copy.HelpGlobalFlags, cmd.InheritedFlags().FlagUsagesWrapped(80))
	}
	if cmd.HasSubCommands() {
		fmt.Fprintf(out, copy.HelpMoreInfoFmt+"\n", filepath.Base(cmd.CommandPath())+" [command]")
	}
}

// visibleCommands 返回 help 中应展示的子命令。
func visibleCommands(cmd *cobra.Command) []*cobra.Command {
	var out []*cobra.Command
	for _, child := range cmd.Commands() {
		if !child.Hidden {
			out = append(out, child)
		}
	}
	return out
}
