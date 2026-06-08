package common

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// WikiSkill 描述一个可安装的 Wikimesh runtime skill。
type WikiSkill struct {
	// Name 是 skill frontmatter 中的稳定名称。
	Name string
	// Description 是 skill frontmatter 中的简短说明。
	Description string
	// Dir 是 skill 源目录的绝对路径。
	Dir string
}

// WikiSkillSource 描述 Wikimesh runtime skills 的下载来源。
type WikiSkillSource struct {
	// Original 是可展示和可复现的来源字符串。
	Original string
	// Type 是来源类型；当前内置安装固定使用 github。
	Type string
	// RepoURL 是 git clone 使用的仓库地址。
	RepoURL string
	// Ref 是分支、标签或提交。
	Ref string
	// Subpath 是仓库内 skills 根目录。
	Subpath string
}

// WikiSkillBundle 是解析后的 skill 集合及其临时资源清理函数。
type WikiSkillBundle struct {
	Source  WikiSkillSource
	Skills  []WikiSkill
	Cleanup func() error
}

// WikiSkillType 描述一类可安装的 Wiki runtime skills。
type WikiSkillType struct {
	Name        string
	Description string
	Subpath     string
}

const (
	defaultWikimeshSkillsRef = "main"
	wikimeshSkillsRepoURL    = "https://github.com/JieWaZi/wikimesh.git"
	defaultWikiSkillType     = "devwiki"
	wikimeshSkillsRefEnv     = "WIKIMESH_SKILLS_REF"
	wikimeshCloneTimeout     = 60 * time.Second
)

var builtinWikiSkillTypes = []WikiSkillType{
	{
		Name:        "devwiki",
		Description: "面向软件工程的 DevWiki",
		Subpath:     "skills/devwiki",
	},
}

// ResolveWikimeshSkills 下载并发现 Wikimesh runtime skills。
var ResolveWikimeshSkills = resolveWikimeshSkills

// DefaultWikiSkillType 返回默认安装的 Wiki 类型。
func DefaultWikiSkillType() string {
	return defaultWikiSkillType
}

// BuiltinWikiSkillTypes 返回当前内置的 Wiki skill 类型。
func BuiltinWikiSkillTypes() []WikiSkillType {
	return append([]WikiSkillType(nil), builtinWikiSkillTypes...)
}

// LookupWikiSkillType 查找内置 Wiki skill 类型。
func LookupWikiSkillType(name string) (WikiSkillType, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = defaultWikiSkillType
	}
	for _, typ := range builtinWikiSkillTypes {
		if typ.Name == name {
			return typ, nil
		}
	}
	return WikiSkillType{}, fmt.Errorf("unsupported wiki skill type %q", name)
}

// NewWikimeshSkillsSource 构造内置 Wikimesh skills 的 GitHub 来源。
func NewWikimeshSkillsSource(wikiType string, ref string) WikiSkillSource {
	typ, err := LookupWikiSkillType(wikiType)
	if err != nil {
		typ = WikiSkillType{Name: strings.TrimSpace(wikiType), Subpath: filepath.ToSlash(filepath.Join("skills", strings.TrimSpace(wikiType)))}
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		ref = strings.TrimSpace(os.Getenv(wikimeshSkillsRefEnv))
	}
	if ref == "" {
		ref = defaultWikimeshSkillsRef
	}
	return WikiSkillSource{
		Original: fmt.Sprintf("JieWaZi/wikimesh/%s#%s", typ.Subpath, ref),
		Type:     "github",
		RepoURL:  wikimeshSkillsRepoURL,
		Ref:      ref,
		Subpath:  typ.Subpath,
	}
}

func resolveWikimeshSkills(wikiType string, ref string) (WikiSkillBundle, error) {
	if _, err := LookupWikiSkillType(wikiType); err != nil {
		return WikiSkillBundle{}, err
	}
	source := NewWikimeshSkillsSource(wikiType, ref)
	root, cleanup, err := resolveWikiSkillSource(context.Background(), source)
	if err != nil {
		return WikiSkillBundle{}, err
	}

	skillsRoot := filepath.Join(root, filepath.FromSlash(source.Subpath))
	found, err := DiscoverWikiSkills(skillsRoot)
	if err != nil {
		_ = cleanup()
		return WikiSkillBundle{}, err
	}
	return WikiSkillBundle{Source: source, Skills: found, Cleanup: cleanup}, nil
}

