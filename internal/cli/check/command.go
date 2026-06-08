package checkcmd

import (
	"fmt"
	"github.com/JieWaZi/wikimesh/internal/cli/common"
	"github.com/spf13/cobra"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/JieWaZi/wikimesh/internal/ui"
)

// NewCommand 构造 `wikimesh check` 命令。
func NewCommand() *cobra.Command {
	msg := ui.Messages()
	cmd := &cobra.Command{
		Use:   "check",
		Short: msg.WikiCheckShort,
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}
	cmd.AddCommand(newDocumentCommand())
	return cmd
}

func newDocumentCommand() *cobra.Command {
	msg := ui.Messages()
	var root, project string
	cmd := &cobra.Command{
		Use:   "document [path...]",
		Short: msg.WikiCheckShort,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWikiCheck(cmd.OutOrStdout(), root, project, append([]string{"document"}, args...))
		},
	}
	cmd.Flags().StringVar(&root, "root", ".", msg.FlagRoot)
	cmd.Flags().StringVar(&project, "project", "", msg.FlagProject)
	return cmd
}

// runWikiCheck 校验 WikiMesh 文档结构；当前只保留 document 检查。
func runWikiCheck(out io.Writer, root string, project string, args []string) error {
	resolvedRoot, err := common.ResolveWikiRoot(root, project)
	if err != nil {
		return err
	}
	root = resolvedRoot
	paths := args
	if len(paths) > 0 && paths[0] == "document" {
		paths = paths[1:]
	}
	if len(paths) == 0 {
		paths = []string{filepath.Join(root, "wiki")}
	}
	var files []string
	for _, path := range paths {
		abs := path
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(root, path)
		}
		info, err := os.Stat(abs)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			if filepath.Ext(abs) == ".md" {
				files = append(files, abs)
			}
			continue
		}
		if err := filepath.WalkDir(abs, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if !entry.IsDir() && filepath.Ext(path) == ".md" {
				files = append(files, path)
			}
			return nil
		}); err != nil {
			return err
		}
	}
	sort.Strings(files)
	var issues []string
	// 只检查一等知识页面；index/glossary/log 是支持文件，不要求 section。
	for _, file := range files {
		rel, _ := filepath.Rel(root, file)
		rel = filepath.ToSlash(rel)
		if rel == "wiki/index.md" || rel == "wiki/glossary.md" || rel == "wiki/log.md" {
			continue
		}
		if !strings.HasPrefix(rel, "wiki/topics/") && !strings.HasPrefix(rel, "wiki/workflows/") {
			continue
		}
		page, err := common.ParseWikiPage(root, rel)
		if err != nil {
			issues = append(issues, err.Error())
			continue
		}
		for _, section := range []string{"card", "core", "explain"} {
			if strings.TrimSpace(page.Sections[section]) == "" {
				issues = append(issues, fmt.Sprintf("%s: missing required section %q", rel, section))
			}
		}
	}
	for _, issue := range issues {
		fmt.Fprintf(out, ui.Messages().OutputWikiCheckIssueFmt, issue)
	}
	if len(issues) > 0 {
		return fmt.Errorf("document check failed with %d issue(s)", len(issues))
	}
	fmt.Fprintf(out, ui.Messages().OutputWikiCheckPassedFmt, len(files))
	return nil
}
