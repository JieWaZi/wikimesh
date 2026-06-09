package wikiinit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/JieWaZi/wikimesh/internal/app/skillapp"
	"github.com/JieWaZi/wikimesh/internal/app/wikiapp"
)

const (
	// ModeCreate 表示在当前目录新建 Wikimesh 文档库。
	ModeCreate = "create"
	// ModeLink 表示登记并关联已有 Wikimesh 文档库。
	ModeLink = "link"
)

// Options 描述初始化或关联 Wikimesh 文档库所需的规范参数。
type Options struct {
	// Mode 是初始化模式：新建文档库或关联已有文档库。
	Mode string
	// ProjectName 是当前 Wikimesh 工作区的展示名称。
	ProjectName string
	// WikiType 是要安装的 skill 类型。
	WikiType string
	// Agent 是目标 Agent 的逗号分隔文本。
	Agent string
	// Agents 是归一化后的目标 Agent 列表。
	Agents []string
	// CodeDirs 是与当前知识库关联的代码仓目录列表。
	CodeDirs []string
	// CodeRepos 是显式输入的代码仓标识和路径。
	CodeRepos []CodeRepo
	// SourceType 是关联文档库时使用的来源类型。
	SourceType string
	// LocalPath 是关联本地文档库时的路径。
	LocalPath string
	// RemoteURL 是关联远端文档库时的 URL。
	RemoteURL string
	// Global 表示把 runtime skills 安装到用户主目录。
	Global bool
	// Yes 表示跳过交互确认并采用默认选择。
	Yes bool
}

// CodeRepo 描述 init 关联文档库时收集的代码仓条目。
type CodeRepo struct {
	// Slug 是用户输入的代码仓标识。
	Slug string
	// Path 是代码仓本地路径。
	Path string
}

// Service 编排 Wikimesh 文档库创建、登记和 runtime skill 安装。
type Service struct{}

// NewService 构造文档库初始化服务。
func NewService() Service {
	return Service{}
}

// CreateTargetDir 根据项目名计算当前目录下的新文档库路径。
func CreateTargetDir(projectName string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	slug := wikiapp.Slug(projectName)
	if slug == "" {
		return "", fmt.Errorf("project is required")
	}
	return filepath.Abs(filepath.Join(cwd, slug))
}

// Create 在当前目录创建新的 Wikimesh 文档库并登记用户级项目配置。
func (Service) Create(ctx context.Context, targetDir string, opts Options, skills []skillapp.Skill) error {
	if err := CreateWorkspace(ctx, targetDir, opts); err != nil {
		return err
	}
	if err := SaveRepoConfig(targetDir, opts); err != nil {
		return err
	}
	if err := InstallSkillsForAgentsInProject(targetDir, opts.Agents, opts.Global, skills); err != nil {
		return err
	}
	return EnsureGitignore(targetDir, opts)
}

// CreateWorkspace 创建 Wikimesh 工作区骨架和 qmd 配置。
func CreateWorkspace(ctx context.Context, targetDir string, opts Options) error {
	if err := createWorkspace(ctx, targetDir, opts); err != nil {
		return err
	}
	return nil
}

// SaveRepoConfig 将新建文档库登记到用户级项目配置。
func SaveRepoConfig(targetDir string, opts Options) error {
	return saveRepoConfig(targetDir, opts)
}

// EnsureGitignore 按初始化结果维护文档库 .gitignore。
func EnsureGitignore(projectRoot string, opts Options) error {
	return ensureGitignore(projectRoot, opts)
}

// Link 登记已有本地或远端文档库，并保存可选代码仓关联。
func (Service) Link(opts Options, skills []skillapp.Skill) error {
	if err := saveLinkedRepoConfig(opts); err != nil {
		return err
	}
	repos := codeReposFromOptions(opts)
	for _, repo := range repos {
		if err := linkCodeRepo(opts.ProjectName, repo.Slug, repo.Path); err != nil {
			return err
		}
	}
	return InstallSkillsForAgents(opts.Agents, opts.Global, skills)
}

// InstallSkillsForAgents 把选中的 runtime skills 安装到每个目标 Agent 的默认位置。
func InstallSkillsForAgents(agents []string, global bool, skills []skillapp.Skill) error {
	if len(skills) == 0 {
		return nil
	}
	for _, agent := range agents {
		if err := skillapp.Install(agent, global, skills); err != nil {
			return err
		}
	}
	return nil
}

// InstallSkillsForAgentsInProject 把选中的 runtime skills 安装到新建文档库内。
func InstallSkillsForAgentsInProject(projectRoot string, agents []string, global bool, skills []skillapp.Skill) error {
	if len(skills) == 0 {
		return nil
	}
	if global {
		return InstallSkillsForAgents(agents, true, skills)
	}
	for _, agent := range agents {
		targetRoot, err := skillTargetRootInProject(projectRoot, agent)
		if err != nil {
			return err
		}
		for _, skill := range skills {
			if err := skillapp.CopyDir(skill.Dir, filepath.Join(targetRoot, skill.Name)); err != nil {
				return err
			}
		}
	}
	return nil
}

// skillTargetRootInProject 返回 init 新建文档库内的项目级 skill 安装目录。
func skillTargetRootInProject(projectRoot string, agent string) (string, error) {
	switch agent {
	case "", "codex":
		return filepath.Join(projectRoot, ".agents", "skills"), nil
	case "cursor":
		return filepath.Join(projectRoot, ".cursor", "skills"), nil
	case "claude":
		return filepath.Join(projectRoot, ".claude", "skills"), nil
	default:
		return "", fmt.Errorf("unsupported agent %q", agent)
	}
}
