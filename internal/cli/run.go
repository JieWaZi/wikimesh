package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	qmd "github.com/JieWaZi/wikimesh/pkg/qmd"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	app := &cliApp{ctx: ctx, configPath: ".wikimesh/wikimesh.yaml", stdout: stdout, stderr: stderr}
	args = applyRootConfigArgs(args, app)
	cmd := newRootCommand(app)
	cmd.SetArgs(args)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	defer app.close()
	return cmd.ExecuteContext(ctx)
}

func applyRootConfigArgs(args []string, app *cliApp) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			out = append(out, args[i:]...)
			break
		}
		if arg != "" && arg[0] != '-' {
			out = append(out, args[i:]...)
			break
		}
		if arg == "-c" || arg == "--config" {
			if i+1 < len(args) {
				app.configPath = args[i+1]
				i++
			}
			continue
		}
		if strings.HasPrefix(arg, "--config=") {
			app.configPath = strings.TrimPrefix(arg, "--config=")
			continue
		}
		out = append(out, arg)
	}
	return out
}

type cliApp struct {
	ctx        context.Context
	configPath string
	stdout     io.Writer
	stderr     io.Writer
	cfg        qmd.FileConfig
	store      *qmd.Store
	loaded     bool
}

func (a *cliApp) loadStore() (*qmd.Store, error) {
	if a.loaded {
		return a.store, nil
	}
	cfg, err := loadCLIConfig(a.configPath)
	if err != nil {
		return nil, err
	}
	store, err := qmd.NewStore(cfg.StoreConfig())
	if err != nil {
		return nil, err
	}
	for _, c := range cfg.Collections {
		if err := store.AddCollection(a.ctx, c); err != nil {
			store.Close()
			return nil, err
		}
	}
	if strings.TrimSpace(cfg.GlobalContext) != "" {
		if err := store.SetGlobalContext(a.ctx, cfg.GlobalContext); err != nil {
			store.Close()
			return nil, err
		}
	}
	a.cfg = cfg
	a.store = store
	a.loaded = true
	return store, nil
}

func (a *cliApp) close() {
	if a.store != nil {
		a.store.Close()
	}
}

func newRootCommand(app *cliApp) *cobra.Command {
	root := &cobra.Command{
		Use:           "wikimesh",
		Short:         "Wikimesh document collection CLI",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&app.configPath, "config", app.configPath, "configuration file")
	root.AddCommand(newCollectionCommand(app))
	root.AddCommand(newTopLevelSearchCommand(app, false))
	root.AddCommand(newTopLevelSearchCommand(app, true))
	root.AddCommand(newTopLevelQueryCommand(app))
	root.AddCommand(newEmbedCommand(app))
	root.AddCommand(newModelCommand(app))
	return root
}

func newCollectionCommand(app *cliApp) *cobra.Command {
	collection := &cobra.Command{
		Use:   "collection",
		Short: "Manage indexed document collections",
	}

	var addName, addPath, addMask, addInclude string
	add := &cobra.Command{
		Use:   "add [path]",
		Short: "Add a collection",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			store, err := app.loadStore()
			if err != nil {
				return err
			}
			path := addPath
			if path == "" && len(args) > 0 {
				path = args[0]
			}
			return runAdd(app.ctx, app.cfg, app.configPath, store, collectionAddOptions{
				name:    addName,
				path:    path,
				mask:    addMask,
				include: addInclude,
			})
		},
	}
	add.Flags().StringVar(&addName, "name", "", "collection name")
	add.Flags().StringVar(&addPath, "path", "", "collection root path")
	add.Flags().StringVar(&addMask, "mask", "", "glob pattern")
	add.Flags().StringVar(&addInclude, "include", "", "comma-separated include globs")
	collection.AddCommand(add)

	collection.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List collections",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			store, err := app.loadStore()
			if err != nil {
				return err
			}
			return runList(app.ctx, app.stdout, store)
		},
	})

	collection.AddCommand(&cobra.Command{
		Use:     "remove <name>",
		Aliases: []string{"rm"},
		Short:   "Remove a collection",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			store, err := app.loadStore()
			if err != nil {
				return err
			}
			return runRemove(app.ctx, app.stdout, app.cfg, app.configPath, store, args[0])
		},
	})

	collection.AddCommand(&cobra.Command{
		Use:   "update <name>",
		Short: "Update a collection index",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			store, err := app.loadStore()
			if err != nil {
				return err
			}
			return runUpdate(app.ctx, app.stdout, app.stderr, store, args[0])
		},
	})

	return collection
}

