package initcmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/JieWaZi/wikimesh/internal/app/wikiapp"
	"github.com/JieWaZi/wikimesh/internal/app/wikiinit"
	"github.com/JieWaZi/wikimesh/internal/ui"
)

var (
	wikiSelectOne         = ui.SelectOne
	wikiSearchMultiselect = ui.SearchMultiselect
)

const (
	// InitModeCreate 表示在当前目录新建 Wikimesh 文档库。
	InitModeCreate = wikiinit.ModeCreate
	// InitModeLink 表示登记并关联已有 Wikimesh 文档库。
	InitModeLink = wikiinit.ModeLink
)

// NewCommand 构造 `wikimesh init` 命令。
func NewCommand() *cobra.Command {
	msg := ui.Messages()
	var opts InitOptions
	cmd := &cobra.Command{
		Use:   "init [project-name]",
		Short: msg.WikiInitShort,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.ProjectName = args[0]
			}
			opts.Mode = strings.TrimSpace(opts.Mode)
			opts.ScopeProvided = cmd.Flags().Changed("global")
			opts.WikiTypeProvided = cmd.Flags().Changed("type")
			if !opts.Yes {
				if err := collectInitOptions(cmd.InOrStdin(), cmd.OutOrStdout(), readerIsTerminal(cmd.InOrStdin()), &opts, cmd.Flags().Changed("agent"), cmd.Flags().Changed("code-dir")); err != nil {
					return err
				}
			}
			return runWikiInit(cmd.Context(), cmd.OutOrStdout(), readerIsTerminal(cmd.InOrStdin()), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Agent, "agent", "codex", msg.FlagAgent)
	cmd.Flags().StringVar(&opts.WikiType, "type", "", msg.FlagWikiType)
	cmd.Flags().StringSliceVar(&opts.CodeDirs, "code-dir", nil, msg.FlagCodeDir)
	cmd.Flags().StringVar(&opts.Mode, "mode", "", msg.FlagWikiInitMode)
	cmd.Flags().StringVar(&opts.SourceType, "source", "", msg.FlagWikiRepoSource)
	cmd.Flags().StringVar(&opts.LocalPath, "path", "", msg.FlagPath)
	cmd.Flags().StringVar(&opts.RemoteURL, "remote", "", msg.FlagRemote)
	cmd.Flags().BoolVarP(&opts.Global, "global", "g", false, msg.FlagGlobal)
	cmd.Flags().BoolVarP(&opts.Yes, "yes", "y", false, msg.FlagYes)
	return cmd
}

func readerIsTerminal(r any) bool {
	file, ok := r.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(file.Fd()))
}

// runWikiInit 创建工作区或登记已有文档库，并安装 runtime skills。
func runWikiInit(ctx context.Context, out io.Writer, interactive bool, opts InitOptions) error {
	msg := ui.Messages()
	resolved, err := normalizeWikiInitOptions(opts)
	if err != nil {
		return err
	}
	if resolved.Mode == InitModeLink {
		return runWikiInitLink(out, interactive, resolved)
	}
	selectedSkills, cleanupSkills, err := selectedWikiInitSkills(&resolved, interactive)
	if err != nil {
		return err
	}
	if cleanupSkills != nil {
		defer func() { _ = cleanupSkills() }()
	}
	targetDir, err := wikiinit.CreateTargetDir(resolved.ProjectName)
	if err != nil {
		return err
	}

	ui.Note(msg.TitleWikiSummary, []string{
		fmt.Sprintf("%s: %s", msg.ProjectLabel, resolved.ProjectName),
		fmt.Sprintf("%s: %s", msg.WikiTypeLabel, resolved.WikiType),
		fmt.Sprintf("%s: %s", msg.SourceLabel, targetDir),
		fmt.Sprintf("%s: %s", msg.AgentLabel, strings.Join(resolved.Agents, ", ")),
		fmt.Sprintf("%s: %s", msg.WikiCodeDirsLabel, strings.Join(resolved.CodeDirs, ", ")),
		fmt.Sprintf("%s: %s", msg.ScopeLabel, ui.ScopeText(resolved.Global)),
	})

	spinner := ui.NewStepPrinter()
	spinner.Start(msg.StepCreatingWikiProject)
	appOpts := resolved.appOptions()
	if err := wikiinit.CreateWorkspace(ctx, targetDir, appOpts); err != nil {
		return err
	}
	if err := wikiinit.SaveRepoConfig(targetDir, appOpts); err != nil {
		return err
	}
	spinner.Stop(fmt.Sprintf(msg.CreatedFmt, targetDir))

	if len(selectedSkills) > 0 {
		spinner.Start(msg.StepInstallingWikiSkills)
		if err := wikiinit.InstallSkillsForAgentsInProject(targetDir, resolved.Agents, resolved.Global, selectedSkills); err != nil {
			return err
		}
		spinner.Stop(fmt.Sprintf(msg.WikiInstalledSkillsFmt, resolved.WikiType, len(selectedSkills)))
	}

	if err := wikiinit.EnsureGitignore(targetDir, appOpts); err != nil {
		return err
	}

	fmt.Printf("%s%s%s\n", ui.Green, msg.Done, ui.Reset)
	ui.Note(msg.TitleQMDManualDownload, []string{
		msg.QMDManualDownloadHint,
		msg.QMDManualDownloadCommand,
	})
	_, _ = out.Write(nil)
	return nil
}

// runWikiInitLink 只登记已有文档库来源和可选代码库，不创建当前目录工作区。
func runWikiInitLink(out io.Writer, interactive bool, opts InitOptions) error {
	selectedSkills, cleanupSkills, err := selectedWikiInitSkills(&opts, interactive)
	if err != nil {
		return err
	}
	if cleanupSkills != nil {
		defer func() { _ = cleanupSkills() }()
	}
	if err := wikiinit.NewService().Link(opts.appOptions(), selectedSkills); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, ui.Messages().OutputWikiRepoSavedFmt, wikiapp.Slug(opts.ProjectName))
	return err
}
