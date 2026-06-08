package skillapp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"time"
)

const (
	defaultWikimeshSkillsRef = "main"
	wikimeshSkillsRepoURL    = "https://github.com/JieWaZi/wikimesh.git"
	defaultType              = "devwiki"
	wikimeshSkillsRefEnv     = "WIKIMESH_SKILLS_REF"
	wikimeshCloneTimeout     = 60 * time.Second
)

// Skill 描述一个可安装的 Wikimesh runtime skill。
type Skill struct {
	// Name 是 skill frontmatter 中的稳定名称。
	Name string
	// Description 是 skill frontmatter 中的简短说明。
	Description string
	// Dir 是 skill 源目录的绝对路径。
	Dir string
}

// Source 描述 Wikimesh runtime skills 的下载来源。
type Source struct {
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

// Bundle 是解析后的 skill 集合及其临时资源清理函数。
type Bundle struct {
	// Source 是解析出这些 skill 时使用的下载来源。
	Source Source
	// Skills 是可安装的 skill 列表。
	Skills []Skill
	// Cleanup 释放临时 clone 目录等资源；调用方应在安装完成后执行。
	Cleanup func() error
}

// Type 描述一类可安装的 Wiki runtime skills。
type Type struct {
	// Name 是命令行和配置中使用的类型名称。
	Name string
	// Description 是交互选择时展示的类型说明。
	Description string
	// Subpath 是仓库内该类型 skills 的相对目录。
	Subpath string
}

var builtinTypes = []Type{
	{
		Name:        "devwiki",
		Description: "面向软件工程的 DevWiki",
		Subpath:     "skills/devwiki",
	},
}

// ResolveWikimeshSkills 下载并发现 Wikimesh runtime skills。
var ResolveWikimeshSkills = resolveWikimeshSkills

// DefaultType 返回默认安装的 Wiki 类型。
func DefaultType() string {
	return defaultType
}

// BuiltinTypes 返回当前内置的 Wiki skill 类型。
func BuiltinTypes() []Type {
	return append([]Type(nil), builtinTypes...)
}

// LookupType 查找内置 Wiki skill 类型。
func LookupType(name string) (Type, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = defaultType
	}
	for _, typ := range builtinTypes {
		if typ.Name == name {
			return typ, nil
		}
	}
	return Type{}, fmt.Errorf("unsupported wiki skill type %q", name)
}

// NewSource 构造内置 Wikimesh skills 的 GitHub 来源。
func NewSource(wikiType string, ref string) Source {
	typ, err := LookupType(wikiType)
	if err != nil {
		typ = Type{Name: strings.TrimSpace(wikiType), Subpath: filepath.ToSlash(filepath.Join("skills", strings.TrimSpace(wikiType)))}
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		ref = strings.TrimSpace(os.Getenv(wikimeshSkillsRefEnv))
	}
	if ref == "" {
		ref = defaultWikimeshSkillsRef
	}
	return Source{
		Original: fmt.Sprintf("JieWaZi/wikimesh/%s#%s", typ.Subpath, ref),
		Type:     "github",
		RepoURL:  wikimeshSkillsRepoURL,
		Ref:      ref,
		Subpath:  typ.Subpath,
	}
}

// resolveWikimeshSkills 解析来源、临时 clone 仓库，并扫描指定类型的 runtime skills。
func resolveWikimeshSkills(wikiType string, ref string) (Bundle, error) {
	if _, err := LookupType(wikiType); err != nil {
		return Bundle{}, err
	}
	source := NewSource(wikiType, ref)
	root, cleanup, err := resolveSource(context.Background(), source)
	if err != nil {
		return Bundle{}, err
	}

	// clone 成功后再扫描子目录，保证 cleanup 在安装阶段结束后由调用方统一执行。
	skillsRoot := filepath.Join(root, filepath.FromSlash(source.Subpath))
	found, err := Discover(skillsRoot)
	if err != nil {
		_ = cleanup()
		return Bundle{}, err
	}
	return Bundle{Source: source, Skills: found, Cleanup: cleanup}, nil
}

// BuiltinRoot 根据当前源码文件定位仓库内置 skills/<wiki-type> 目录。
func BuiltinRoot(wikiType ...string) (string, error) {
	typName := defaultType
	if len(wikiType) > 0 {
		typName = wikiType[0]
	}
	typ, err := LookupType(typName)
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

// resolveSource 按来源类型拉取 skill 资产，并返回临时根目录和清理函数。
func resolveSource(ctx context.Context, source Source) (string, func() error, error) {
	if source.Type != "github" {
		return "", nil, fmt.Errorf("unsupported wikimesh skill source type %q", source.Type)
	}
	tempDir, err := os.MkdirTemp("", "wikimesh-skills-*")
	if err != nil {
		return "", nil, err
	}

	cloneCtx, cancel := context.WithTimeout(ctx, wikimeshCloneTimeout)
	defer cancel()
	if err := cloneSkills(cloneCtx, source, tempDir); err != nil {
		_ = os.RemoveAll(tempDir)
		return "", nil, err
	}
	return tempDir, func() error { return os.RemoveAll(tempDir) }, nil
}

// cloneSkills 执行浅 clone，避免安装 runtime skills 时拉取完整历史。
func cloneSkills(ctx context.Context, source Source, tempDir string) error {
	args := []string{"clone", "--depth", "1"}
	if strings.TrimSpace(source.Ref) != "" {
		args = append(args, "--branch", source.Ref)
	}
	args = append(args, source.RepoURL, tempDir)
	return runGit(ctx, "", args...)
}

// runGit 封装 git 命令执行，并禁用交互式凭据提示。
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
