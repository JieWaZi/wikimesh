package repocmd

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/JieWaZi/wikimesh/internal/app/wikiapp"
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

// runWikiRepoAdd 新增或更新一个 Wikimesh 项目的本地/远端来源。
func runWikiRepoAdd(out io.Writer, project, path, remoteURL string) error {
	slug := wikiapp.Slug(project)
	if slug == "" {
		return fmt.Errorf("project is required")
	}
	cfg := wikiapp.RepoConfig{ProjectName: project, ProjectSlug: slug, Language: "zh", ActiveSource: wikiapp.SourceLocal}
	if existing, err := wikiapp.LoadRepoConfig(slug); err == nil {
		cfg = existing
	}
	if strings.TrimSpace(remoteURL) != "" {
		cfg.Sources.Remote = &wikiapp.RepoSource{Type: wikiapp.SourceRemote, URL: remoteURL}
		cfg.ActiveSource = wikiapp.SourceRemote
	} else {
		abs, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		cfg.Sources.Local = &wikiapp.RepoSource{Type: wikiapp.SourceLocal, Path: abs}
		cfg.ActiveSource = wikiapp.SourceLocal
	}
	if cfg.ProjectName == "" {
		cfg.ProjectName = project
	}
	if cfg.Language == "" {
		cfg.Language = "zh"
	}
	if err := wikiapp.SaveRepoConfig(cfg); err != nil {
		return err
	}
	_, err := fmt.Fprintf(out, ui.Messages().OutputWikiRepoSavedFmt, slug)
	return err
}

// runWikiRepoInfo 输出项目来源配置；无 project 时输出已登记项目列表。
func runWikiRepoInfo(out io.Writer, project string) error {
	if strings.TrimSpace(project) == "" {
		slugs, err := wikiapp.ListRepoSlugs()
		if err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(slugs)
	}
	cfg, err := wikiapp.LoadRepoConfig(wikiapp.Slug(project))
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(cfg)
}

// runWikiRepoLink 把代码仓路径记录到指定 Wikimesh 项目配置中。
func runWikiRepoLink(out io.Writer, project, repoSlug, path string) error {
	cfg, err := wikiapp.LoadRepoConfig(wikiapp.Slug(project))
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	repo := wikiapp.CodeRepo{Name: filepath.Base(abs), Slug: repoSlug, Path: abs}
	replaced := false
	for i := range cfg.CodeRepos {
		if cfg.CodeRepos[i].Slug == repoSlug {
			// 同 slug 视为更新已有代码库路径，避免生成重复关联。
			cfg.CodeRepos[i] = repo
			replaced = true
			break
		}
	}
	if !replaced {
		cfg.CodeRepos = append(cfg.CodeRepos, repo)
	}
	if err := wikiapp.SaveRepoConfig(cfg); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, ui.Messages().OutputWikiRepoLinkedFmt, repoSlug)
	return err
}

// runWikiRepoUse 切换项目当前激活的知识来源。
func runWikiRepoUse(out io.Writer, project, sourceType string) error {
	cfg, err := wikiapp.LoadRepoConfig(wikiapp.Slug(project))
	if err != nil {
		return err
	}
	switch sourceType {
	case wikiapp.SourceLocal:
		if cfg.Sources.Local == nil {
			return fmt.Errorf("local source is not configured")
		}
	case wikiapp.SourceRemote:
		if cfg.Sources.Remote == nil {
			return fmt.Errorf("remote source is not configured")
		}
	default:
		return fmt.Errorf("unsupported source type %q", sourceType)
	}
	cfg.ActiveSource = sourceType
	if err := wikiapp.SaveRepoConfig(cfg); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, ui.Messages().OutputWikiRepoActiveFmt, sourceType)
	return err
}
