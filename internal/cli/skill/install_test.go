package skillcmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JieWaZi/wikimesh/internal/cli/common"
)

func TestRunWikiSkillInstallResolvesGitHubSource(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	project := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd error = %v", err)
	}
	if err := os.Chdir(project); err != nil {
		t.Fatalf("Chdir(%s) error = %v", project, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(old); err != nil {
			t.Fatalf("restore cwd error = %v", err)
		}
	})

	sourceDir := filepath.Join(t.TempDir(), "query")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(sourceDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "SKILL.md"), []byte("---\nname: devwiki-query\ndescription: query\n---\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(SKILL.md) error = %v", err)
	}

	originalResolver := common.ResolveWikimeshSkills
	t.Cleanup(func() { common.ResolveWikimeshSkills = originalResolver })

	var gotSource common.WikiSkillSource
	common.ResolveWikimeshSkills = func(wikiType string, ref string) (common.WikiSkillBundle, error) {
		if wikiType != "devwiki" {
			t.Fatalf("resolved wikiType = %q, want devwiki", wikiType)
		}
		gotSource = common.NewWikimeshSkillsSource(wikiType, ref)
		return common.WikiSkillBundle{
			Source: gotSource,
			Skills: []common.WikiSkill{
				{Name: "devwiki-query", Description: "query", Dir: sourceDir},
			},
		}, nil
	}

	var out bytes.Buffer
	if err := runWikiSkillInstall(&out, "codex", "devwiki", ""); err != nil {
		t.Fatalf("runWikiSkillInstall error = %v", err)
	}

	if gotSource.RepoURL != "https://github.com/JieWaZi/wikimesh.git" {
		t.Fatalf("resolved RepoURL = %q, want GitHub", gotSource.RepoURL)
	}
	if gotSource.Subpath != "skills/devwiki" {
		t.Fatalf("resolved Subpath = %q, want skills/devwiki", gotSource.Subpath)
	}
	if _, err := os.Stat(filepath.Join(project, ".agents", "skills", "devwiki-query", "SKILL.md")); err != nil {
		t.Fatalf("missing installed skill: %v", err)
	}
	if !strings.Contains(out.String(), "已为 codex 安装 devwiki 的 1 个 Wikimesh skill") {
		t.Fatalf("unexpected output:\n%s", out.String())
	}
}

func TestRunWikiSkillInstallUsesExplicitVersion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	project := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd error = %v", err)
	}
	if err := os.Chdir(project); err != nil {
		t.Fatalf("Chdir(%s) error = %v", project, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(old); err != nil {
			t.Fatalf("restore cwd error = %v", err)
		}
	})

	sourceDir := filepath.Join(t.TempDir(), "query")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(sourceDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "SKILL.md"), []byte("---\nname: devwiki-query\ndescription: query\n---\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(SKILL.md) error = %v", err)
	}

	originalResolver := common.ResolveWikimeshSkills
	t.Cleanup(func() { common.ResolveWikimeshSkills = originalResolver })

	var gotRef string
	common.ResolveWikimeshSkills = func(wikiType string, ref string) (common.WikiSkillBundle, error) {
		gotRef = ref
		return common.WikiSkillBundle{
			Source: common.NewWikimeshSkillsSource(wikiType, ref),
			Skills: []common.WikiSkill{
				{Name: "devwiki-query", Description: "query", Dir: sourceDir},
			},
		}, nil
	}

	var out bytes.Buffer
	if err := runWikiSkillInstall(&out, "codex", "devwiki", "v0.2.0"); err != nil {
		t.Fatalf("runWikiSkillInstall error = %v", err)
	}
	if gotRef != "v0.2.0" {
		t.Fatalf("resolved ref = %q, want v0.2.0", gotRef)
	}
}
