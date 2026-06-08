package qmdcmd

import (
	"context"
	"fmt"
	"github.com/JieWaZi/wikimesh/pkg/qmd"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
	"io"
	"time"

	"github.com/JieWaZi/wikimesh/internal/app/qmdapp"
	"github.com/JieWaZi/wikimesh/internal/ui"
)

type embedCLIOptions struct {
	collections []string
	provider    string
	model       string
	command     string
	dimensions  int
	force       bool
}

func newEmbedCommand() *cobra.Command {
	msg := ui.Messages()
	var opts embedCLIOptions
	cmd := &cobra.Command{
		Use:   "embed",
		Short: msg.EmbedShort,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, configPath := workspaceQMDConfigPath()
			return runEmbed(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), configPath, opts)
		},
	}
	cmd.Flags().BoolVarP(&opts.force, "force", "f", false, msg.FlagForce)
	cmd.Flags().StringArrayVarP(&opts.collections, "collection", "c", nil, msg.FlagCollectionFilter)
	cmd.Flags().StringVar(&opts.provider, "provider", "", msg.FlagProvider)
	cmd.Flags().StringVar(&opts.model, "model", "", msg.FlagModel)
	cmd.Flags().StringVar(&opts.command, "command", "", msg.FlagCommand)
	cmd.Flags().IntVar(&opts.dimensions, "dimensions", 0, msg.FlagDimensions)
	return cmd
}

// runEmbed 为已索引文档生成 embedding 向量。
func runEmbed(ctx context.Context, out, errOut io.Writer, configPath string, opts embedCLIOptions) error {
	cfg, err := qmdapp.LoadConfig(configPath)
	if err != nil {
		return err
	}
	if root, ok := qmdConfigRoot(configPath); ok {
		absolutizeQMDConfig(root, &cfg)
	}
	if opts.provider != "" {
		cfg.Embedding.Provider = opts.provider
	}
	if opts.model != "" {
		cfg.Embedding.Model = opts.model
	}
	if opts.command != "" {
		cfg.Embedding.Command = opts.command
	}
	if opts.dimensions > 0 {
		cfg.Embedding.Dimensions = opts.dimensions
	}
	if err := ensureModelForRole(ctx, cfg, qmd.ModelRoleEmbed, out, errOut); err != nil {
		return err
	}

	store, err := qmdapp.OpenStoreFromConfig(ctx, cfg)
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
		for _, c := range collections {
			targets = append(targets, c.Name)
		}
	}
	if len(targets) == 0 {
		fmt.Fprintln(out, ui.Messages().OutputQMDNoCollections)
		return nil
	}

	model := store.EmbeddingModelName()
	if model == "" {
		return fmt.Errorf("embedding provider is not configured")
	}
	copy := ui.Messages()
	fmt.Fprintf(out, copy.OutputQMDModelFmt, model)

	total := qmd.EmbedResult{}
	for i, collection := range targets {
		fmt.Fprintf(out, "[%d/%d] %s\n", i+1, len(targets), collection)
		var bar *progressbar.ProgressBar
		progress := func(info qmd.EmbedProgress) {
			if !writerIsTerminal(errOut) || info.Total <= 0 {
				return
			}
			if bar == nil {
				bar = progressbar.NewOptions(info.Total,
					progressbar.OptionSetWriter(errOut),
					progressbar.OptionSetDescription("Embedding"),
					progressbar.OptionSetWidth(28),
					progressbar.OptionShowCount(),
					progressbar.OptionClearOnFinish(),
					progressbar.OptionThrottle(100*time.Millisecond),
				)
			}
			_ = bar.Set(info.Current)
		}
		result, err := store.EmbedCollection(ctx, collection, qmd.EmbedOptions{Force: opts.force, Progress: progress})
		if err != nil {
			return err
		}
		if bar != nil {
			_ = bar.Finish()
		}
		total.Scanned += result.Scanned
		total.Skipped += result.Skipped
		total.Embedded += result.Embedded
		fmt.Fprintf(out, copy.OutputQMDEmbeddedFmt, result.Embedded, result.Scanned, result.Skipped)
	}
	if len(targets) > 1 {
		fmt.Fprintf(out, copy.OutputQMDEmbeddedTotalFmt, total.Embedded, total.Scanned, total.Skipped)
	}
	fmt.Fprint(out, copy.OutputQMDEmbedDone)
	return nil
}

// ensureModelForRole 确保指定角色的本地模型已经存在。
func ensureModelForRole(ctx context.Context, cfg qmd.FileConfig, role qmd.ModelRole, out, errOut io.Writer) error {
	_, err := downloadConfiguredModel(ctx, cfg, role, out, errOut)
	return err
}
