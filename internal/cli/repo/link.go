package repocmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"io"
	"path/filepath"

	"github.com/JieWaZi/wikimesh/internal/cli/common"
	"github.com/JieWaZi/wikimesh/internal/ui"
)

func newLinkCommand() *cobra.Command {
	msg := ui.Messages()
	return &cobra.Command{
		Use:   "link <project> <repo-slug> <path>",
		Short: msg.WikiRepoLinkShort,
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWikiRepoLink(cmd.OutOrStdout(), args[0], args[1], args[2])
		},
	}
}

// runWikiRepoLink 把代码仓路径记录到指定 Wikimesh 项目配置中。
func runWikiRepoLink(out io.Writer, project, repoSlug, path string) error {
	cfg, err := common.LoadWikiRepoConfig(common.WikiSlug(project))
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	repo := common.WikiCodeRepo{Name: filepath.Base(abs), Slug: repoSlug, Path: abs}
	replaced := false
	for i := range cfg.CodeRepos {
		if cfg.CodeRepos[i].Slug == repoSlug {
			cfg.CodeRepos[i] = repo
			replaced = true
			break
		}
	}
	if !replaced {
		cfg.CodeRepos = append(cfg.CodeRepos, repo)
	}
	if err := common.SaveWikiRepoConfig(cfg); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, ui.Messages().OutputWikiRepoLinkedFmt, repoSlug)
	return err
}
