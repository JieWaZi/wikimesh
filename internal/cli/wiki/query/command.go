package querycmd

import (
	"context"
	"encoding/json"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/JieWaZi/wikimesh/internal/app/qmdapp"
	"github.com/JieWaZi/wikimesh/internal/app/wikiapp"
	"github.com/JieWaZi/wikimesh/internal/ui"
	"github.com/JieWaZi/wikimesh/pkg/qmd"
)

type queryOptions struct {
	question       string
	searchQueries  []string
	intent         string
	limit          int
	minScore       float64
	candidateLimit int
	explain        bool
	noRerank       bool
}

// NewCommand 构造 `wikimesh query` 命令。
func NewCommand() *cobra.Command {
	msg := ui.Messages()
	var root, project string
	var collections []string
	var opts queryOptions
	cmd := &cobra.Command{
		Use:   "query <question...>",
		Short: msg.WikiQueryShort,
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.question = strings.Join(args, " ")
			return runWikiQuery(cmd.Context(), cmd.OutOrStdout(), root, project, collections, opts)
		},
	}
	cmd.Flags().StringVar(&root, "root", ".", msg.FlagRoot)
	cmd.Flags().StringVar(&project, "project", "", msg.FlagProject)
	cmd.Flags().IntVarP(&opts.limit, "limit", "n", 10, msg.FlagLimit)
	cmd.Flags().Float64Var(&opts.minScore, "min-score", 0, msg.FlagMinScore)
	cmd.Flags().StringArrayVarP(&collections, "collection", "c", nil, msg.FlagCollectionFilter)
	cmd.Flags().StringArrayVar(&opts.searchQueries, "search-query", nil, msg.FlagSearchQuery)
	cmd.Flags().StringVar(&opts.intent, "intent", "", msg.FlagIntent)
	cmd.Flags().IntVar(&opts.candidateLimit, "candidate-limit", 0, msg.FlagCandidateLimit)
	cmd.Flags().BoolVar(&opts.explain, "explain", false, msg.FlagExplain)
	cmd.Flags().BoolVar(&opts.noRerank, "no-rerank", false, msg.FlagNoRerank)
	return cmd
}

// runWikiQuery 解析 Wikimesh project/root 后执行 qmd 混合查询。
func runWikiQuery(ctx context.Context, out io.Writer, root, project string, collections []string, opts queryOptions) error {
	resolvedRoot, err := wikiapp.ResolveRoot(root, project)
	if err != nil {
		return err
	}
	cfg, err := qmdapp.LoadConfig(qmdapp.ConfigPathForRoot(resolvedRoot))
	if err != nil {
		return err
	}
	if err := absolutizeQMDConfig(resolvedRoot, &cfg); err != nil {
		return err
	}
	store, err := qmdapp.OpenStoreFromConfig(ctx, cfg)
	if err != nil {
		return err
	}
	defer store.Close()
	return runQueryAgainstStore(ctx, out, store, collections, opts)
}

// absolutizeQMDConfig 把工作区相对 qmd 路径转为绝对路径，支持 --root 从任意目录查询。
func absolutizeQMDConfig(root string, cfg *qmd.FileConfig) error {
	return wikiapp.AbsolutizeQMDConfig(root, cfg)
}

// runQueryAgainstStore 在一个或多个 collection 上查询，并合并排序输出。
func runQueryAgainstStore(ctx context.Context, out io.Writer, store *qmd.Store, collections []string, opts queryOptions) error {
	targets, err := resolveQueryCollections(ctx, store, collections)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return printQueryJSON(out, qmd.QueryResult{Question: opts.question})
	}
	if opts.limit <= 0 {
		opts.limit = 10
	}

	result := qmd.QueryResult{Question: opts.question}
	perCollectionLimit := opts.limit
	if len(targets) > 1 {
		perCollectionLimit = maxQueryInt(20, opts.limit*2)
	}
	for _, collection := range targets {
		partial, err := store.Query(ctx, collection, opts.question, qmd.QueryOptions{
			Limit:          perCollectionLimit,
			MinScore:       opts.minScore,
			CandidateLimit: opts.candidateLimit,
			Explain:        opts.explain,
			SkipRerank:     opts.noRerank,
			Intent:         opts.intent,
			SearchQueries:  opts.searchQueries,
		})
		if err != nil {
			return err
		}
		result.Results = append(result.Results, partial.Results...)
	}
	sort.SliceStable(result.Results, func(i, j int) bool {
		if result.Results[i].Score == result.Results[j].Score {
			if result.Results[i].Collection == result.Results[j].Collection {
				return result.Results[i].Path < result.Results[j].Path
			}
			return result.Results[i].Collection < result.Results[j].Collection
		}
		return result.Results[i].Score > result.Results[j].Score
	})
	if len(result.Results) > opts.limit {
		result.Results = result.Results[:opts.limit]
	}
	return printQueryJSON(out, result)
}

// resolveQueryCollections 根据显式参数或 qmd 默认集合决定查询范围。
func resolveQueryCollections(ctx context.Context, store *qmd.Store, collections []string) ([]string, error) {
	if len(collections) > 0 {
		return uniqueQueryNonEmpty(collections), nil
	}
	return store.DefaultCollectionNames(ctx)
}

func uniqueQueryNonEmpty(items []string) []string {
	seen := map[string]bool{}
	var result []string
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		result = append(result, item)
	}
	return result
}

func maxQueryInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func printQueryJSON(out io.Writer, v any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
