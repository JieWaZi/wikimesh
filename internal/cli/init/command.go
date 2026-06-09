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

var (
	wikiSelectOne         = ui.SelectOne
	wikiSearchMultiselect = ui.SearchMultiselect
)

const (
	// InitModeCreate 表示在当前目录新建 Wikimesh 文档库。
	InitModeCreate = "create"
	// InitModeLink 表示登记并关联已有 Wikimesh 文档库。
	InitModeLink = "link"
)

// InitOptions 描述 `wikimesh init` 的初始化参数。
type InitOptions struct {
	// Mode 是初始化模式：新建文档库或关联已有文档库。
	Mode string
	// ProjectName 是当前 Wikimesh 工作区的展示名称。
	ProjectName string
	// WikiType 是要安装的 skill 类型。
	WikiType string
	// Agent 是要生成运行时入口的目标 Agent。
	Agent string
	// Agents 是归一化后的目标 Agent 列表。
	Agents []string
	// CodeDirs 是与当前知识库关联的代码仓目录列表。
	CodeDirs []string
	// CodeRepos 是交互关联文档库时显式输入的代码仓标识和路径。
	CodeRepos []InitCodeRepo
	// SourceType 是关联文档库时使用的来源类型。
	SourceType string
	// LocalPath 是关联本地文档库时的路径。
	LocalPath string
	// RemoteURL 是关联远端文档库时的 URL。
	RemoteURL string
	// Global 表示把 runtime skills 安装到用户主目录。
	Global bool
	// ScopeProvided 表示用户已经通过参数指定安装范围。
	ScopeProvided bool
	// WikiTypeProvided 表示用户已经通过参数指定 skill 类型。
	WikiTypeProvided bool
	// Yes 表示跳过交互确认并采用默认选择。
	Yes bool
}

