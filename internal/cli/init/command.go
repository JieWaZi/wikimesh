package initcmd

import (
	"bufio"
	"context"
	"embed"
	"errors"
	"fmt"
	"github.com/JieWaZi/wikimesh/pkg/qmd"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/JieWaZi/wikimesh/internal/cli/common"
	"github.com/JieWaZi/wikimesh/internal/ui"
)

//go:embed template/docs/*
var initTemplateFS embed.FS

// InitOptions 描述 `wikimesh init` 的初始化参数。
type InitOptions struct {
	// ProjectName 是当前 Wikimesh 工作区的展示名称。
	ProjectName string
	// WikiType 是要初始化和安装的 Wiki 类型。
	WikiType string
	// Agent 是要生成运行时入口的目标 Agent。
	Agent string
	// CodeDirs 是与当前知识库关联的代码仓目录列表。
	CodeDirs []string
	// Global 表示把 runtime skills 安装到用户主目录。
	Global bool
	// ScopeProvided 表示用户已经通过参数指定安装范围。
	ScopeProvided bool
	// WikiTypeProvided 表示用户已经通过参数指定 Wiki 类型。
	WikiTypeProvided bool
	// Yes 表示跳过交互确认并采用默认选择。
	Yes bool
}

// NewCommand 构造 `wikimesh init` 命令。
func NewCommand() *cobra.Command {
	msg := ui.Messages()
	var opts InitOptions
	cmd := &cobra.Command{
		Use:   "init [project-name]",
		Short: msg.WikiInitShort,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.ProjectName = args[0]
			}
			opts.ScopeProvided = cmd.Flags().Changed("global")
			opts.WikiTypeProvided = cmd.Flags().Changed("type")
			if !opts.Yes {
				if err := collectInitOptions(cmd.InOrStdin(), cmd.OutOrStdout(), readerIsTerminal(cmd.InOrStdin()), &opts, cmd.Flags().Changed("agent"), cmd.Flags().Changed("code-dir")); err != nil {
					return err
				}
			}
			return runWikiInit(cmd.Context(), cmd.OutOrStdout(), readerIsTerminal(cmd.InOrStdin()), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Agent, "agent", "codex", msg.FlagAgent)
	cmd.Flags().StringVar(&opts.WikiType, "type", common.DefaultWikiSkillType(), msg.FlagWikiType)
	cmd.Flags().StringSliceVar(&opts.CodeDirs, "code-dir", nil, msg.FlagCodeDir)
	cmd.Flags().BoolVarP(&opts.Global, "global", "g", false, msg.FlagGlobal)
	cmd.Flags().BoolVarP(&opts.Yes, "yes", "y", false, msg.FlagYes)
	return cmd
}

func readerIsTerminal(r any) bool {
	file, ok := r.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(file.Fd()))
}

// runWikiInit 创建工作区并安装 runtime skills。
func runWikiInit(ctx context.Context, out io.Writer, interactive bool, opts InitOptions) error {
	msg := ui.Messages()
	resolved, err := normalizeWikiInitOptions(opts)
	if err != nil {
		return err
	}
	targetDir, err := os.Getwd()
	if err != nil {
		return err
	}
	targetDir, err = filepath.Abs(targetDir)
	if err != nil {
		return err
	}

	ui.Note(msg.TitleWikiSummary, []string{
		fmt.Sprintf("%s: %s", msg.ProjectLabel, resolved.ProjectName),
		fmt.Sprintf("%s: %s", msg.WikiTypeLabel, resolved.WikiType),
		fmt.Sprintf("%s: %s", msg.SourceLabel, targetDir),
		fmt.Sprintf("%s: %s", msg.AgentLabel, resolved.Agent),
		fmt.Sprintf("%s: %s", msg.WikiCodeDirsLabel, strings.Join(resolved.CodeDirs, ", ")),
		fmt.Sprintf("%s: %s", msg.ScopeLabel, ui.ScopeText(resolved.Global)),
	})

	spinner := ui.NewStepPrinter()
	spinner.Start(msg.StepCreatingWikiProject)
	if err := createWikiWorkspace(ctx, targetDir, resolved); err != nil {
		return err
	}
	if err := saveWikiInitRepoConfig(targetDir, resolved); err != nil {
		return err
	}
	spinner.Stop(fmt.Sprintf(msg.CreatedFmt, targetDir))

	bundle, err := common.ResolveWikimeshSkills(resolved.WikiType, "")
	if err != nil {
		return err
	}
	if bundle.Cleanup != nil {
		defer func() { _ = bundle.Cleanup() }()
	}
	if len(bundle.Skills) > 0 {
		spinner.Start(msg.StepInstallingWikiSkills)
		if err := common.InstallWikiSkills(resolved.Agent, resolved.Global, bundle.Skills); err != nil {
			return err
		}
		spinner.Stop(fmt.Sprintf(msg.WikiInstalledSkillsFmt, resolved.WikiType, len(bundle.Skills)))
	}

	if err := ensureWikiGitignore(targetDir, resolved); err != nil {
		return err
	}

	fmt.Printf("%s%s%s\n", ui.Green, msg.Done, ui.Reset)
	ui.Note(msg.TitleQMDManualDownload, []string{
		msg.QMDManualDownloadHint,
		msg.QMDManualDownloadCommand,
	})
	_, _ = out.Write(nil)
	return nil
}