func newTopLevelSearchCommand(app *cliApp, vector bool) *cobra.Command {
	use := "search <query...>"
	short := "Search default collections"
	if vector {
		use = "vsearch <query...>"
		short = "Vector search default collections"
	}
	var collections []string
	var rawVector bool
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := app.loadStore()
			if err != nil {
				return err
			}
			limit, err := cmd.Flags().GetInt("limit")
			if err != nil {
				return err
			}
			all, err := cmd.Flags().GetBool("all")
			if err != nil {
				return err
			}
			minScore, err := cmd.Flags().GetFloat64("min-score")
			if err != nil {
				return err
			}
			return runTopLevelSearch(app.ctx, app.stdout, store, collections, strings.Join(args, " "), limit, minScore, all, vector, rawVector)
		},
	}
	cmd.Flags().IntP("limit", "n", 20, "maximum result count")
	cmd.Flags().Bool("all", false, "return all search results")
	cmd.Flags().Float64("min-score", 0, "minimum score")
	cmd.Flags().StringArrayVarP(&collections, "collection", "c", nil, "collection filter; repeat for multiple collections")
	if vector {
		cmd.Flags().BoolVar(&rawVector, "raw", false, "use raw vector search without query expansion")
	}
	return cmd
}

func newTopLevelQueryCommand(app *cliApp) *cobra.Command {
	var collections []string
	var queryOptions queryCLIOptions
	cmd := &cobra.Command{
		Use:   "query <question...>",
		Short: "Run hybrid query against default collections",
		Args: func(cmd *cobra.Command, args []string) error {
			queriesJSON, err := cmd.Flags().GetString("queries")
			if err != nil {
				return err
			}
			if strings.TrimSpace(queriesJSON) != "" {
				return nil
			}
			return cobra.MinimumNArgs(1)(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := app.loadStore()
			if err != nil {
				return err
			}
			limit, err := cmd.Flags().GetInt("limit")
			if err != nil {
				return err
			}
			minScore, err := cmd.Flags().GetFloat64("min-score")
			if err != nil {
				return err
			}
			queryOptions.limit = limit
			queryOptions.minScore = minScore
			queryOptions.question = strings.Join(args, " ")
			queryOptions.queriesJSON, err = cmd.Flags().GetString("queries")
			if err != nil {
				return err
			}
			queryOptions.intent, err = cmd.Flags().GetString("intent")
			if err != nil {
				return err
			}
			queryOptions.candidateLimit, err = cmd.Flags().GetInt("candidate-limit")
			if err != nil {
				return err
			}
			queryOptions.explain, err = cmd.Flags().GetBool("explain")
			if err != nil {
				return err
			}
			queryOptions.noRerank, err = cmd.Flags().GetBool("no-rerank")
			if err != nil {
				return err
			}
			return runTopLevelQuery(app.ctx, app.stdout, store, collections, queryOptions)
		},
	}
	cmd.Flags().IntP("limit", "n", 10, "maximum result count")
	cmd.Flags().Float64("min-score", 0, "minimum score")
	cmd.Flags().StringArrayVarP(&collections, "collection", "c", nil, "collection filter; repeat for multiple collections")
	cmd.Flags().String("queries", "", "pre-expanded typed queries JSON")
	cmd.Flags().String("intent", "", "domain intent hint")
	cmd.Flags().Int("candidate-limit", 0, "maximum candidates before rerank")
	cmd.Flags().Bool("explain", false, "include RRF/rerank traces")
	cmd.Flags().Bool("no-rerank", false, "skip reranker and return RRF position scores")
	return cmd
}

type queryCLIOptions struct {
	question       string
	queriesJSON    string
	intent         string
	limit          int
	minScore       float64
	candidateLimit int
	explain        bool
	noRerank       bool
}

type embedCLIOptions struct {
	collections []string
	provider    string
	model       string
	command     string
	dimensions  int
	force       bool
}

func newEmbedCommand(app *cliApp) *cobra.Command {
	opts := embedCLIOptions{}
	cmd := &cobra.Command{
		Use:   "embed",
		Short: "Generate vector embeddings for indexed documents",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runEmbed(app.ctx, app.stdout, app.stderr, app.configPath, opts)
		},
	}
	cmd.Flags().BoolVarP(&opts.force, "force", "f", false, "rebuild existing vectors")
	cmd.Flags().StringArrayVarP(&opts.collections, "collection", "c", nil, "collection filter; repeat for multiple collections")
	cmd.Flags().StringVar(&opts.provider, "provider", "", "embedding provider override")
	cmd.Flags().StringVar(&opts.model, "model", "", "embedding model override")
	cmd.Flags().StringVar(&opts.command, "command", "", "embedding command override")
	cmd.Flags().IntVar(&opts.dimensions, "dimensions", 0, "embedding dimensions override")
	return cmd
}

