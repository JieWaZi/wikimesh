package qmdcmd

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"

	"github.com/JieWaZi/wikimesh/internal/cli/common"
	"github.com/JieWaZi/wikimesh/internal/ui"
	"github.com/JieWaZi/wikimesh/pkg/qmd"
)

type qmdUpdateOptions struct {
	pull        bool
	collections []string
}

func newStatusCommand() *cobra.Command {
	msg := ui.Messages()
	return &cobra.Command{
		Use:   "status",
		Short: msg.QMDStatusShort,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, configPath := workspaceQMDConfigPath()
			return runQMDStatus(cmd.Context(), cmd.OutOrStdout(), configPath)
		},
	}
}

func newUpdateCommand() *cobra.Command {
	msg := ui.Messages()
	var opts qmdUpdateOptions
	cmd := &cobra.Command{
		Use:   "update",
		Short: msg.QMDUpdateShort,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, configPath := workspaceQMDConfigPath()
			return runQMDUpdate(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), configPath, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.pull, "pull", false, msg.FlagPull)
	cmd.Flags().StringArrayVarP(&opts.collections, "collection", "c", nil, msg.FlagCollectionFilter)
	return cmd
}

// runQMDUpdate 刷新当前工作区内一个或多个 qmd 集合索引。
func runQMDUpdate(ctx context.Context, out, errOut io.Writer, configPath string, opts qmdUpdateOptions) error {
	cfg, err := common.LoadQMDConfig(configPath)
	if err != nil {
		return err
	}
	if root, ok := qmdConfigRoot(configPath); ok {
		absolutizeQMDConfig(root, &cfg)
	}
	store, err := common.OpenQMDStoreFromConfig(ctx, cfg)
	if err != nil {
		return err
	}
	defer store.Close()

	targets := uniqueNonEmpty(opts.collections)
	if len(targets) == 0 {
		collections, err := store.ListCollections(ctx)
		if err != nil {
			return err
		}
		for _, collection := range collections {
			targets = append(targets, collection.Name)
		}
	}
	if len(targets) == 0 {
		fmt.Fprintln(out, ui.Messages().OutputQMDNoCollections)
		return nil
	}

	msg := ui.Messages()
	fmt.Fprintf(out, msg.OutputQMDUpdateStartFmt, len(targets))
	changed := false
	for _, name := range targets {
		fmt.Fprintf(out, msg.OutputQMDCollectionFmt, name)
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
		result, err := store.UpdateCollection(ctx, name, qmd.UpdateOptions{RunUpdateCommand: opts.pull, Progress: progress})
		if err != nil {
			return err
		}
		if bar != nil {
			_ = bar.Finish()
		}
		if result.Indexed > 0 || result.Removed > 0 {
			changed = true
		}
		fmt.Fprintf(out, msg.OutputQMDIndexedFmt, result.Indexed, result.Skipped, result.Removed)
	}
	if changed {
		fmt.Fprint(out, msg.OutputQMDEmbedHint)
	}
	fmt.Fprint(out, msg.OutputQMDUpdateDone)
	return nil
}

// runQMDStatus 输出当前工作区 qmd 索引和集合状态。
func runQMDStatus(ctx context.Context, out io.Writer, configPath string) error {
	cfg, err := common.LoadQMDConfig(configPath)
	if err != nil {
		return err
	}
	if root, ok := qmdConfigRoot(configPath); ok {
		absolutizeQMDConfig(root, &cfg)
	}
	store, err := common.OpenQMDStoreFromConfig(ctx, cfg)
	if err != nil {
		return err
	}
	defer store.Close()

	status, err := store.Status(ctx)
	if err != nil {
		return err
	}
	msg := ui.Messages()
	fmt.Fprintln(out, msg.OutputQMDStatusTitle)
	fmt.Fprintf(out, msg.OutputQMDStatusIndexFmt, status.DBPath)
	fmt.Fprintf(out, msg.OutputQMDStatusDocumentsFmt, status.TotalDocuments, status.VectorCount)
	fmt.Fprintf(out, msg.OutputQMDStatusPendingFmt, status.NeedsEmbedding)
	if len(status.Collections) == 0 {
		fmt.Fprintln(out, msg.OutputQMDNoCollections)
		return nil
	}
	fmt.Fprintln(out, msg.OutputQMDStatusCollections)
	for _, collection := range status.Collections {
		lastModified := collection.LastModified
		if lastModified == "" {
			lastModified = "never"
		}
		fmt.Fprintf(out, msg.OutputQMDStatusCollectionFmt, collection.Name, collection.ActiveCount, collection.Pattern, lastModified)
		if len(collection.Context) > 0 {
			fmt.Fprintf(out, msg.OutputQMDStatusCollectionContextFmt, len(collection.Context))
		}
	}
	return nil
}
