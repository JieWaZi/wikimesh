package qmdcmd

import (
	"context"
	"fmt"
	"github.com/JieWaZi/wikimesh/pkg/qmd"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/JieWaZi/wikimesh/internal/cli/common"
	"github.com/JieWaZi/wikimesh/internal/ui"
)

type collectionAddOptions struct {
	project string
	name    string
	path    string
	mask    string
	include string
}

func newCollectionCommand() *cobra.Command {
	msg := ui.Messages()
	cmd := &cobra.Command{
		Use:   "collection",
		Short: msg.CollectionShort,
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}
	cmd.AddCommand(newCollectionAddCommand())
	cmd.AddCommand(newCollectionListCommand())
	cmd.AddCommand(newCollectionRemoveCommand())
	cmd.AddCommand(newCollectionUpdateCommand())
	return cmd
}

func newCollectionAddCommand() *cobra.Command {
	msg := ui.Messages()
	var project, addName, addPath, addMask, addInclude string
	cmd := &cobra.Command{
		Use:   "add [path]",
		Short: msg.CollectionAddShort,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, cfg, configPath, err := openStoreForProject(cmd.Context(), project)
			if err != nil {
				return err
			}
			defer store.Close()
			path := addPath
			if path == "" && len(args) > 0 {
				path = args[0]
			}
			return runAdd(cmd.Context(), cfg, configPath, store, collectionAddOptions{
				name:    addName,
				path:    path,
				mask:    addMask,
				include: addInclude,
			})
		},
	}
	cmd.Flags().StringVar(&project, "project", "", msg.FlagProject)
	cmd.Flags().StringVar(&addName, "name", "", msg.FlagCollectionName)
	cmd.Flags().StringVar(&addPath, "path", "", msg.FlagCollectionPath)
	cmd.Flags().StringVar(&addMask, "mask", "", msg.FlagCollectionMask)
	cmd.Flags().StringVar(&addInclude, "include", "", msg.FlagCollectionInclude)
	return cmd
}

// runAdd 新增 qmd 集合并同步写回配置文件。
func runAdd(ctx context.Context, cfg qmd.FileConfig, configPath string, store *qmd.Store, opts collectionAddOptions) error {
	if opts.path == "" {
		return fmt.Errorf("collection add requires path")
	}
	name := opts.name
	if name == "" {
		name = filepath.Base(filepath.Clean(opts.path))
		if name == "." || name == string(filepath.Separator) {
			name = "root"
		}
	}
	collection := qmd.Collection{Name: name, Path: opts.path}
	if opts.mask != "" {
		collection.Pattern = opts.mask
	}
	if opts.include != "" {
		collection.Include = strings.Split(opts.include, ",")
	}
	return common.AddQMDCollectionAndSync(ctx, cfg, configPath, store, collection)
}

func newCollectionListCommand() *cobra.Command {
	msg := ui.Messages()
	var project string
	cmd := &cobra.Command{
		Use:   "list",
		Short: msg.CollectionListShort,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, _, _, err := openStoreForProject(cmd.Context(), project)
			if err != nil {
				return err
			}
			defer store.Close()
			return runList(cmd.Context(), cmd.OutOrStdout(), store)
		},
	}
	cmd.Flags().StringVar(&project, "project", "", msg.FlagProject)
	return cmd
}

// runList 列出 qmd 集合并输出 JSON。
func runList(ctx context.Context, out io.Writer, store *qmd.Store) error {
	collections, err := store.ListCollections(ctx)
	if err != nil {
		return err
	}
	if collections == nil {
		collections = []qmd.Collection{}
	}
	return printJSON(out, collections)
}

func newCollectionRemoveCommand() *cobra.Command {
	msg := ui.Messages()
	var project string
	cmd := &cobra.Command{
		Use:     "remove <name>",
		Aliases: []string{"rm"},
		Short:   msg.CollectionRemoveShort,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, cfg, configPath, err := openStoreForProject(cmd.Context(), project)
			if err != nil {
				return err
			}
			defer store.Close()
			return runRemove(cmd.Context(), cmd.OutOrStdout(), cfg, configPath, store, args[0])
		},
	}
	cmd.Flags().StringVar(&project, "project", "", msg.FlagProject)
	return cmd
}

// runRemove 删除 qmd 集合并同步配置文件。
func runRemove(ctx context.Context, out io.Writer, cfg qmd.FileConfig, configPath string, store *qmd.Store, name string) error {
	removed, err := store.RemoveCollection(ctx, name)
	if err != nil {
		return err
	}
	if removed {
		if err := common.SyncQMDConfigFile(ctx, cfg, configPath, store); err != nil {
			return err
		}
	}
	return printJSON(out, map[string]any{"removed": removed})
}

func newCollectionUpdateCommand() *cobra.Command {
	msg := ui.Messages()
	var project string
	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: msg.CollectionUpdateShort,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, _, _, err := openStoreForProject(cmd.Context(), project)
			if err != nil {
				return err
			}
			defer store.Close()
			return runUpdate(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), store, args[0])
		},
	}
	cmd.Flags().StringVar(&project, "project", "", msg.FlagProject)
	return cmd
}

// runUpdate 刷新 qmd 集合索引，并在终端环境显示进度条。
func runUpdate(ctx context.Context, out, errOut io.Writer, store *qmd.Store, name string) error {
	var bar *progressbar.ProgressBar
	progress := func(info qmd.UpdateProgress) {
		if !writerIsTerminal(errOut) || info.Total <= 0 {
			return
		}
		if bar == nil {
			bar = progressbar.NewOptions(info.Total,
				progressbar.OptionSetWriter(errOut),
				progressbar.OptionSetDescription("Indexing"),
				progressbar.OptionSetWidth(28),
				progressbar.OptionShowCount(),
				progressbar.OptionClearOnFinish(),
				progressbar.OptionThrottle(100*time.Millisecond),
			)
		}
		_ = bar.Set(info.Current)
	}
	result, err := store.UpdateCollection(ctx, name, qmd.UpdateOptions{Progress: progress})
	if err != nil {
		return err
	}
	if bar != nil {
		_ = bar.Finish()
	}
	msg := ui.Messages()
	fmt.Fprintf(out, msg.OutputQMDCollectionFmt, name)
	fmt.Fprintf(out, msg.OutputQMDIndexedFmt, result.Indexed, result.Skipped, result.Removed)
	if result.Indexed > 0 || result.Removed > 0 {
		fmt.Fprint(out, msg.OutputQMDEmbedHint)
	}
	return nil
}