// InitCodeRepo 描述 init 关联文档库时收集的代码仓条目。
type InitCodeRepo struct {
	// Slug 是用户输入的代码仓标识。
	Slug string
	// Path 是代码仓本地路径。
	Path string
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
			opts.Mode = strings.TrimSpace(opts.Mode)
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
	cmd.Flags().StringVar(&opts.WikiType, "type", "", msg.FlagWikiType)
	cmd.Flags().StringSliceVar(&opts.CodeDirs, "code-dir", nil, msg.FlagCodeDir)
	cmd.Flags().StringVar(&opts.Mode, "mode", "", msg.FlagWikiInitMode)
	cmd.Flags().StringVar(&opts.SourceType, "source", "", msg.FlagWikiRepoSource)
	cmd.Flags().StringVar(&opts.LocalPath, "path", "", msg.FlagPath)
	cmd.Flags().StringVar(&opts.RemoteURL, "remote", "", msg.FlagRemote)
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
	if resolved.Mode == InitModeLink {
		return runWikiInitLink(out, interactive, resolved)
	}
	selectedSkills, cleanupSkills, err := selectedWikiInitSkills(&resolved, interactive)
	if err != nil {
		return err
	}
	if cleanupSkills != nil {
		defer func() { _ = cleanupSkills() }()
	}
	targetDir, err := wikiInitCreateTargetDir(resolved.ProjectName)
	if err != nil {
		return err
	}

	ui.Note(msg.TitleWikiSummary, []string{
		fmt.Sprintf("%s: %s", msg.ProjectLabel, resolved.ProjectName),
		fmt.Sprintf("%s: %s", msg.WikiTypeLabel, resolved.WikiType),
		fmt.Sprintf("%s: %s", msg.SourceLabel, targetDir),
		fmt.Sprintf("%s: %s", msg.AgentLabel, strings.Join(resolved.Agents, ", ")),
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

	if len(selectedSkills) > 0 {
		spinner.Start(msg.StepInstallingWikiSkills)
		if err := installWikiSkillsForAgentsInProject(targetDir, resolved.Agents, resolved.Global, selectedSkills); err != nil {
			return err
		}
		spinner.Stop(fmt.Sprintf(msg.WikiInstalledSkillsFmt, resolved.WikiType, len(selectedSkills)))
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

// wikiInitCreateTargetDir 用项目 slug 作为新建文档库目录名。
func wikiInitCreateTargetDir(projectName string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	slug := common.WikiSlug(projectName)
	if slug == "" {
		return "", errors.New(ui.Messages().ErrorProjectRequired)
	}
	return filepath.Abs(filepath.Join(cwd, slug))
}

// runWikiInitLink 只登记已有文档库来源和可选代码库，不创建当前目录工作区。
func runWikiInitLink(out io.Writer, interactive bool, opts InitOptions) error {
	selectedSkills, cleanupSkills, err := selectedWikiInitSkills(&opts, interactive)
	if err != nil {
		return err
	}
	if cleanupSkills != nil {
		defer func() { _ = cleanupSkills() }()
	}
	if err := saveLinkedWikiRepoConfig(opts); err != nil {
		return err
	}
	repos := wikiInitCodeReposFromOptions(opts)
	for _, repo := range repos {
		if err := linkWikiInitCodeRepo(opts.ProjectName, repo.Slug, repo.Path); err != nil {
			return err
		}
	}
	if err := installWikiSkillsForAgents(opts.Agents, opts.Global, selectedSkills); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, ui.Messages().OutputWikiRepoSavedFmt, common.WikiSlug(opts.ProjectName))
	return err
}

// selectedWikiInitSkills 解析并按交互选择过滤 Wikimesh runtime skills。
func selectedWikiInitSkills(opts *InitOptions, interactive bool) ([]common.WikiSkill, func() error, error) {
	if strings.TrimSpace(opts.WikiType) == "" && interactive {
		items := make([]ui.Option, 0, len(common.BuiltinWikiSkillTypes()))
		for _, typ := range common.BuiltinWikiSkillTypes() {
			items = append(items, ui.Option{Value: typ.Name, Label: typ.Name, Hint: typ.Description})
		}
		value, cancelled, err := wikiSelectOne(ui.SelectOneOptions{
			Message: ui.Messages().PromptWikiType,
			Items:   items,
		})
		if err != nil {
			return nil, nil, err
		}
		if cancelled {
			return nil, nil, errors.New(ui.Messages().Cancelled)
		}
		opts.WikiType = value
	}
	if strings.TrimSpace(opts.WikiType) == "" {
		opts.WikiType = common.DefaultWikiSkillType()
	}
	source := common.NewWikimeshSkillsSource(opts.WikiType, "")
	ui.Step(fmt.Sprintf(ui.Messages().StepFetchingWikiSkillsFmt, wikiSkillSourceDisplay(source)))
	bundle, err := common.ResolveWikimeshSkills(opts.WikiType, "")
	if err != nil {
		return nil, nil, err
	}
	selected, err := resolveSelectedWikiSkills(bundle.Skills, *opts, interactive)
	if err != nil {
		if bundle.Cleanup != nil {
			_ = bundle.Cleanup()
		}
		return nil, nil, err
	}
	return selected, bundle.Cleanup, nil
}

// wikiSkillSourceDisplay 将 skill 来源转成用户可直接访问的展示地址。
func wikiSkillSourceDisplay(source common.WikiSkillSource) string {
	repo := strings.TrimSuffix(source.RepoURL, ".git")
	if source.Ref == "" || source.Subpath == "" {
		return source.Original
	}
	return fmt.Sprintf("%s/tree/%s/%s", repo, source.Ref, source.Subpath)
}

func installWikiSkillsForAgents(agents []string, global bool, skills []common.WikiSkill) error {
	if len(skills) == 0 {
		return nil
	}
	for _, agent := range agents {
		if err := common.InstallWikiSkills(agent, global, skills); err != nil {
			return err
		}
	}
	return nil
}

func installWikiSkillsForAgentsInProject(projectRoot string, agents []string, global bool, skills []common.WikiSkill) error {
	if len(skills) == 0 {
		return nil
	}
	if global {
		return installWikiSkillsForAgents(agents, true, skills)
	}
	for _, agent := range agents {
		targetRoot, err := wikiSkillTargetRootInProject(projectRoot, agent)
		if err != nil {
			return err
		}
		for _, skill := range skills {
			if err := common.CopyDir(skill.Dir, filepath.Join(targetRoot, skill.Name)); err != nil {
				return err
			}
		}
	}
	return nil
}

// wikiSkillTargetRootInProject 返回 init 新建文档库内的项目级 skill 安装目录。
func wikiSkillTargetRootInProject(projectRoot string, agent string) (string, error) {
	switch agent {
	case "", "codex":
		return filepath.Join(projectRoot, ".agents", "skills"), nil
	case "cursor":
		return filepath.Join(projectRoot, ".cursor", "skills"), nil
	case "claude":
		return filepath.Join(projectRoot, ".claude", "skills"), nil
	default:
		return "", fmt.Errorf(ui.Messages().ErrorUnsupportedAgentFmt, agent)
	}
}

// wikiInitCodeReposFromOptions 为显式代码仓或 code-dir 参数生成稳定关联条目。
func wikiInitCodeReposFromOptions(opts InitOptions) []InitCodeRepo {
	if len(opts.CodeRepos) > 0 {
		return append([]InitCodeRepo(nil), opts.CodeRepos...)
	}
	repos := make([]InitCodeRepo, 0, len(opts.CodeDirs))
	seenRepos := map[string]int{}
	for _, dir := range opts.CodeDirs {
		baseSlug := common.WikiSlug(filepath.Base(dir))
		seenRepos[baseSlug]++
		repoSlug := baseSlug
		if seenRepos[baseSlug] > 1 {
			repoSlug = fmt.Sprintf("%s-%d", baseSlug, seenRepos[baseSlug])
		}
		repos = append(repos, InitCodeRepo{Slug: repoSlug, Path: dir})
	}
	return repos
}

// ensureWikiGitignore 按项目级安装结果补充 .gitignore，避免 runtime 目录进入仓库。
func ensureWikiGitignore(projectRoot string, opts InitOptions) error {
	if opts.Global {
		return nil
	}
	paths := []string{filepath.Join(projectRoot, ".wikimesh")}
	for _, agent := range opts.Agents {
		installDir, err := wikiSkillTargetRootInProject(projectRoot, agent)
		if err != nil {
			return err
		}
		paths = append(paths, installDir)
	}
	return ensureProjectGitignore(projectRoot, paths...)
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
	selectedNames, cancelled, err := wikiSearchMultiselect(ui.SearchMultiselectOptions{
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
	for _, repo := range wikiInitCodeReposFromOptions(opts) {
		name := filepath.Base(repo.Path)
		cfg.CodeRepos = append(cfg.CodeRepos, common.WikiCodeRepo{
			Name:    name,
			Slug:    repo.Slug,
			Path:    repo.Path,
			Default: len(cfg.CodeRepos) == 0,
		})
	}
	return common.SaveWikiRepoConfig(cfg)
}

// saveLinkedWikiRepoConfig 将已有本地或远端文档库登记为用户级项目来源。
func saveLinkedWikiRepoConfig(opts InitOptions) error {
	msg := ui.Messages()
	slug := common.WikiSlug(opts.ProjectName)
	cfg := common.WikiRepoConfig{
		ProjectName: opts.ProjectName,
		ProjectSlug: slug,
		Language:    "zh",
	}
	if existing, err := common.LoadWikiRepoConfig(slug); err == nil {
		cfg = existing
		cfg.ProjectName = opts.ProjectName
		cfg.ProjectSlug = slug
		if strings.TrimSpace(cfg.Language) == "" {
			cfg.Language = "zh"
		}
	}
	switch opts.SourceType {
	case common.WikiRepoSourceLocal:
		cfg.Sources.Local = &common.WikiRepoSource{Type: common.WikiRepoSourceLocal, Path: opts.LocalPath}
		cfg.ActiveSource = common.WikiRepoSourceLocal
	case common.WikiRepoSourceRemote:
		cfg.Sources.Remote = &common.WikiRepoSource{Type: common.WikiRepoSourceRemote, URL: opts.RemoteURL}
		cfg.ActiveSource = common.WikiRepoSourceRemote
	default:
		return fmt.Errorf(msg.ErrorUnsupportedWikiSourceFmt, opts.SourceType)
	}
	return common.SaveWikiRepoConfig(cfg)
}

// linkWikiInitCodeRepo 复用 repo link 的配置规则保存可选代码库关联。
func linkWikiInitCodeRepo(project, repoSlug, path string) error {
	cfg, err := common.LoadWikiRepoConfig(common.WikiSlug(project))
	if err != nil {
		return err
	}
	repo := common.WikiCodeRepo{
		Name:    filepath.Base(path),
		Slug:    repoSlug,
		Path:    path,
		Default: len(cfg.CodeRepos) == 0,
	}
	replaced := false
	for i := range cfg.CodeRepos {
		if cfg.CodeRepos[i].Slug == repoSlug {
			repo.Default = cfg.CodeRepos[i].Default
			cfg.CodeRepos[i] = repo
			replaced = true
			break
		}
	}
	if !replaced {
		cfg.CodeRepos = append(cfg.CodeRepos, repo)
	}
	if !hasDefaultWikiCodeRepo(cfg.CodeRepos) && len(cfg.CodeRepos) > 0 {
		cfg.CodeRepos[0].Default = true
	}
	return common.SaveWikiRepoConfig(cfg)
}

func hasDefaultWikiCodeRepo(repos []common.WikiCodeRepo) bool {
	for _, repo := range repos {
		if repo.Default {
			return true
		}
	}
	return false
}

// normalizeWikiInitOptions 校验并补齐 init 参数，保证后续流程只处理规范值。
func normalizeWikiInitOptions(opts InitOptions) (InitOptions, error) {
	msg := ui.Messages()
	opts.Mode = strings.TrimSpace(opts.Mode)
	if opts.Mode == "" {
		opts.Mode = InitModeCreate
	}
	switch opts.Mode {
	case InitModeCreate, InitModeLink:
	default:
		return opts, fmt.Errorf(msg.ErrorUnsupportedInitModeFmt, opts.Mode)
	}
	opts.ProjectName = strings.TrimSpace(opts.ProjectName)
	if opts.ProjectName == "" {
		return opts, errors.New(msg.ErrorProjectRequired)
	}
	if opts.Mode == InitModeLink {
		return normalizeWikiInitLinkOptions(opts)
	}
	opts.WikiType = strings.TrimSpace(opts.WikiType)
	if opts.WikiType != "" {
		if _, err := common.LookupWikiSkillType(opts.WikiType); err != nil {
			return opts, err
		}
	}
	agents, err := normalizeWikiInitAgents(opts.Agent, opts.Agents)
	if err != nil {
		return opts, err
	}
	opts.Agents = agents
	opts.Agent = strings.Join(agents, ",")
	codeDirs, err := normalizeWikiInitCodeDirs(opts.CodeDirs)
	if err != nil {
		return opts, err
	}
	opts.CodeDirs = codeDirs
	codeRepos, err := normalizeWikiInitCodeRepos(opts.CodeRepos)
	if err != nil {
		return opts, err
	}
	opts.CodeRepos = codeRepos
	return opts, nil
}

// normalizeWikiInitLinkOptions 校验关联已有文档库所需来源参数。
func normalizeWikiInitLinkOptions(opts InitOptions) (InitOptions, error) {
	msg := ui.Messages()
	opts.SourceType = strings.TrimSpace(opts.SourceType)
	if opts.SourceType == "" {
		opts.SourceType = common.WikiRepoSourceLocal
	}
	switch opts.SourceType {
	case common.WikiRepoSourceLocal:
		opts.LocalPath = strings.TrimSpace(opts.LocalPath)
		if opts.LocalPath == "" {
			return opts, errors.New(msg.ErrorLocalWikiSourcePathRequired)
		}
		abs, err := filepath.Abs(opts.LocalPath)
		if err != nil {
			return opts, err
		}
		if info, err := os.Stat(abs); err != nil {
			return opts, err
		} else if !info.IsDir() {
			return opts, fmt.Errorf(msg.ErrorLocalWikiSourcePathNotDirFmt, abs)
		}
		opts.LocalPath = abs
	case common.WikiRepoSourceRemote:
		opts.RemoteURL = strings.TrimSpace(opts.RemoteURL)
		if opts.RemoteURL == "" {
			return opts, errors.New(msg.ErrorRemoteWikiSourceURLRequired)
		}
	default:
		return opts, fmt.Errorf(msg.ErrorUnsupportedWikiSourceFmt, opts.SourceType)
	}
	agents, err := normalizeWikiInitAgents(opts.Agent, opts.Agents)
	if err != nil {
		return opts, err
	}
	opts.Agents = agents
	opts.Agent = strings.Join(agents, ",")
	opts.WikiType = strings.TrimSpace(opts.WikiType)
	if opts.WikiType != "" {
		if _, err := common.LookupWikiSkillType(opts.WikiType); err != nil {
			return opts, err
		}
	}
	codeDirs, err := normalizeWikiInitCodeDirs(opts.CodeDirs)
	if err != nil {
		return opts, err
	}
	opts.CodeDirs = codeDirs
	codeRepos, err := normalizeWikiInitCodeRepos(opts.CodeRepos)
	if err != nil {
		return opts, err
	}
	opts.CodeRepos = codeRepos
	return opts, nil
}

// normalizeWikiInitAgents 归一化单值、逗号列表或交互多选得到的 agent。
func normalizeWikiInitAgents(agentText string, selected []string) ([]string, error) {
	msg := ui.Messages()
	values := make([]string, 0, len(selected)+1)
	values = append(values, selected...)
	values = append(values, splitCommaValues(agentText)...)
	if len(values) == 0 {
		values = []string{"codex"}
	}
	agents := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		agent := strings.TrimSpace(value)
		if agent == "" {
			continue
		}
		switch agent {
		case "codex", "cursor", "claude":
		default:
			return nil, fmt.Errorf(msg.ErrorUnsupportedAgentFmt, agent)
		}
		if _, ok := seen[agent]; ok {
			continue
		}
		seen[agent] = struct{}{}
		agents = append(agents, agent)
	}
	if len(agents) == 0 {
		agents = []string{"codex"}
	}
	return agents, nil
}

// normalizeWikiInitCodeDirs 归一化可选代码库目录；空列表表示不关联代码库。
func normalizeWikiInitCodeDirs(rawDirs []string) ([]string, error) {
	msg := ui.Messages()
	codeDirs := make([]string, 0, len(rawDirs))
	for _, raw := range rawDirs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		abs, err := filepath.Abs(raw)
		if err != nil {
			return nil, err
		}
		if info, err := os.Stat(abs); err != nil {
			return nil, err
		} else if !info.IsDir() {
			return nil, fmt.Errorf(msg.ErrorCodeDirNotDirFmt, abs)
		}
		codeDirs = append(codeDirs, abs)
	}
	return codeDirs, nil
}

// normalizeWikiInitCodeRepos 归一化显式关联代码库条目；空列表表示不关联代码库。
func normalizeWikiInitCodeRepos(rawRepos []InitCodeRepo) ([]InitCodeRepo, error) {
	msg := ui.Messages()
	repos := make([]InitCodeRepo, 0, len(rawRepos))
	for _, raw := range rawRepos {
		slug := common.WikiSlug(raw.Slug)
		if slug == "" {
			return nil, errors.New(msg.ErrorCodeRepoSlugRequired)
		}
		path := strings.TrimSpace(raw.Path)
		if path == "" {
			return nil, errors.New(msg.ErrorCodeRepoPathRequired)
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		if info, err := os.Stat(abs); err != nil {
			return nil, err
		} else if !info.IsDir() {
			return nil, fmt.Errorf(msg.ErrorCodeDirNotDirFmt, abs)
		}
		repos = append(repos, InitCodeRepo{Slug: slug, Path: abs})
	}
	return repos, nil
}

// createWikiWorkspace 创建 Wikimesh 工作区骨架和 qmd 配置。
func createWikiWorkspace(ctx context.Context, targetDir string, opts InitOptions) error {
	projectName := opts.ProjectName

	// 创建 Wikimesh 固定目录，保持 raw/wiki 两层边界清晰。
	for _, dir := range []string{
		"raw/requirements",
		"raw/designs",
		"raw/features",
		"raw/tests",
		"wiki/topics",
		"wiki/workflows",
		"wiki/troubleshooting",
		"wiki/outputs",
	} {
		if err := os.MkdirAll(filepath.Join(targetDir, dir), 0o755); err != nil {
			return err
		}
	}
	// 写入基础导航文件；已有文件不覆盖，避免破坏用户内容。
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
	templateValues := map[string]string{
		"PROJECT_NAME":     projectName,
		"PROJECT_SLUG":     slug,
		"AGENT":            strings.Join(opts.Agents, ", "),
		"RUNTIME_FILE":     strings.Join(runtimeFilesForAgents(opts.Agents), "、"),
		"PRIMARY_CODE_DIR": primaryCodeDir(opts.CodeDirs),
	}
	readme, err := renderInitTemplate("README.md", templateValues)
	if err != nil {
		return err
	}
	if err := writeFileIfMissing(filepath.Join(targetDir, "README.md"), readme); err != nil {
		return err
	}
	for _, runtimeFile := range runtimeFilesForAgents(opts.Agents) {
		runtimeValues := map[string]string{}
		for key, value := range templateValues {
			runtimeValues[key] = value
		}
		runtimeValues["RUNTIME_FILE"] = runtimeFile
		runtimeContent, err := renderInitTemplate(runtimeFile, runtimeValues)
		if err != nil {
			return err
		}
		if err := writeFileIfMissing(filepath.Join(targetDir, runtimeFile), runtimeContent); err != nil {
			return err
		}
	}

	return initializeWikiQMDCollection(ctx, targetDir, slug)
}

// runtimeFilesForAgents 返回选中 agent 需要写入的运行时入口文件。
func runtimeFilesForAgents(agents []string) []string {
	files := []string{}
	seen := map[string]struct{}{}
	for _, agent := range agents {
		file := "AGENTS.md"
		if agent == "claude" {
			file = "CLAUDE.md"
		}
		if _, ok := seen[file]; ok {
			continue
		}
		seen[file] = struct{}{}
		files = append(files, file)
	}
	if len(files) == 0 {
		return []string{"AGENTS.md"}
	}
	return files
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
	collection := qmd.Collection{Name: projectSlug, Path: filepath.Join(targetDir, "wiki"), Pattern: "**/*.md"}
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

// collectInitOptions 在缺少 init 参数时按收集交互回答。
func collectInitOptions(in io.Reader, out io.Writer, interactive bool, opts *InitOptions, agentProvided, codeDirsProvided bool) error {
	msg := ui.Messages()
	reader := bufio.NewReader(in)
	if strings.TrimSpace(opts.Mode) == "" {
		if interactive {
			value, cancelled, err := wikiSelectOne(ui.SelectOneOptions{
				Message: msg.PromptWikiInitMode,
				Items: []ui.Option{
					{Value: InitModeCreate, Label: msg.WikiInitModeCreateLabel},
					{Value: InitModeLink, Label: msg.WikiInitModeLinkLabel},
				},
			})
			if err != nil {
				return err
			}
			if cancelled {
				return errors.New(msg.Cancelled)
			}
			opts.Mode = value
		} else {
			value, err := promptWikiLine(reader, out, msg.PromptWikiInitMode, InitModeCreate)
			if err != nil {
				return err
			}
			opts.Mode = value
		}
	}
	if strings.TrimSpace(opts.ProjectName) == "" {
		value, err := promptWikiLine(reader, out, msg.PromptWikiProjectName, "")
		if err != nil {
			return err
		}
		opts.ProjectName = value
	}
	_ = opts.WikiTypeProvided
	if !agentProvided {
		if interactive {
			values, cancelled, err := wikiSearchMultiselect(ui.SearchMultiselectOptions{
				Message: msg.PromptWikiAgent,
				Items: []ui.Option{
					{Value: "codex", Label: "Codex"},
					{Value: "cursor", Label: "Cursor"},
					{Value: "claude", Label: "Claude Code"},
				},
				InitialSelected: []string{"codex"},
				Required:        true,
				MaxVisible:      3,
			})
			if err != nil {
				return err
			}
			if cancelled {
				return errors.New(msg.Cancelled)
			}
			opts.Agents = values
			opts.Agent = strings.Join(values, ",")
		} else {
			value, err := promptWikiLine(reader, out, msg.PromptWikiAgent, "codex")
			if err != nil {
				return err
			}
			opts.Agent = value
			opts.Agents = splitCommaValues(value)
		}
	}
	if strings.TrimSpace(opts.Mode) == InitModeLink {
		if err := collectWikiLinkOptions(reader, out, interactive, opts, codeDirsProvided); err != nil {
			return err
		}
		if !opts.ScopeProvided {
			return collectWikiInitScope(reader, out, interactive, opts)
		}
		return nil
	}
	if !codeDirsProvided && len(opts.CodeDirs) == 0 {
		if interactive {
			if err := collectWikiCodeReposInteractive(reader, out, opts); err != nil {
				return err
			}
		} else {
			value, err := promptWikiLine(reader, out, msg.PromptWikiCodeDirs, "")
			if err != nil {
				return err
			}
			opts.CodeDirs = splitCommaValues(value)
		}
	}
	if !opts.ScopeProvided {
		return collectWikiInitScope(reader, out, interactive, opts)
	}
	return nil
}

// collectWikiInitScope 收集 runtime skill 安装范围。
func collectWikiInitScope(in *bufio.Reader, out io.Writer, interactive bool, opts *InitOptions) error {
	msg := ui.Messages()
	if interactive {
		value, cancelled, err := wikiSelectOne(ui.SelectOneOptions{
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
		return nil
	}
	value, err := promptWikiLine(in, out, msg.PromptWikiScope, "project")
	if err != nil {
		return err
	}
	opts.Global = strings.EqualFold(strings.TrimSpace(value), "global")
	return nil
}

// collectWikiLinkOptions 收集关联已有文档库所需来源和可选代码库。
func collectWikiLinkOptions(in *bufio.Reader, out io.Writer, interactive bool, opts *InitOptions, codeDirsProvided bool) error {
	msg := ui.Messages()
	if strings.TrimSpace(opts.SourceType) == "" {
		if interactive {
			value, cancelled, err := wikiSelectOne(ui.SelectOneOptions{
				Message: msg.PromptWikiRepoSource,
				Items: []ui.Option{
					{Value: common.WikiRepoSourceLocal, Label: msg.WikiRepoSourceLocalLabel},
					{Value: common.WikiRepoSourceRemote, Label: msg.WikiRepoSourceRemoteLabel},
				},
			})
			if err != nil {
				return err
			}
			if cancelled {
				return errors.New(msg.Cancelled)
			}
			opts.SourceType = value
		} else {
			value, err := promptWikiLine(in, out, msg.PromptWikiRepoSource, common.WikiRepoSourceLocal)
			if err != nil {
				return err
			}
			opts.SourceType = value
		}
	}
	switch strings.TrimSpace(opts.SourceType) {
	case common.WikiRepoSourceLocal:
		if strings.TrimSpace(opts.LocalPath) == "" {
			defaultDir, err := os.Getwd()
			if err != nil {
				defaultDir = "."
			}
			value, err := promptWikiLine(in, out, msg.PromptWikiRepoLocalPath, defaultDir)
			if err != nil {
				return err
			}
			opts.LocalPath = value
		}
	case common.WikiRepoSourceRemote:
		if strings.TrimSpace(opts.RemoteURL) == "" {
			value, err := promptWikiLine(in, out, msg.PromptWikiRepoRemoteURL, "")
			if err != nil {
				return err
			}
			opts.RemoteURL = value
		}
	}
	if codeDirsProvided || len(opts.CodeDirs) > 0 {
		return nil
	}
	if interactive {
		return collectWikiCodeReposInteractive(in, out, opts)
	}
	value, err := promptWikiLine(in, out, msg.PromptWikiCodeDirs, "")
	if err != nil {
		return err
	}
	opts.CodeDirs = splitCommaValues(value)
	return nil
}

// collectWikiCodeReposInteractive 按循环收集可选代码库。
func collectWikiCodeReposInteractive(in io.Reader, out io.Writer, opts *InitOptions) error {
	msg := ui.Messages()
	linkedAny := false
	for {
		prompt := msg.PromptWikiRepoLinkCode
		items := []ui.Option{
			{Value: "link", Label: msg.WikiRepoLinkCodeLabel},
			{Value: "continue", Label: msg.WikiRepoContinueLabel},
		}
		if linkedAny {
			prompt = msg.PromptWikiRepoLinkMore
			items = []ui.Option{
				{Value: "link", Label: msg.WikiRepoLinkAnotherLabel},
				{Value: "finish", Label: msg.WikiRepoFinishLabel},
			}
		}
		choice, cancelled, err := wikiSelectOne(ui.SelectOneOptions{Message: prompt, Items: items})
		if err != nil {
			return err
		}
		if cancelled || choice == "continue" || choice == "finish" {
			return nil
		}
		repoSlug, err := promptWikiLine(in, out, msg.PromptWikiRepoCodeName, "")
		if err != nil {
			return err
		}
		repoPath, err := promptWikiLine(in, out, msg.PromptWikiRepoCodePath, ".")
		if err != nil {
			return err
		}
		opts.CodeRepos = append(opts.CodeRepos, InitCodeRepo{Slug: repoSlug, Path: repoPath})
		linkedAny = true
	}
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
