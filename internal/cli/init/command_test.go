package initcmd

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JieWaZi/wikimesh/internal/cli/common"
)

func TestRunWikiInitInstallsSkillsFromResolvedGitHubBundle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	root := t.TempDir()
	chdirInitTest(t, root)

	sourceDir := filepath.Join(t.TempDir(), "query")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(sourceDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "SKILL.md"), []byte("---\nname: devwiki-query\ndescription: query\n---\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(SKILL.md) error = %v", err)
	}

	originalResolver := common.ResolveWikimeshSkills
	t.Cleanup(func() { common.ResolveWikimeshSkills = originalResolver })

	var resolved bool
	common.ResolveWikimeshSkills = func(wikiType string, ref string) (common.WikiSkillBundle, error) {
		resolved = true
		if wikiType != "devwiki" {
			t.Fatalf("init should resolve devwiki skills, got type %q", wikiType)
		}
		if ref != "" {
			t.Fatalf("init should use latest Wikimesh skills without explicit ref, got %q", ref)
		}
		return common.WikiSkillBundle{
			Source: common.NewWikimeshSkillsSource(wikiType, ""),
			Skills: []common.WikiSkill{
				{Name: "devwiki-query", Description: "query", Dir: sourceDir},
			},
		}, nil
	}

	err := runWikiInit(context.Background(), io.Discard, false, InitOptions{
		ProjectName: "Sample",
		Agent:       "codex",
		CodeDirs:    []string{root},
		Yes:         true,
	})
	if err != nil {
		t.Fatalf("runWikiInit error = %v", err)
	}
	if !resolved {
		t.Fatal("runWikiInit should resolve the shared Wikimesh skill bundle")
	}
	if _, err := os.Stat(filepath.Join(root, ".agents", "skills", "devwiki-query", "SKILL.md")); err != nil {
		t.Fatalf("missing installed skill: %v", err)
	}

	runtimeData, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatalf("ReadFile(AGENTS.md) error = %v", err)
	}
	runtime := string(runtimeData)
	for _, want := range []string{"# DevWiki", "wikimesh read", "wikimesh search"} {
		if !strings.Contains(runtime, want) {
			t.Fatalf("AGENTS.md missing %q:\n%s", want, runtime)
		}
	}
	if strings.Contains(runtime, "zatools devwiki") {
		t.Fatalf("AGENTS.md should not mention zatools devwiki:\n%s", runtime)
	}

	projectData, err := os.ReadFile(filepath.Join(root, "config/project.yaml"))
	if err != nil {
		t.Fatalf("ReadFile(config/project.yaml) error = %v", err)
	}
	if !strings.Contains(string(projectData), "wiki_type: devwiki") {
		t.Fatalf("project config missing wiki_type devwiki:\n%s", string(projectData))
	}
}

func chdirInitTest(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd error = %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir(%s) error = %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(old); err != nil {
			t.Fatalf("restore cwd %s error = %v", old, err)
		}
	})
}