func newModelCommand(app *cliApp) *cobra.Command {
	model := &cobra.Command{
		Use:   "model",
		Short: "Manage local GGUF models",
	}
	downloadCmd := &cobra.Command{
		Use:   "download [embed|rerank|generate|all]",
		Short: "Download configured models into .wikimesh/models",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			target := "all"
			if len(args) > 0 {
				target = args[0]
			}
			return runModelDownload(app.ctx, app.stdout, app.stderr, app.configPath, target)
		},
	}
	model.AddCommand(downloadCmd)

	lib := &cobra.Command{
		Use:   "lib",
		Short: "Manage llama.cpp runtime libraries",
	}
	var libPath, processor, version, osName string
	var upgrade bool
	install := &cobra.Command{
		Use:   "install",
		Short: "Install yzma llama.cpp libraries into .wikimesh/lib",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runModelLibInstall(app.ctx, app.stdout, app.stderr, modelLibInstallOptions{
				libPath:   libPath,
				processor: processor,
				version:   version,
				osName:    osName,
				upgrade:   upgrade,
			})
		},
	}
	install.Flags().StringVar(&libPath, "lib", qmd.DefaultLlamaLibDir(), "llama.cpp library directory")
	install.Flags().StringVarP(&processor, "processor", "p", "auto", "processor: auto, cpu, metal, cuda, vulkan, rocm")
	install.Flags().StringVar(&version, "version", "latest", "llama.cpp release version")
	install.Flags().StringVar(&osName, "os", runtime.GOOS, "runtime OS: linux, darwin, windows, bookworm, trixie")
	install.Flags().BoolVarP(&upgrade, "upgrade", "u", false, "download even when libraries already exist")
	lib.AddCommand(install)
	model.AddCommand(lib)
	return model
}

func runTopLevelSearch(ctx context.Context, out io.Writer, store *qmd.Store, collections []string, query string, limit int, minScore float64, all bool, vector bool, rawVector bool) error {
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
		if minScore == 0 && !rawVector {
			minScore = 0.3
		}
		if rawVector {
			results, err = store.SearchVector(ctx, query, qmd.VectorSearchOptions{Collection: searchCollection, Limit: searchLimit, MinScore: minScore})
		} else {
			results, err = store.VSearch(ctx, searchCollection, query, qmd.SearchOptions{Limit: searchLimit, MinScore: minScore})
		}
	} else {
		results, err = store.Search(ctx, searchCollection, query, qmd.SearchOptions{Limit: searchLimit, MinScore: minScore})
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

func resolveSearchCollections(ctx context.Context, store *qmd.Store, collections []string) ([]string, error) {
	if len(collections) > 0 {
		return uniqueNonEmpty(collections), nil
	}
	return store.DefaultCollectionNames(ctx)
}

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

func uniqueNonEmpty(items []string) []string {
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

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func loadCLIConfig(path string) (qmd.FileConfig, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			cfg := qmd.DefaultFileConfig()
			if err := qmd.SaveConfigFile(path, cfg); err != nil {
				return qmd.FileConfig{}, err
			}
			return cfg, nil
		}
		return qmd.FileConfig{}, err
	}
	return qmd.LoadConfigFile(path)
}

