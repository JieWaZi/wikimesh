package skillcmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JieWaZi/wikimesh/internal/app/skillapp"
	"github.com/JieWaZi/wikimesh/internal/ui"
)

func TestRunSkillInstallResolvesGitHubSource(t *testing.T) {
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

	originalResolver := skillapp.ResolveWikimeshSkills
	t.Cleanup(func() { skillapp.ResolveWikimeshSkills = originalResolver })

	var gotSource skillapp.Source
	skillapp.ResolveWikimeshSkills = func(wikiType string, ref string) (skillapp.Bundle, error) {
		if wikiType != "devwiki" {
			t.Fatalf("resolved wikiType = %q, want devwiki", wikiType)
		}
		gotSource = skillapp.NewSource(wikiType, ref)
		return skillapp.Bundle{
			Source: gotSource,
			Skills: []skillapp.Skill{
				{Name: "devwiki-query", Description: "query", Dir: sourceDir},
			},
		}, nil
	}

	var out bytes.Buffer
	if err := runWikiSkillInstall(&out, false, InstallOptions{Agent: "codex", WikiType: "devwiki"}, ""); err != nil {
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

func TestRunSkillInstallUsesExplicitVersion(t *testing.T) {
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

	originalResolver := skillapp.ResolveWikimeshSkills
	t.Cleanup(func() { skillapp.ResolveWikimeshSkills = originalResolver })

	var gotRef string
	skillapp.ResolveWikimeshSkills = func(wikiType string, ref string) (skillapp.Bundle, error) {
		gotRef = ref
		return skillapp.Bundle{
			Source: skillapp.NewSource(wikiType, ref),
			Skills: []skillapp.Skill{
				{Name: "devwiki-query", Description: "query", Dir: sourceDir},
			},
		}, nil
	}

	var out bytes.Buffer
	if err := runWikiSkillInstall(&out, false, InstallOptions{Agent: "codex", WikiType: "devwiki"}, "v0.2.0"); err != nil {
		t.Fatalf("runWikiSkillInstall error = %v", err)
	}
	if gotRef != "v0.2.0" {
		t.Fatalf("resolved ref = %q, want v0.2.0", gotRef)
	}
}

func TestRunSkillInstallWithExplicitFlagsDoesNotPromptAgain(t *testing.T) {
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

	queryDir := writeTestSkill(t, "devwiki-query")
	originalResolver := skillapp.ResolveWikimeshSkills
	t.Cleanup(func() { skillapp.ResolveWikimeshSkills = originalResolver })
	skillapp.ResolveWikimeshSkills = func(wikiType string, ref string) (skillapp.Bundle, error) {
		return skillapp.Bundle{
			Source: skillapp.NewSource(wikiType, ref),
			Skills: []skillapp.Skill{{Name: "devwiki-query", Description: "query", Dir: queryDir}},
		}, nil
	}

	originalSelectOne := skillSelectOne
	originalSearchMultiselect := skillSearchMultiselect
	t.Cleanup(func() {
		skillSelectOne = originalSelectOne
		skillSearchMultiselect = originalSearchMultiselect
	})
	skillSelectOne = func(options ui.SelectOneOptions) (string, bool, error) {
		t.Fatalf("unexpected SelectOne prompt %q", options.Message)
		return "", false, nil
	}
	skillSearchMultiselect = func(options ui.SearchMultiselectOptions) ([]string, bool, error) {
		if options.Message == ui.Messages().PromptSelectWikiSkills {
			return []string{"devwiki-query"}, false, nil
		}
		t.Fatalf("unexpected SearchMultiselect prompt %q", options.Message)
		return nil, false, nil
	}

	opts := InstallOptions{
		Agent:            "codex,cursor",
		WikiType:         "devwiki",
		AgentProvided:    true,
		WikiTypeProvided: true,
	}
	if err := runWikiSkillInstall(io.Discard, true, opts, ""); err != nil {
		t.Fatalf("runWikiSkillInstall error = %v", err)
	}
}

func TestRunSkillInstallInstallsSelectedSkillsForMultipleAgents(t *testing.T) {
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

	queryDir := writeTestSkill(t, "devwiki-query")
	codeDir := writeTestSkill(t, "devwiki-code")
	originalResolver := skillapp.ResolveWikimeshSkills
	t.Cleanup(func() { skillapp.ResolveWikimeshSkills = originalResolver })
	skillapp.ResolveWikimeshSkills = func(wikiType string, ref string) (skillapp.Bundle, error) {
		return skillapp.Bundle{
			Source: skillapp.NewSource(wikiType, ref),
			Skills: []skillapp.Skill{
				{Name: "devwiki-query", Description: "query", Dir: queryDir},
				{Name: "devwiki-code", Description: "code", Dir: codeDir},
			},
		}, nil
	}

	var out bytes.Buffer
	var calls []string
	originalSelectOne := skillSelectOne
	originalSearchMultiselect := skillSearchMultiselect
	t.Cleanup(func() {
		skillSelectOne = originalSelectOne
		skillSearchMultiselect = originalSearchMultiselect
	})
	skillSelectOne = func(options ui.SelectOneOptions) (string, bool, error) {
		calls = append(calls, options.Message)
		return "devwiki", false, nil
	}
	skillSearchMultiselect = func(options ui.SearchMultiselectOptions) ([]string, bool, error) {
		calls = append(calls, options.Message)
		switch options.Message {
		case ui.Messages().PromptWikiAgent:
			return []string{"codex", "cursor"}, false, nil
		case ui.Messages().PromptSelectWikiSkills:
			return []string{"devwiki-query"}, false, nil
		default:
			t.Fatalf("unexpected multiselect prompt %q", options.Message)
			return nil, false, nil
		}
	}
	opts := InstallOptions{
		Yes: false,
	}
	if err := runWikiSkillInstall(&out, true, opts, ""); err != nil {
		t.Fatalf("runWikiSkillInstall error = %v", err)
	}

	for _, rel := range []string{
		filepath.Join(".agents", "skills", "devwiki-query", "SKILL.md"),
		filepath.Join(".cursor", "skills", "devwiki-query", "SKILL.md"),
	} {
		if _, err := os.Stat(filepath.Join(project, rel)); err != nil {
			t.Fatalf("missing installed skill %s: %v", rel, err)
		}
	}
	for _, rel := range []string{
		filepath.Join(".agents", "skills", "devwiki-code", "SKILL.md"),
		filepath.Join(".cursor", "skills", "devwiki-code", "SKILL.md"),
	} {
		if _, err := os.Stat(filepath.Join(project, rel)); err == nil {
			t.Fatalf("unexpected installed skill %s", rel)
		}
	}
	if !strings.Contains(out.String(), "已为 codex,cursor 安装 devwiki 的 1 个 Wikimesh skill") {
		t.Fatalf("unexpected output:\n%s", out.String())
	}
	wantCalls := []string{
		ui.Messages().PromptWikiAgent,
		ui.Messages().PromptWikiType,
		ui.Messages().PromptSelectWikiSkills,
	}
	if strings.Join(calls, "|") != strings.Join(wantCalls, "|") {
		t.Fatalf("calls = %#v, want %#v", calls, wantCalls)
	}
}

func writeTestSkill(t *testing.T, name string) string {
	t.Helper()
	sourceDir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", sourceDir, err)
	}
	content := []byte("---\nname: " + name + "\ndescription: test\n---\n")
	if err := os.WriteFile(filepath.Join(sourceDir, "SKILL.md"), content, 0o644); err != nil {
		t.Fatalf("WriteFile(SKILL.md) error = %v", err)
	}
	return sourceDir
}
