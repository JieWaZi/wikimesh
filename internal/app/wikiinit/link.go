package wikiinit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	codeLinkStartMarker = "<!-- wikimesh:devwiki-link:start -->"
	codeLinkEndMarker   = "<!-- wikimesh:devwiki-link:end -->"
)

// EnsureCodeRepoLink 将代码仓与 DevWiki 项目的托管关联块写入运行时入口文件。
func EnsureCodeRepoLink(codeRoot string, devwikiRoot string, projectSlug string) error {
	if strings.TrimSpace(codeRoot) == "" {
		return fmt.Errorf("code repo path is required")
	}
	absCodeRoot, err := filepath.Abs(codeRoot)
	if err != nil {
		return err
	}
	info, err := os.Stat(absCodeRoot)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("code repo path is not a directory: %s", absCodeRoot)
	}

	devwikiRoot = strings.TrimSpace(devwikiRoot)
	if devwikiRoot != "" {
		devwikiRoot, err = filepath.Abs(devwikiRoot)
		if err != nil {
			return err
		}
	}
	block := renderCodeRepoLinkBlock(devwikiRoot, projectSlug)
	wrote := false
	for _, filename := range []string{"AGENTS.md", "CLAUDE.md"} {
		targetPath := filepath.Join(absCodeRoot, filename)
		if _, err := os.Stat(targetPath); err == nil {
			if err := upsertManagedFileBlock(targetPath, block); err != nil {
				return err
			}
			wrote = true
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	if wrote {
		return nil
	}
	return upsertManagedFileBlock(filepath.Join(absCodeRoot, "AGENTS.md"), block)
}

func renderCodeRepoLinkBlock(devwikiRoot string, projectSlug string) string {
	lines := []string{
		codeLinkStartMarker,
		"## 关联 DevWiki",
		fmt.Sprintf("DevWiki project：`%s`。", projectSlug),
		fmt.Sprintf("统一查询命令使用：`--project %s`。", projectSlug),
	}
	if strings.TrimSpace(devwikiRoot) != "" {
		lines = append(lines,
			fmt.Sprintf("DevWiki 文档库根目录：`%s`。", devwikiRoot),
			"查询时以关联 DevWiki 根目录下的 `wiki/`、`raw/`、`.wikimesh/qmd.yaml` 为知识来源。",
			"`devwiki-code-to-doc` 生成的 workflow / topic 相关页面必须写入关联 DevWiki 文档库，不要写入本代码库。",
		)
	}
	lines = append(lines,
		"使用 DevWiki skills 前先判定目标产物和定位锚点：领域/功能/特性描述且缺少代码锚点时，可显式使用 `$devwiki-code` 定位代码入口和规则边界。",
		"使用 `devwiki-query` 或 `devwiki-code` 时，必须严格遵循对应 Skill.md 的查询和定位步骤；禁止绕过 skill 流程自行做全仓广泛搜索或自由发挥式检索。",
		"用户已经给出具体文件、函数、代码块、当前 diff、完整 patch 或明确替换方式时，不自动进入 `devwiki-code`，按普通编辑任务处理。",
		codeLinkEndMarker,
		"",
	)
	return strings.Join(lines, "\n")
}

func upsertManagedFileBlock(targetPath string, block string) error {
	data, err := os.ReadFile(targetPath)
	if os.IsNotExist(err) {
		return os.WriteFile(targetPath, []byte("# 仓库运行时入口\n\n"+block), 0o644)
	}
	if err != nil {
		return err
	}
	updated := upsertDelimitedBlock(string(data), block, codeLinkStartMarker, codeLinkEndMarker)
	return os.WriteFile(targetPath, []byte(updated), 0o644)
}

func upsertDelimitedBlock(content string, block string, startMarker string, endMarker string) string {
	start := strings.Index(content, startMarker)
	if start == -1 {
		content = strings.TrimRight(content, "\n")
		if content != "" {
			content += "\n\n"
		}
		return content + block
	}

	end := strings.Index(content[start:], endMarker)
	if end == -1 {
		content = strings.TrimRight(content[:start], "\n")
		if content != "" {
			content += "\n\n"
		}
		return content + block
	}

	end += start + len(endMarker)
	prefix := strings.TrimRight(content[:start], "\n")
	suffix := strings.TrimLeft(content[end:], "\n")

	parts := make([]string, 0, 3)
	if prefix != "" {
		parts = append(parts, prefix)
	}
	parts = append(parts, strings.TrimRight(block, "\n"))
	if suffix != "" {
		parts = append(parts, suffix)
	}
	return strings.Join(parts, "\n\n") + "\n"
}