func openStoreFromConfig(ctx context.Context, cfg qmd.FileConfig) (*qmd.Store, error) {
	store, err := qmd.NewStore(cfg.StoreConfig())
	if err != nil {
		return nil, err
	}
	for _, c := range cfg.Collections {
		if err := store.AddCollection(ctx, c); err != nil {
			store.Close()
			return nil, err
		}
	}
	if strings.TrimSpace(cfg.GlobalContext) != "" {
		if err := store.SetGlobalContext(ctx, cfg.GlobalContext); err != nil {
			store.Close()
			return nil, err
		}
	}
	return store, nil
}

type collectionAddOptions struct {
	name    string
	path    string
	mask    string
	include string
}

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
	if err := store.AddCollection(ctx, collection); err != nil {
		return err
	}
	return syncConfigFile(ctx, cfg, configPath, store)
}

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

func runRemove(ctx context.Context, out io.Writer, cfg qmd.FileConfig, configPath string, store *qmd.Store, name string) error {
	removed, err := store.RemoveCollection(ctx, name)
	if err != nil {
		return err
	}
	if removed {
		if err := syncConfigFile(ctx, cfg, configPath, store); err != nil {
			return err
		}
	}
	return printJSON(out, map[string]any{"removed": removed})
}

func syncConfigFile(ctx context.Context, cfg qmd.FileConfig, configPath string, store *qmd.Store) error {
	collections, err := store.ListCollections(ctx)
	if err != nil {
		return err
	}
	cfg.Collections = collections
	global, err := store.GlobalContext(ctx)
	if err != nil {
		return err
	}
	cfg.GlobalContext = global
	return qmd.SaveConfigFile(configPath, cfg)
}

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
	fmt.Fprintf(out, "Collection: %s\n", name)
	fmt.Fprintf(out, "Indexed: %d new/updated, %d unchanged, %d removed\n", result.Indexed, result.Skipped, result.Removed)
	if result.Indexed > 0 || result.Removed > 0 {
		fmt.Fprintf(out, "\nRun 'wikimesh embed' to update embeddings\n")
	}
	return nil
}

func runEmbed(ctx context.Context, out, errOut io.Writer, configPath string, opts embedCLIOptions) error {
	cfg, err := loadCLIConfig(configPath)
	if err != nil {
		return err
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

	store, err := openStoreFromConfig(ctx, cfg)
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
		fmt.Fprintln(out, "No collections found. Run 'wikimesh collection add .' to index markdown files.")
		return nil
	}

	model := store.EmbeddingModelName()
	if model == "" {
		return fmt.Errorf("embedding provider is not configured")
	}
	fmt.Fprintf(out, "Model: %s\n\n", model)

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
		fmt.Fprintf(out, "Embedded: %d chunks, %d documents checked, %d unchanged\n\n", result.Embedded, result.Scanned, result.Skipped)
	}
	if len(targets) > 1 {
		fmt.Fprintf(out, "Embedded: %d chunks total, %d documents checked, %d unchanged\n", total.Embedded, total.Scanned, total.Skipped)
	}
	fmt.Fprintf(out, "All embeddings updated.\n")
	return nil
}

