package initcmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JieWaZi/wikimesh/internal/app/skillapp"
	"github.com/JieWaZi/wikimesh/internal/app/wikiapp"
	"github.com/JieWaZi/wikimesh/internal/app/wikiinit"
	"github.com/JieWaZi/wikimesh/internal/ui"
)

// InitOptions 描述 `wikimesh init` 的初始化参数。
type InitOptions struct {
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
	CodeRepos []InitCodeRepo
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
	// ScopeProvided 表示用户已经通过参数指定安装范围。
	ScopeProvided bool
	// WikiTypeProvided 表示用户已经通过参数指定 skill 类型。
	WikiTypeProvided bool
}

// InitCodeRepo 描述 init 关联文档库时收集的代码仓条目。
type InitCodeRepo = wikiinit.CodeRepo

// appOptions 将 CLI 已归一化参数转换为 app 层输入，避免命令层直接暴露 app 结构细节。
func (opts InitOptions) appOptions() wikiinit.Options {
	return wikiinit.Options{
		Mode:        opts.Mode,
		ProjectName: opts.ProjectName,
		WikiType:    opts.WikiType,
		Agent:       opts.Agent,
		Agents:      append([]string(nil), opts.Agents...),
		CodeDirs:    append([]string(nil), opts.CodeDirs...),
		CodeRepos:   append([]wikiinit.CodeRepo(nil), opts.CodeRepos...),
		SourceType:  opts.SourceType,
		LocalPath:   opts.LocalPath,
		RemoteURL:   opts.RemoteURL,
		Global:      opts.Global,
		Yes:         opts.Yes,
	}
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
		if _, err := skillapp.LookupType(opts.WikiType); err != nil {
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
		opts.SourceType = wikiapp.SourceLocal
	}
	switch opts.SourceType {
	case wikiapp.SourceLocal:
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
	case wikiapp.SourceRemote:
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
		if _, err := skillapp.LookupType(opts.WikiType); err != nil {
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
		slug := wikiapp.Slug(raw.Slug)
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