func resolveWikiSkillSource(ctx context.Context, source WikiSkillSource) (string, func() error, error) {
	if source.Type != "github" {
		return "", nil, fmt.Errorf("unsupported wikimesh skill source type %q", source.Type)
	}
	tempDir, err := os.MkdirTemp("", "wikimesh-skills-*")
	if err != nil {
		return "", nil, err
	}

	cloneCtx, cancel := context.WithTimeout(ctx, wikimeshCloneTimeout)
	defer cancel()
	if err := cloneWikiSkills(cloneCtx, source, tempDir); err != nil {
		_ = os.RemoveAll(tempDir)
		return "", nil, err
	}
	return tempDir, func() error { return os.RemoveAll(tempDir) }, nil
}

func cloneWikiSkills(ctx context.Context, source WikiSkillSource, tempDir string) error {
	args := []string{"clone", "--depth", "1"}
	if strings.TrimSpace(source.Ref) != "" {
		args = append(args, "--branch", source.Ref)
	}
	args = append(args, source.RepoURL, tempDir)
	return runGit(ctx, "", args...)
}

func runGit(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(output))
	}
	return nil
}

// DiscoverWikiSkills 扫描指定目录下包含 SKILL.md 的 skill 目录。
func DiscoverWikiSkills(root string) ([]WikiSkill, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var found []WikiSkill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillDir := filepath.Join(root, entry.Name())
		if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		skill, err := ParseWikiSkill(skillDir)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(skill.Name) == "" {
			continue
		}
		found = append(found, skill)
	}
	sort.Slice(found, func(i, j int) bool { return found[i].Name < found[j].Name })
	return found, nil
}

// ParseWikiSkill 读取 SKILL.md frontmatter 中的名称和说明。
func ParseWikiSkill(dir string) (WikiSkill, error) {
	data, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		return WikiSkill{}, err
	}
	meta, _, ok := splitFrontmatter(data)
	if !ok {
		return WikiSkill{}, fmt.Errorf("%s: missing YAML frontmatter", filepath.Join(dir, "SKILL.md"))
	}
	var raw struct {
		// Name 是 skill frontmatter 中声明的稳定名称。
		Name string `yaml:"name"`
		// Description 是 skill frontmatter 中声明的简短说明。
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal(meta, &raw); err != nil {
		return WikiSkill{}, err
	}
	name := strings.TrimSpace(raw.Name)
	if name == "" {
		name = filepath.Base(dir)
	}
	return WikiSkill{Name: name, Description: strings.TrimSpace(raw.Description), Dir: dir}, nil
}

// InstallWikiSkills 把选中的 Wikimesh skills 安装到指定 Agent 的目标目录。
func InstallWikiSkills(agent string, global bool, selected []WikiSkill) error {
	targetRoot, err := WikiSkillTargetRoot(agent, global)
	if err != nil {
		return err
	}
	for _, skill := range selected {
		if err := CopyDir(skill.Dir, filepath.Join(targetRoot, skill.Name)); err != nil {
			return err
		}
	}
	return nil
}

// WikimeshSkillsRoot 根据当前源码文件定位仓库内置 skills/<wiki-type> 目录。
func WikimeshSkillsRoot(wikiType ...string) (string, error) {
	typName := defaultWikiSkillType
	if len(wikiType) > 0 {
		typName = wikiType[0]
	}
	typ, err := LookupWikiSkillType(typName)
	if err != nil {
		return "", err
	}
	_, file, _, ok := goruntime.Caller(0)
	if !ok {
		return "", fmt.Errorf("cannot locate source root")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	return filepath.Join(root, filepath.FromSlash(typ.Subpath)), nil
}

// WikiSkillTargetRoot 返回指定 Agent 和安装范围对应的 skill 安装目录。
func WikiSkillTargetRoot(agent string, global bool) (string, error) {
	if global {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		switch agent {
		case "", "codex":
			return filepath.Join(home, ".codex", "skills"), nil
		case "cursor":
			return filepath.Join(home, ".cursor", "skills"), nil
		case "claude":
			return filepath.Join(home, ".claude", "skills"), nil
		default:
			return "", fmt.Errorf("unsupported agent %q", agent)
		}
	}
	switch agent {
	case "", "codex":
		return filepath.Abs(filepath.Join(".agents", "skills"))
	case "cursor":
		return filepath.Abs(filepath.Join(".cursor", "skills"))
	case "claude":
		return filepath.Abs(filepath.Join(".claude", "skills"))
	default:
		return "", fmt.Errorf("unsupported agent %q", agent)
	}
}

// CopyDir 递归复制目录，用于安装 Wikimesh runtime skills。
func CopyDir(src, dst string) error {
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