func runModelDownload(ctx context.Context, out, errOut io.Writer, configPath string, target string) error {
	cfg, err := loadCLIConfig(configPath)
	if err != nil {
		return err
	}
	roles, err := modelDownloadRoles(target)
	if err != nil {
		return err
	}
	for _, role := range roles {
		result, err := downloadConfiguredModel(ctx, cfg, role, out, errOut)
		if err != nil {
			return err
		}
		if result.Downloaded {
			fmt.Fprintf(out, "Downloaded: %s -> %s\n", result.Source, result.Destination)
		} else {
			fmt.Fprintf(out, "Exists: %s\n", result.Destination)
		}
	}
	return nil
}

func modelDownloadRoles(target string) ([]qmd.ModelRole, error) {
	switch strings.TrimSpace(target) {
	case "", "all":
		return []qmd.ModelRole{qmd.ModelRoleEmbed, qmd.ModelRoleRerank, qmd.ModelRoleGenerate}, nil
	case "embed":
		return []qmd.ModelRole{qmd.ModelRoleEmbed}, nil
	case "rerank":
		return []qmd.ModelRole{qmd.ModelRoleRerank}, nil
	case "generate", "query":
		return []qmd.ModelRole{qmd.ModelRoleGenerate}, nil
	default:
		return nil, fmt.Errorf("unknown model role: %s", target)
	}
}

func ensureModelForRole(ctx context.Context, cfg qmd.FileConfig, role qmd.ModelRole, out, errOut io.Writer) error {
	_, err := downloadConfiguredModel(ctx, cfg, role, out, errOut)
	return err
}

func downloadConfiguredModel(ctx context.Context, cfg qmd.FileConfig, role qmd.ModelRole, out, errOut io.Writer) (qmd.ModelDownload, error) {
	source, destination := modelSourceAndDestination(cfg, role)
	if strings.TrimSpace(destination) == "" {
		destination = qmd.LocalModelPath(source)
	}
	if _, err := os.Stat(destination); err == nil {
		return qmd.DownloadModel(ctx, role, source, destination)
	} else if err != nil && !os.IsNotExist(err) {
		return qmd.ModelDownload{Role: role, Source: source, Destination: destination}, err
	}
	fmt.Fprintf(out, "Downloading: %s -> %s\n", source, destination)
	progress := newModelDownloadProgress(errOut, role)
	result, err := qmd.DownloadModelWithOptions(ctx, role, source, destination, qmd.ModelDownloadOptions{Progress: progress.report})
	progress.finish()
	return result, err
}

type modelDownloadProgress struct {
	errOut io.Writer
	role   qmd.ModelRole
	bar    *progressbar.ProgressBar
}

func newModelDownloadProgress(errOut io.Writer, role qmd.ModelRole) *modelDownloadProgress {
	return &modelDownloadProgress{errOut: errOut, role: role}
}

func (p *modelDownloadProgress) report(info qmd.ModelDownloadProgress) {
	if !writerIsTerminal(p.errOut) || info.Total <= 0 {
		return
	}
	if p.bar == nil {
		p.bar = progressbar.NewOptions64(info.Total,
			progressbar.OptionSetWriter(p.errOut),
			progressbar.OptionSetDescription(fmt.Sprintf("Downloading %s", p.role)),
			progressbar.OptionSetWidth(28),
			progressbar.OptionShowBytes(true),
			progressbar.OptionShowCount(),
			progressbar.OptionClearOnFinish(),
			progressbar.OptionThrottle(100*time.Millisecond),
		)
	}
	_ = p.bar.Set64(info.Current)
}

func (p *modelDownloadProgress) finish() {
	if p.bar != nil {
		_ = p.bar.Finish()
	}
}

func modelSourceAndDestination(cfg qmd.FileConfig, role qmd.ModelRole) (string, string) {
	fallback := func(source, destination string) (string, string) {
		if source == "" {
			source = destination
		}
		if destination == "" {
			destination = qmd.LocalModelPath(source)
		}
		return source, destination
	}
	switch role {
	case qmd.ModelRoleEmbed:
		return fallback(cfg.Models.Embed, cfg.Embedding.Model)
	case qmd.ModelRoleRerank:
		return fallback(cfg.Models.Rerank, cfg.Reranker.Model)
	case qmd.ModelRoleGenerate:
		return fallback(cfg.Models.Generate, cfg.QueryExpansion.Model)
	default:
		return "", ""
	}
}