// ensureWikiGitignore 按项目级安装结果补充 .gitignore，避免 runtime 目录进入仓库。
func ensureWikiGitignore(projectRoot string, opts InitOptions) error {
	if opts.Global {
		return nil
	}
	installDir, err := common.WikiSkillTargetRoot(opts.Agent, false)
	if err != nil {
		return err
	}
	return ensureProjectGitignore(projectRoot, filepath.Join(projectRoot, ".wikimesh"), installDir)
}

// ensureProjectGitignore 确保项目级 runtime 目录写入 .gitignore。
func ensureProjectGitignore(projectRoot string, paths ...string) error {
	entries := make([]string, 0, len(paths))
	seen := map[string]struct{}{}
	for _, path := range paths {
		entry, err := gitignoreEntryForProjectPath(projectRoot, path)
		if err != nil {
			return err
		}
		if _, ok := seen[entry]; ok {
			continue
		}
		seen[entry] = struct{}{}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return nil
	}

	gitignorePath := filepath.Join(projectRoot, ".gitignore")
	data, err := os.ReadFile(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	existing := map[string]struct{}{}
	for _, line := range strings.Split(string(data), "\n") {
		normalized := normalizeGitignoreEntry(line)
		if normalized != "" {
			existing[normalized] = struct{}{}
		}
	}

	var missing []string
	for _, entry := range entries {
		if _, ok := existing[normalizeGitignoreEntry(entry)]; !ok {
			missing = append(missing, entry)
		}
	}
	if len(missing) == 0 {
		return nil
	}

	content := string(data)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += strings.Join(missing, "\n") + "\n"
	return os.WriteFile(gitignorePath, []byte(content), 0o644)
}

// gitignoreEntryForProjectPath 将项目内路径转成稳定的 .gitignore 条目。
func gitignoreEntryForProjectPath(projectRoot string, path string) (string, error) {
	if strings.TrimSpace(projectRoot) == "" {
		return "", fmt.Errorf("project root is required")
	}
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path is required")
	}

	candidate := path
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(projectRoot, candidate)
	}
	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", err
	}
	absPath, err := filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return "", err
	}
	if rel == "." {
		return "", fmt.Errorf("refuse to ignore the project root directly")
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q is outside project root %q", path, projectRoot)
	}

	entry := filepath.ToSlash(filepath.Clean(rel))
	if strings.HasPrefix(entry, "./") {
		entry = strings.TrimPrefix(entry, "./")
	}
	parts := strings.Split(entry, "/")
	if len(parts) > 1 && strings.HasPrefix(parts[0], ".") {
		return parts[0], nil
	}
	return entry, nil
}

// normalizeGitignoreEntry 规范化 .gitignore 单行内容用于去重。
func normalizeGitignoreEntry(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return ""
	}
	return strings.TrimSuffix(trimmed, "/")
}

