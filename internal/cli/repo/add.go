package repocmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"io"
	"path/filepath"
	"strings"

	"github.com/JieWaZi/wikimesh/internal/cli/common"
	"github.com/JieWaZi/wikimesh/internal/ui"
)

func newAddCommand() *cobra.Command {
	copy := ui.Messages()
	var remoteURL string
	cmd := &cobra.Command{
		Use:   "add <project> [path]",
		Short: copy.WikiRepoAddShort,
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) > 1 {
				path = args[1]
			}
			return runWikiRepoAdd(cmd.OutOrStdout(), args[0], path, remoteURL)
		},
	}
	cmd.Flags().StringVar(&remoteURL, "remote", "", copy.FlagRemote)
	return cmd
}

// runWikiRepoAdd 新增或更新一个 Wikimesh 项目的本地/远端来源。
func runWikiRepoAdd(out io.Writer, project, path, remoteURL string) error {
	slug := common.WikiSlug(project)
	if slug == "" {
		return fmt.Errorf("project is required")
	}
	cfg := common.WikiRepoConfig{ProjectName: project, ProjectSlug: slug, Language: "zh", ActiveSource: common.WikiRepoSourceLocal}
	if existing, err := common.LoadWikiRepoConfig(slug); err == nil {
		cfg = existing
	}
	if strings.TrimSpace(remoteURL) != "" {
		cfg.Sources.Remote = &common.WikiRepoSource{Type: common.WikiRepoSourceRemote, URL: remoteURL}
		cfg.ActiveSource = common.WikiRepoSourceRemote
	} else {
		abs, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		cfg.Sources.Local = &common.WikiRepoSource{Type: common.WikiRepoSourceLocal, Path: abs}
		cfg.ActiveSource = common.WikiRepoSourceLocal
	}
	if cfg.ProjectName == "" {
		cfg.ProjectName = project
	}
	if cfg.Language == "" {
		cfg.Language = "zh"
	}
	if err := common.SaveWikiRepoConfig(cfg); err != nil {
		return err
	}
	_, err := fmt.Fprintf(out, ui.Messages().OutputWikiRepoSavedFmt, slug)
	return err
}
