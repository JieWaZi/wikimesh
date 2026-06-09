package wikiinit

import (
	"context"
	"embed"
	"os"
	"path/filepath"
	"strings"

	"github.com/JieWaZi/wikimesh/internal/app/qmdapp"
	"github.com/JieWaZi/wikimesh/internal/app/wikiapp"
	"github.com/JieWaZi/wikimesh/pkg/qmd"
)

//go:embed template/docs/*
var initTemplateFS embed.FS

// createWorkspace 创建 Wikimesh 工作区骨架和 qmd 配置。
func createWorkspace(ctx context.Context, targetDir string, opts Options) error {
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
	slug := wikiapp.Slug(projectName)
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

	return initializeQMDCollection(ctx, targetDir, slug)
}

// initializeQMDCollection 初始化 qmd 配置，并通过 collection 添加链路登记 wiki 目录。
func initializeQMDCollection(ctx context.Context, targetDir string, projectSlug string) error {
	configPath := qmdapp.ConfigPathForRoot(targetDir)
	cfg := qmd.DefaultFileConfig()
	if err := qmd.SaveConfigFile(configPath, cfg); err != nil {
		return err
	}
	store, err := qmdapp.OpenStoreFromConfig(ctx, cfg)
	if err != nil {
		return err
	}
	defer store.Close()

	// collection 名称与项目 slug 保持一致，让 qmd 查询和用户级项目配置可以直接对应。
	collection := qmd.Collection{Name: projectSlug, Path: filepath.Join(targetDir, "wiki"), Pattern: "**/*.md"}
	return qmdapp.AddCollectionAndSync(ctx, cfg, configPath, store, collection)
}

// renderInitTemplate 读取内置初始化模板，并替换少量稳定占位符。
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

func primaryCodeDir(codeDirs []string) string {
	if len(codeDirs) == 0 {
		return "."
	}
	return codeDirs[0]
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