// resolveSelectedWikiSkills 按交互状态决定要安装哪些 Wikimesh skills。
func resolveSelectedWikiSkills(found []common.WikiSkill, opts InitOptions, interactive bool) ([]common.WikiSkill, error) {
	if len(found) == 0 {
		return nil, nil
	}
	if !interactive || opts.Yes {
		return append([]common.WikiSkill(nil), found...), nil
	}

	items := make([]ui.Option, 0, len(found))
	initial := make([]string, 0, len(found))
	for _, skill := range found {
		items = append(items, ui.Option{Value: skill.Name, Label: skill.Name, Hint: skill.Description})
		initial = append(initial, skill.Name)
	}
	selectedNames, cancelled, err := ui.SearchMultiselect(ui.SearchMultiselectOptions{
		Message:         ui.Messages().PromptSelectWikiSkills,
		Items:           items,
		MaxVisible:      8,
		InitialSelected: initial,
		Required:        false,
	})
	if err != nil {
		return nil, err
	}
	if cancelled {
		return nil, nil
	}
	selectedSet := make(map[string]struct{}, len(selectedNames))
	for _, name := range selectedNames {
		selectedSet[name] = struct{}{}
	}
	selected := make([]common.WikiSkill, 0, len(selectedNames))
	for _, skill := range found {
		if _, ok := selectedSet[skill.Name]; ok {
			selected = append(selected, skill)
		}
	}
	return selected, nil
}

// saveWikiInitRepoConfig 将 init 创建的本地工作区登记到用户级项目配置。
func saveWikiInitRepoConfig(targetDir string, opts InitOptions) error {
	slug := common.WikiSlug(opts.ProjectName)
	cfg := common.WikiRepoConfig{
		ProjectName:  opts.ProjectName,
		ProjectSlug:  slug,
		Language:     "zh",
		ActiveSource: common.WikiRepoSourceLocal,
	}
	if existing, err := common.LoadWikiRepoConfig(slug); err == nil {
		cfg = existing
		cfg.ProjectName = opts.ProjectName
		cfg.ProjectSlug = slug
		if strings.TrimSpace(cfg.Language) == "" {
			cfg.Language = "zh"
		}
	}
	cfg.ActiveSource = common.WikiRepoSourceLocal
	cfg.Sources.Local = &common.WikiRepoSource{Type: common.WikiRepoSourceLocal, Path: targetDir}
	cfg.CodeRepos = nil
	seenRepos := map[string]int{}
	for _, dir := range opts.CodeDirs {
		name := filepath.Base(dir)
		baseSlug := common.WikiSlug(name)
		seenRepos[baseSlug]++
		repoSlug := baseSlug
		if seenRepos[baseSlug] > 1 {
			repoSlug = fmt.Sprintf("%s-%d", baseSlug, seenRepos[baseSlug])
		}
		cfg.CodeRepos = append(cfg.CodeRepos, common.WikiCodeRepo{
			Name:    name,
			Slug:    repoSlug,
			Path:    dir,
			Default: len(cfg.CodeRepos) == 0,
		})
	}
	return common.SaveWikiRepoConfig(cfg)
}

// normalizeWikiInitOptions 校验并补齐 init 参数，保证后续流程只处理规范值。
func normalizeWikiInitOptions(opts InitOptions) (InitOptions, error) {
	opts.ProjectName = strings.TrimSpace(opts.ProjectName)
	if opts.ProjectName == "" {
		return opts, fmt.Errorf("project name is required")
	}
	opts.WikiType = strings.TrimSpace(opts.WikiType)
	if opts.WikiType == "" {
		opts.WikiType = common.DefaultWikiSkillType()
	}
	if _, err := common.LookupWikiSkillType(opts.WikiType); err != nil {
		return opts, err
	}
	opts.Agent = strings.TrimSpace(opts.Agent)
	if opts.Agent == "" {
		opts.Agent = "codex"
	}
	switch opts.Agent {
	case "codex", "cursor", "claude":
	default:
		return opts, fmt.Errorf("unsupported agent %q", opts.Agent)
	}
	if len(opts.CodeDirs) == 0 {
		opts.CodeDirs = []string{"."}
	}
	codeDirs := make([]string, 0, len(opts.CodeDirs))
	for _, raw := range opts.CodeDirs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		abs, err := filepath.Abs(raw)
		if err != nil {
			return opts, err
		}
		if info, err := os.Stat(abs); err != nil {
			return opts, err
		} else if !info.IsDir() {
			return opts, fmt.Errorf("code-dir %q is not a directory", abs)
		}
		codeDirs = append(codeDirs, abs)
	}
	if len(codeDirs) == 0 {
		return opts, fmt.Errorf("code-dir is required")
	}
	opts.CodeDirs = codeDirs
	return opts, nil
}