type modelLibInstallOptions struct {
	// libPath 是 llama.cpp 动态库安装目录。
	libPath string
	// processor 是 yzma 下载的硬件后端，auto 表示按平台选择。
	processor string
	// version 是 llama.cpp release 版本，latest 表示使用最新可用版本。
	version string
	// osName 是 yzma 下载目标操作系统。
	osName string
	// upgrade 表示即使已安装也重新下载。
	upgrade bool
}

func runModelLibInstall(ctx context.Context, out, errOut io.Writer, opts modelLibInstallOptions) error {
	libPath := strings.TrimSpace(opts.libPath)
	if libPath == "" {
		libPath = qmd.DefaultLlamaLibDir()
	}
	processor := qmd.ResolveLlamaLibProcessor(opts.processor)
	version := strings.TrimSpace(opts.version)
	if version == "" {
		version = "latest"
	}
	osName := strings.TrimSpace(opts.osName)
	if osName == "" {
		osName = runtime.GOOS
	}
	if !opts.upgrade && qmd.LlamaLibAlreadyInstalled(libPath) {
		fmt.Fprintf(out, "Exists: %s\n", libPath)
		return nil
	}
	fmt.Fprintf(out, "Installing llama.cpp: processor=%s version=%s -> %s\n", processor, version, libPath)
	progress := newLibraryDownloadProgress(errOut)
	err := qmd.InstallLlamaLib(ctx, qmd.LlamaLibInstallOptions{
		LibPath:   libPath,
		Processor: processor,
		Version:   version,
		OS:        osName,
		Progress:  progress,
	})
	progress.finish()
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Installed: %s\n", libPath)
	return nil
}

type libraryDownloadProgress struct {
	errOut io.Writer
	bar    *progressbar.ProgressBar
}

func newLibraryDownloadProgress(errOut io.Writer) *libraryDownloadProgress {
	return &libraryDownloadProgress{errOut: errOut}
}

func (p *libraryDownloadProgress) TrackProgress(_ string, currentSize, totalSize int64, stream io.ReadCloser) io.ReadCloser {
	if stream == nil {
		return nil
	}
	return &libraryProgressReader{
		reader:      stream,
		progress:    p,
		currentSize: currentSize,
		totalSize:   totalSize,
	}
}

func (p *libraryDownloadProgress) report(currentSize, totalSize int64) {
	if !writerIsTerminal(p.errOut) || totalSize <= 0 {
		return
	}
	if p.bar == nil {
		p.bar = progressbar.NewOptions64(totalSize,
			progressbar.OptionSetWriter(p.errOut),
			progressbar.OptionSetDescription("Downloading llama.cpp"),
			progressbar.OptionSetWidth(28),
			progressbar.OptionShowBytes(true),
			progressbar.OptionShowCount(),
			progressbar.OptionClearOnFinish(),
			progressbar.OptionThrottle(100*time.Millisecond),
		)
	}
	_ = p.bar.Set64(currentSize)
}

func (p *libraryDownloadProgress) finish() {
	if p.bar != nil {
		_ = p.bar.Finish()
	}
}

type libraryProgressReader struct {
	// reader 是 go-getter 提供的下载流。
	reader io.ReadCloser
	// progress 是共享进度条状态。
	progress *libraryDownloadProgress
	// currentSize 是已经读取的字节数。
	currentSize int64
	// totalSize 是下载响应声明的总字节数。
	totalSize int64
}

func (r *libraryProgressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		r.currentSize += int64(n)
		r.progress.report(r.currentSize, r.totalSize)
	}
	return n, err
}

func (r *libraryProgressReader) Close() error {
	r.progress.report(r.currentSize, r.totalSize)
	return r.reader.Close()
}

var _ qmd.LlamaLibProgressTracker = (*libraryDownloadProgress)(nil)

func writerIsTerminal(w io.Writer) bool {
	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(file.Fd()))
}

func printJSON(out io.Writer, v any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
