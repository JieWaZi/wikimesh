package wikiinit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JieWaZi/wikimesh/internal/app/wikiapp"
)

// codeReposFromOptions 为显式代码仓或 code-dir 参数生成稳定关联条目。
func codeReposFromOptions(opts Options) []CodeRepo {
	if len(opts.CodeRepos) > 0 {
		return append([]CodeRepo(nil), opts.CodeRepos...)
	}
	repos := make([]CodeRepo, 0, len(opts.CodeDirs))
	seenRepos := map[string]int{}
	for _, dir := range opts.CodeDirs {
		// code-dir 没有显式 slug 时使用目录名派生；重复目录名追加序号，保证配置稳定。
		baseSlug := wikiapp.Slug(filepath.Base(dir))
		seenRepos[baseSlug]++
		repoSlug := baseSlug
		if seenRepos[baseSlug] > 1 {
			repoSlug = fmt.Sprintf("%s-%d", baseSlug, seenRepos[baseSlug])
		}
		repos = append(repos, CodeRepo{Slug: repoSlug, Path: dir})
	}
	return repos
}

// saveRepoConfig 将 init 创建的本地工作区登记到用户级项目配置。
func saveRepoConfig(targetDir string, opts Options) error {
	slug := wikiapp.Slug(opts.ProjectName)
	cfg := wikiapp.RepoConfig{
		ProjectName:  opts.ProjectName,
		ProjectSlug:  slug,
		Language:     "zh",
		ActiveSource: wikiapp.SourceLocal,
	}
	// 已有项目配置可能包含远端来源或历史代码库；创建本地库时只覆盖本次负责的字段。
	if existing, err := wikiapp.LoadRepoConfig(slug); err == nil {
		cfg = existing
		cfg.ProjectName = opts.ProjectName
		cfg.ProjectSlug = slug
		if strings.TrimSpace(cfg.Language) == "" {
			cfg.Language = "zh"
		}
	}
	cfg.ActiveSource = wikiapp.SourceLocal
	cfg.Sources.Local = &wikiapp.RepoSource{Type: wikiapp.SourceLocal, Path: targetDir}
	cfg.CodeRepos = nil
	// init 创建模式下，代码仓列表以本次参数为准，避免保留已经无效的旧路径。
	for _, repo := range codeReposFromOptions(opts) {
		name := filepath.Base(repo.Path)
		cfg.CodeRepos = append(cfg.CodeRepos, wikiapp.CodeRepo{
			Name:    name,
			Slug:    repo.Slug,
			Path:    repo.Path,
			Default: len(cfg.CodeRepos) == 0,
		})
	}
	if err := wikiapp.SaveRepoConfig(cfg); err != nil {
		return err
	}
	for _, repo := range cfg.CodeRepos {
		if err := EnsureCodeRepoLink(repo.Path, targetDir, cfg.ProjectSlug); err != nil {
			return err
		}
	}
	return nil
}

// saveLinkedRepoConfig 将已有本地或远端文档库登记为用户级项目来源。
func saveLinkedRepoConfig(opts Options) error {
	slug := wikiapp.Slug(opts.ProjectName)
	cfg := wikiapp.RepoConfig{
		ProjectName: opts.ProjectName,
		ProjectSlug: slug,
		Language:    "zh",
	}
	// link 模式只切换来源信息，不主动丢弃已有代码库关联。
	if existing, err := wikiapp.LoadRepoConfig(slug); err == nil {
		cfg = existing
		cfg.ProjectName = opts.ProjectName
		cfg.ProjectSlug = slug
		if strings.TrimSpace(cfg.Language) == "" {
			cfg.Language = "zh"
		}
	}
	switch opts.SourceType {
	case wikiapp.SourceLocal:
		cfg.Sources.Local = &wikiapp.RepoSource{Type: wikiapp.SourceLocal, Path: opts.LocalPath}
		cfg.ActiveSource = wikiapp.SourceLocal
	case wikiapp.SourceRemote:
		cfg.Sources.Remote = &wikiapp.RepoSource{Type: wikiapp.SourceRemote, URL: opts.RemoteURL}
		cfg.ActiveSource = wikiapp.SourceRemote
	default:
		return fmt.Errorf("unsupported wiki source type %q", opts.SourceType)
	}
	return wikiapp.SaveRepoConfig(cfg)
}

// linkCodeRepo 复用 repo link 的配置规则保存可选代码库关联。
func linkCodeRepo(project, repoSlug, path string) error {
	cfg, err := wikiapp.LoadRepoConfig(wikiapp.Slug(project))
	if err != nil {
		return err
	}
	repo := wikiapp.CodeRepo{
		Name:    filepath.Base(path),
		Slug:    repoSlug,
		Path:    path,
		Default: len(cfg.CodeRepos) == 0,
	}
	replaced := false
	for i := range cfg.CodeRepos {
		if cfg.CodeRepos[i].Slug == repoSlug {
			// 同 slug 视为更新路径，保留原来的 default 选择。
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
	if err := wikiapp.SaveRepoConfig(cfg); err != nil {
		return err
	}
	devwikiRoot := ""
	if cfg.ActiveSource == wikiapp.SourceLocal && cfg.Sources.Local != nil {
		devwikiRoot = cfg.Sources.Local.Path
	}
	return EnsureCodeRepoLink(repo.Path, devwikiRoot, cfg.ProjectSlug)
}

// hasDefaultWikiCodeRepo 判断代码库列表是否已经存在默认项。
func hasDefaultWikiCodeRepo(repos []wikiapp.CodeRepo) bool {
	for _, repo := range repos {
		if repo.Default {
			return true
		}
	}
	return false
}

// ensureGitignore 按项目级安装结果补充 .gitignore，避免 runtime 目录进入仓库。
func ensureGitignore(projectRoot string, opts Options) error {
	if opts.Global {
		return nil
	}
	paths := []string{filepath.Join(projectRoot, ".wikimesh")}
	for _, agent := range opts.Agents {
		installDir, err := skillTargetRootInProject(projectRoot, agent)
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
		// 先把绝对路径归一到项目内条目，再用规范化结果去重。
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
		// 忽略注释和空行，只比较有效规则，避免重复写入。
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
