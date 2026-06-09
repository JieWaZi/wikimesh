package qmdcmd

import (
	"context"
	"github.com/JieWaZi/wikimesh/pkg/qmd"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/JieWaZi/wikimesh/internal/ui"
)

type searchOptions struct {
	Stdout      io.Writer
	Collections []string
	Queries     []string
	Limit       int
	MinScore    float64
	All         bool
	Vector      bool
	RawVector   bool
}

func newSearchCommand(vector bool) *cobra.Command {
	msg := ui.Messages()
	use := "search <query...>"
	short := msg.SearchShort
	if vector {
		use = "vsearch <query...>"
		short = msg.VSearchShort
	}
	var opts searchOptions
	opts.Vector = vector
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, _, _, err := openWorkspaceStore(cmd.Context())
			if err != nil {
				return err
			}
			defer store.Close()
			opts.Stdout = cmd.OutOrStdout()
			opts.Queries = args
			return runTopLevelSearch(cmd.Context(), opts.Stdout, store, opts.Collections, opts.Queries, opts.Limit, opts.MinScore, opts.All, opts.Vector, opts.RawVector)
		},
	}
	cmd.Flags().IntVarP(&opts.Limit, "limit", "n", 20, msg.FlagLimit)
	cmd.Flags().BoolVar(&opts.All, "all", false, msg.FlagAll)
	cmd.Flags().Float64Var(&opts.MinScore, "min-score", 0, msg.FlagMinScore)
	cmd.Flags().StringArrayVarP(&opts.Collections, "collection", "c", nil, msg.FlagCollectionFilter)
	if vector {
		cmd.Flags().BoolVar(&opts.RawVector, "raw", false, msg.FlagRawVector)
	}
	return cmd
}

// runTopLevelSearch 使用 qmd store 执行关键词或向量检索并输出 JSON。
func runTopLevelSearch(ctx context.Context, out io.Writer, store *qmd.Store, collections []string, queries []string, limit int, minScore float64, all bool, vector bool, rawVector bool) error {
	targets, err := resolveSearchCollections(ctx, store, collections)
	if err != nil {
		return err
	}
	if all {
		if vector {
			limit = 500
		} else {
			limit = 100000
		}
	} else if limit <= 0 {
		limit = 20
	}

	searchCollection := ""
	if len(targets) == 1 {
		searchCollection = targets[0]
	}
	searchLimit := limit
	if !vector && len(targets) > 1 && !all {
		searchLimit = maxInt(50, limit*2)
	}

	var results []qmd.SearchResult
	if vector {
		query := strings.Join(queries, " ")
		if minScore == 0 && !rawVector {
			minScore = 0.3
		}
		if rawVector {
			results, err = store.SearchVector(ctx, query, qmd.VectorSearchOptions{Collection: searchCollection, Limit: searchLimit, MinScore: minScore})
		} else {
			results, err = store.VSearch(ctx, searchCollection, query, qmd.SearchOptions{Limit: searchLimit, MinScore: minScore})
		}
	} else {
		results, err = store.SearchMany(ctx, searchCollection, queries, qmd.SearchOptions{Limit: searchLimit, MinScore: minScore})
	}
	if err != nil {
		return err
	}
	results = filterSearchCollections(results, targets)
	if len(results) > limit {
		results = results[:limit]
	}
	return printJSON(out, results)
}

// filterSearchCollections 过滤 qmd 搜索结果，避免多集合查询返回未指定集合。
func filterSearchCollections(results []qmd.SearchResult, collections []string) []qmd.SearchResult {
	allowed := map[string]bool{}
	for _, collection := range collections {
		if collection != "" {
			allowed[collection] = true
		}
	}
	if len(allowed) == 0 {
		return results
	}
	filtered := make([]qmd.SearchResult, 0, len(results))
	for _, result := range results {
		if allowed[result.Collection] {
			filtered = append(filtered, result)
		}
	}
	return filtered
}