// createWikiWorkspace 创建 Wikimesh 工作区骨架和 qmd 配置。
func createWikiWorkspace(ctx context.Context, targetDir string, opts InitOptions) error {
	projectName := opts.ProjectName

	// 创建 Wikimesh 固定目录，保持 raw/wiki/config 三层边界清晰。
	for _, dir := range []string{
		"raw/requirements",
		"raw/designs",
		"raw/features",
		"raw/tests",
		"wiki/topics",
		"wiki/workflows",
		"wiki/troubleshooting",
		"wiki/outputs",
		"config",
	} {
		if err := os.MkdirAll(filepath.Join(targetDir, dir), 0o755); err != nil {
			return err
		}
	}
	// 写入基础导航文件和项目配置；已有文件不覆盖，避免破坏用户内容。
	slug := common.WikiSlug(projectName)
	if err := writeFileIfMissing(filepath.Join(targetDir, "wiki/index.md"), "# Wiki Index\n\n| type | description | slug |\n|---|---|---|\n"); err != nil {
		return err
	}
	if err := writeFileIfMissing(filepath.Join(targetDir, "wiki/glossary.md"), "# Glossary\n\n| glossary | type | description | slug |\n|---|---|---|---|\n"); err != nil {
		return err
	}
	if err := writeFileIfMissing(filepath.Join(targetDir, "wiki/log.md"), "# Wiki Log\n\n> Append-only chronological log.\n"); err != nil {
		return err
	}
	projectConfig := fmt.Sprintf("project_name: %s\nproject_slug: %s\nwiki_type: %s\nagent: %s\nlanguage: zh\n", projectName, slug, opts.WikiType, opts.Agent)
	if err := writeFileIfMissing(filepath.Join(targetDir, "config/project.yaml"), projectConfig); err != nil {
		return err
	}
	runtimeFile := "AGENTS.md"
	if opts.Agent == "claude" {
		runtimeFile = "CLAUDE.md"
	}
	templateValues := map[string]string{
		"PROJECT_NAME":     projectName,
		"PROJECT_SLUG":     slug,
		"AGENT":            opts.Agent,
		"RUNTIME_FILE":     runtimeFile,
		"PRIMARY_CODE_DIR": primaryCodeDir(opts.CodeDirs),
	}
	readme, err := renderInitTemplate("README.md", templateValues)
	if err != nil {
		return err
	}
	if err := writeFileIfMissing(filepath.Join(targetDir, "README.md"), readme); err != nil {
		return err
	}
	runtimeContent, err := renderInitTemplate(runtimeFile, templateValues)
	if err != nil {
		return err
	}
	if err := writeFileIfMissing(filepath.Join(targetDir, runtimeFile), runtimeContent); err != nil {
		return err
	}

	return initializeWikiQMDCollection(ctx, targetDir, slug)
}

func renderInitTemplate(name string, values map[string]string) (string, error) {
	data, err := initTemplateFS.ReadFile(filepath.ToSlash(filepath.Join("template/docs", name)))
	if err != nil {
		return "", err
	}
	content := string(data)
	for key, value := range values {
		content = strings.ReplaceAll(content, "{{"+key+"}}", value)
	}
	return content, nil
}

func primaryCodeDir(codeDirs []string) string {
	if len(codeDirs) == 0 {
		return "."
	}
	return codeDirs[0]
}

// initializeWikiQMDCollection 初始化 qmd 配置，并通过 collection 添加链路登记 wiki 目录。
func initializeWikiQMDCollection(ctx context.Context, targetDir string, projectSlug string) error {
	configPath := common.QMDConfigPathForRoot(targetDir)
	cfg := qmd.DefaultFileConfig()
	if err := qmd.SaveConfigFile(configPath, cfg); err != nil {
		return err
	}
	store, err := common.OpenQMDStoreFromConfig(ctx, cfg)
	if err != nil {
		return err
	}
	defer store.Close()

	// collection 名称与项目 slug 保持一致，让 qmd 查询和用户级项目配置可以直接对应。
	collection := qmd.Collection{Name: projectSlug, Path: "wiki", Pattern: "**/*.md"}
	return common.AddQMDCollectionAndSync(ctx, cfg, configPath, store, collection)
}

