package daemonapp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JieWaZi/wikimesh/internal/app/wikiapp"
)

// ResolveCwd 根据显式 cwd 或 wikimesh project 解析 Codex 执行目录。
func ResolveCwd(cwd string, project string) (string, error) {
	// 显式 cwd 优先，方便调试或 web 侧覆盖默认文档库目录。
	if strings.TrimSpace(cwd) != "" {
		return validateCwd(cwd)
	}
	// project 是文档库名称/slug；ResolveRoot 会读取用户级 repo config。
	if strings.TrimSpace(project) != "" {
		root, err := wikiapp.ResolveRoot("", project)
		if err != nil {
			return "", err
		}
		return validateCwd(root)
	}
	return "", nil
}

// ResolveRunCwd 按 run cwd、run project、session cwd、session project 的顺序解析目录。
func ResolveRunCwd(runCwd string, runProject string, sessionCwd string, sessionProject string) (string, error) {
	for _, candidate := range []struct {
		cwd     string
		project string
	}{
		{cwd: runCwd},
		{project: runProject},
		{cwd: sessionCwd},
		{project: sessionProject},
	} {
		if strings.TrimSpace(candidate.cwd) != "" || strings.TrimSpace(candidate.project) != "" {
			return ResolveCwd(candidate.cwd, candidate.project)
		}
	}
	return "", nil
}

func validateCwd(cwd string) (string, error) {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return "", nil
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("cwd %q is not a directory", abs)
	}
	return abs, nil
}
