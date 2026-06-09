package qmdcmd

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/JieWaZi/wikimesh/pkg/qmd"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/JieWaZi/wikimesh/internal/ui"
)

type queryCLIOptions struct {
	question       string
	queriesJSON    string
	searchQueries  []string
	intent         string
	limit          int
	minScore       float64
	candidateLimit int
	explain        bool
	noRerank       bool
}

func newQueryCommand() *cobra.Command {
	msg := ui.Messages()
	var opts queryCLIOptions
	var collections []string
	cmd := &cobra.Command{
		Use:   "query <question...>",
		Short: msg.QueryShort,
		Args: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(opts.queriesJSON) != "" || len(opts.searchQueries) > 0 {
				return nil
			}
			return cobra.MinimumNArgs(1)(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			store, _, _, err := openWorkspaceStore(cmd.Context())
			if err != nil {
				return err
			}
			defer store.Close()
			opts.question = strings.Join(args, " ")
			return runTopLevelQuery(cmd.Context(), cmd.OutOrStdout(), store, collections, opts)
		},
	}
	cmd.Flags().IntVarP(&opts.limit, "limit", "n", 10, msg.FlagLimit)
	cmd.Flags().Float64Var(&opts.minScore, "min-score", 0, msg.FlagMinScore)
	cmd.Flags().StringArrayVarP(&collections, "collection", "c", nil, msg.FlagCollectionFilter)
	cmd.Flags().StringVar(&opts.queriesJSON, "queries", "", msg.FlagQueries)
	cmd.Flags().StringArrayVar(&opts.searchQueries, "search-query", nil, msg.FlagSearchQuery)
	cmd.Flags().StringVar(&opts.intent, "intent", "", msg.FlagIntent)
	cmd.Flags().IntVar(&opts.candidateLimit, "candidate-limit", 0, msg.FlagCandidateLimit)
	cmd.Flags().BoolVar(&opts.explain, "explain", false, msg.FlagExplain)
	cmd.Flags().BoolVar(&opts.noRerank, "no-rerank", false, msg.FlagNoRerank)
	return cmd
}

// runTopLevelQuery 使用 qmd store 执行多集合混合查询并输出 JSON。
func runTopLevelQuery(ctx context.Context, out io.Writer, store *qmd.Store, collections []string, opts queryCLIOptions) error {
	targets, err := resolveSearchCollections(ctx, store, collections)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return printJSON(out, qmd.QueryResult{Question: opts.question})
	}
	if opts.limit <= 0 {
		opts.limit = 10
	}
	queries, err := parseQueryExpansions(opts.queriesJSON)
	if err != nil {
		return err
	}

	result := qmd.QueryResult{Question: opts.question}
	perCollectionLimit := opts.limit
	if len(targets) > 1 {
		perCollectionLimit = maxInt(20, opts.limit*2)
	}
	for _, collection := range targets {
		partial, err := store.Query(ctx, collection, opts.question, qmd.QueryOptions{
			Limit:          perCollectionLimit,
			MinScore:       opts.minScore,
			CandidateLimit: opts.candidateLimit,
			Explain:        opts.explain,
			SkipRerank:     opts.noRerank,
			Intent:         opts.intent,
			Queries:        queries,
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
	return printJSON(out, result)
}

// resolveSearchCollections 根据命令参数和配置默认集合计算搜索目标。
func resolveSearchCollections(ctx context.Context, store *qmd.Store, collections []string) ([]string, error) {
	if len(collections) > 0 {
		return uniqueNonEmpty(collections), nil
	}
	return store.DefaultCollectionNames(ctx)
}

// parseQueryExpansions 解析命令行传入的查询扩展 JSON。
func parseQueryExpansions(raw string) ([]qmd.QueryExpansion, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var queries []qmd.QueryExpansion
	if err := json.Unmarshal([]byte(raw), &queries); err != nil {
		return nil, fmt.Errorf("parse --queries JSON: %w", err)
	}
	for i := range queries {
		if queries[i].Query == "" {
			queries[i].Query = queries[i].Text
		}
		if queries[i].Text == "" {
			queries[i].Text = queries[i].Query
		}
	}
	return queries, nil
}