// writeFileIfMissing 只在文件不存在时写入模板，避免覆盖用户已有内容。
func writeFileIfMissing(path, content string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && filepath.Dir(path) != "." {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// collectInitOptions 在缺少 init 参数时按 zatools 风格收集交互回答。
func collectInitOptions(in io.Reader, out io.Writer, interactive bool, opts *InitOptions, agentProvided, codeDirsProvided bool) error {
	msg := ui.Messages()
	reader := bufio.NewReader(in)
	if strings.TrimSpace(opts.ProjectName) == "" {
		value, err := promptWikiLine(reader, out, msg.PromptWikiProjectName, "")
		if err != nil {
			return err
		}
		opts.ProjectName = value
	}
	if !opts.WikiTypeProvided {
		if interactive {
			items := make([]ui.Option, 0, len(common.BuiltinWikiSkillTypes()))
			for _, typ := range common.BuiltinWikiSkillTypes() {
				items = append(items, ui.Option{Value: typ.Name, Label: typ.Name, Hint: typ.Description})
			}
			value, cancelled, err := ui.SelectOne(ui.SelectOneOptions{
				Message: msg.PromptWikiType,
				Items:   items,
			})
			if err != nil {
				return err
			}
			if cancelled {
				return errors.New(msg.Cancelled)
			}
			opts.WikiType = value
		} else {
			value, err := promptWikiLine(reader, out, msg.PromptWikiType, common.DefaultWikiSkillType())
			if err != nil {
				return err
			}
			opts.WikiType = value
		}
	}
	if !agentProvided {
		if interactive {
			value, cancelled, err := ui.SelectOne(ui.SelectOneOptions{
				Message: msg.PromptWikiAgent,
				Items: []ui.Option{
					{Value: "codex", Label: "Codex"},
					{Value: "cursor", Label: "Cursor"},
					{Value: "claude", Label: "Claude Code"},
				},
			})
			if err != nil {
				return err
			}
			if cancelled {
				return errors.New(msg.Cancelled)
			}
			opts.Agent = value
		} else {
			value, err := promptWikiLine(reader, out, msg.PromptWikiAgent, "codex")
			if err != nil {
				return err
			}
			opts.Agent = value
		}
	}
	if !codeDirsProvided && len(opts.CodeDirs) == 0 {
		defaultDir, err := os.Getwd()
		if err != nil {
			defaultDir = "."
		}
		value, err := promptWikiLine(reader, out, msg.PromptWikiCodeDirs, defaultDir)
		if err != nil {
			return err
		}
		opts.CodeDirs = splitCommaValues(value)
	}
	if !opts.ScopeProvided {
		if interactive {
			value, cancelled, err := ui.SelectOne(ui.SelectOneOptions{
				Message: msg.PromptWikiScope,
				Items: []ui.Option{
					{Value: "project", Label: msg.InstallInProject},
					{Value: "global", Label: msg.InstallInHome},
				},
			})
			if err != nil {
				return err
			}
			if cancelled {
				return errors.New(msg.Cancelled)
			}
			opts.Global = value == "global"
		} else {
			value, err := promptWikiLine(reader, out, msg.PromptWikiScope, "project")
			if err != nil {
				return err
			}
			opts.Global = strings.EqualFold(strings.TrimSpace(value), "global")
		}
	}
	return nil
}

// promptWikiLine 输出提示并读取一行输入；空输入使用默认值。
func promptWikiLine(in io.Reader, out io.Writer, prompt string, defaultValue string) (string, error) {
	label := prompt
	if defaultValue != "" {
		label = fmt.Sprintf("%s [%s]", prompt, defaultValue)
	}
	if _, err := fmt.Fprintf(out, "%s: ", label); err != nil {
		return "", err
	}
	value, err := readPromptLine(in)
	if err != nil {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultValue, nil
	}
	return value, nil
}

// readPromptLine 从输入流读取一行，优先复用外层传入的缓冲 reader。
func readPromptLine(in io.Reader) (string, error) {
	if reader, ok := in.(*bufio.Reader); ok {
		return reader.ReadString('\n')
	}
	return bufio.NewReader(in).ReadString('\n')
}

// splitCommaValues 将逗号分隔输入拆成非空列表。
func splitCommaValues(input string) []string {
	parts := strings.Split(input, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}
