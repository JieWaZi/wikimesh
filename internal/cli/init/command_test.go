package initcmd

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JieWaZi/wikimesh/internal/cli/common"
	"github.com/JieWaZi/wikimesh/internal/ui"
	"github.com/JieWaZi/wikimesh/pkg/qmd"
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
	targetDir := filepath.Join(root, "sample")
	if _, err := os.Stat(filepath.Join(targetDir, ".agents", "skills", "devwiki-query", "SKILL.md")); err != nil {
		t.Fatalf("missing installed skill: %v", err)
	}

	runtimeData, err := os.ReadFile(filepath.Join(targetDir, "AGENTS.md"))
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

	if _, err := os.Stat(filepath.Join(targetDir, "config/project.yaml")); !os.IsNotExist(err) {
		t.Fatalf("document library should not contain config/project.yaml, stat error = %v", err)
	}
	cfg, err := common.LoadWikiRepoConfig("sample")
	if err != nil {
		t.Fatalf("LoadWikiRepoConfig error = %v", err)
	}
	if cfg.ProjectName != "Sample" || cfg.ProjectSlug != "sample" || cfg.ActiveSource != common.WikiRepoSourceLocal {
		t.Fatalf("repo config = %#v, want user-level config for sample", cfg)
	}
}

func TestRunWikiInitInstallsSkillsForMultipleAgents(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	root := t.TempDir()
	chdirInitTest(t, root)
	stubWikiSkillResolverWithSkill(t, "devwiki-query")

	err := runWikiInit(context.Background(), io.Discard, false, InitOptions{
		ProjectName: "Multi Agent",
		Agent:       "codex,cursor,claude",
		Yes:         true,
	})
	if err != nil {
		t.Fatalf("runWikiInit error = %v", err)
	}

	targetDir := filepath.Join(root, "multi-agent")
	for _, rel := range []string{
		filepath.Join(".agents", "skills", "devwiki-query", "SKILL.md"),
		filepath.Join(".cursor", "skills", "devwiki-query", "SKILL.md"),
		filepath.Join(".claude", "skills", "devwiki-query", "SKILL.md"),
	} {
		if _, err := os.Stat(filepath.Join(targetDir, rel)); err != nil {
			t.Fatalf("missing installed skill %s: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(targetDir, "CLAUDE.md")); err != nil {
		t.Fatalf("missing CLAUDE.md for claude agent: %v", err)
	}
}

func TestRunWikiInitKeepsResolvedSkillBundleUntilInstallCompletes(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	root := t.TempDir()
	chdirInitTest(t, root)

	originalResolver := common.ResolveWikimeshSkills
	t.Cleanup(func() { common.ResolveWikimeshSkills = originalResolver })

	var cleaned bool
	common.ResolveWikimeshSkills = func(wikiType string, ref string) (common.WikiSkillBundle, error) {
		sourceDir := filepath.Join(t.TempDir(), "devwiki-query")
		if err := os.MkdirAll(sourceDir, 0o755); err != nil {
			t.Fatalf("MkdirAll(sourceDir) error = %v", err)
		}
		if err := os.WriteFile(filepath.Join(sourceDir, "SKILL.md"), []byte("---\nname: devwiki-query\ndescription: query\n---\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(SKILL.md) error = %v", err)
		}
		return common.WikiSkillBundle{
			Source: common.NewWikimeshSkillsSource(wikiType, ref),
			Skills: []common.WikiSkill{{Name: "devwiki-query", Description: "query", Dir: sourceDir}},
			Cleanup: func() error {
				cleaned = true
				return os.RemoveAll(filepath.Dir(sourceDir))
			},
		}, nil
	}

	if err := runWikiInit(context.Background(), io.Discard, false, InitOptions{
		ProjectName: "Cleanup Timing",
		Agent:       "codex",
		Yes:         true,
	}); err != nil {
		t.Fatalf("runWikiInit error = %v", err)
	}
	targetDir := filepath.Join(root, "cleanup-timing")
	if _, err := os.Stat(filepath.Join(targetDir, ".agents", "skills", "devwiki-query", "SKILL.md")); err != nil {
		t.Fatalf("missing installed skill after cleanup: %v", err)
	}
	if !cleaned {
		t.Fatal("skill bundle cleanup was not called")
	}
}

func TestRunWikiInitReportsSkillFetchSource(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	root := t.TempDir()
	chdirInitTest(t, root)
	stubWikiSkillResolver(t)

	output := captureStdout(t, func() {
		if err := runWikiInit(context.Background(), io.Discard, false, InitOptions{
			ProjectName: "Fetch Source",
			Agent:       "codex",
			Yes:         true,
		}); err != nil {
			t.Fatalf("runWikiInit error = %v", err)
		}
	})

	for _, want := range []string{
		"获取 Wikimesh skills",
		"https://github.com/JieWaZi/wikimesh/tree/main/skills/devwiki",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunWikiInitCreateAllowsNoCodeRepos(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	root := t.TempDir()
	chdirInitTest(t, root)
	stubWikiSkillResolver(t)

	err := runWikiInit(context.Background(), io.Discard, false, InitOptions{
		ProjectName: "No Code Repo",
		Agent:       "codex",
		CodeDirs:    []string{},
		Yes:         true,
	})
	if err != nil {
		t.Fatalf("runWikiInit error = %v", err)
	}

	targetDir := filepath.Join(root, "no-code-repo")
	cfg, err := common.LoadWikiRepoConfig("no-code-repo")
	if err != nil {
		t.Fatalf("LoadWikiRepoConfig error = %v", err)
	}
	if len(cfg.CodeRepos) != 0 {
		t.Fatalf("CodeRepos = %#v, want empty", cfg.CodeRepos)
	}
	if cfg.ActiveSource != common.WikiRepoSourceLocal || cfg.Sources.Local == nil || !samePath(t, cfg.Sources.Local.Path, targetDir) {
		t.Fatalf("local source = %#v active=%q, want %s", cfg.Sources.Local, cfg.ActiveSource, targetDir)
	}
}

func TestRunWikiInitCreateSavesExplicitCodeRepos(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	root := t.TempDir()
	codeRoot := t.TempDir()
	chdirInitTest(t, root)
	stubWikiSkillResolver(t)

	err := runWikiInit(context.Background(), io.Discard, false, InitOptions{
		ProjectName: "Create Linked Code",
		Agent:       "codex",
		CodeRepos:   []InitCodeRepo{{Slug: "Main Repo", Path: codeRoot}},
		Yes:         true,
	})
	if err != nil {
		t.Fatalf("runWikiInit error = %v", err)
	}

	cfg, err := common.LoadWikiRepoConfig("create-linked-code")
	if err != nil {
		t.Fatalf("LoadWikiRepoConfig error = %v", err)
	}
	if len(cfg.CodeRepos) != 1 {
		t.Fatalf("CodeRepos len = %d, want 1: %#v", len(cfg.CodeRepos), cfg.CodeRepos)
	}
	if cfg.CodeRepos[0].Slug != "main-repo" || !samePath(t, cfg.CodeRepos[0].Path, codeRoot) || !cfg.CodeRepos[0].Default {
		t.Fatalf("CodeRepos[0] = %#v, want default main-repo path %s", cfg.CodeRepos[0], codeRoot)
	}
}

func TestRunWikiInitCreateUsesProjectSlugDirectory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	root := t.TempDir()
	chdirInitTest(t, root)
	stubWikiSkillResolver(t)

	err := runWikiInit(context.Background(), io.Discard, false, InitOptions{
		ProjectName: "My Project",
		Agent:       "codex",
		Yes:         true,
	})
	if err != nil {
		t.Fatalf("runWikiInit error = %v", err)
	}

	targetDir := filepath.Join(root, "my-project")
	for _, rel := range []string{"wiki", ".wikimesh", "README.md", "AGENTS.md"} {
		if _, err := os.Stat(filepath.Join(targetDir, rel)); err != nil {
			t.Fatalf("missing created path %s under project directory: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(targetDir, "config/project.yaml")); !os.IsNotExist(err) {
		t.Fatalf("document library should not contain config/project.yaml, stat error = %v", err)
	}
	for _, rel := range []string{"wiki", "config"} {
		if _, err := os.Stat(filepath.Join(root, rel)); !os.IsNotExist(err) {
			t.Fatalf("create mode should not write %s directly under cwd, stat error = %v", rel, err)
		}
	}

	cfg, err := common.LoadWikiRepoConfig("my-project")
	if err != nil {
		t.Fatalf("LoadWikiRepoConfig error = %v", err)
	}
	if cfg.Sources.Local == nil || !samePath(t, cfg.Sources.Local.Path, targetDir) {
		t.Fatalf("local source = %#v, want %s", cfg.Sources.Local, targetDir)
	}
}

func TestRunWikiInitCreateAnchorsQMDCollectionToProjectDirectory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	root := t.TempDir()
	chdirInitTest(t, root)
	stubWikiSkillResolver(t)

	err := runWikiInit(context.Background(), io.Discard, false, InitOptions{
		ProjectName: "Huawei ZDDI Wiki New",
		Agent:       "codex",
		Yes:         true,
	})
	if err != nil {
		t.Fatalf("runWikiInit error = %v", err)
	}

	targetDir := filepath.Join(root, "huawei-zddi-wiki-new")
	cfg, err := qmd.LoadConfigFile(filepath.Join(targetDir, ".wikimesh", "qmd.yaml"))
	if err != nil {
		t.Fatalf("LoadConfigFile error = %v", err)
	}
	if len(cfg.Collections) != 1 {
		t.Fatalf("collections = %#v, want one collection", cfg.Collections)
	}
	got := cfg.Collections[0].Path
	if got == filepath.Join(root, "wiki") {
		t.Fatalf("qmd collection path = %q, want project wiki directory under %s", got, targetDir)
	}
	if got == "wiki" {
		return
	}
	if _, err := os.Stat(got); err != nil {
		t.Fatalf("qmd collection path = %q is not an existing project wiki directory: %v", got, err)
	}
	if !samePath(t, got, filepath.Join(targetDir, "wiki")) {
		t.Fatalf("qmd collection path = %q, want wiki or %s", got, filepath.Join(targetDir, "wiki"))
	}
}

func TestRunWikiInitLinkLocalWikiRepo(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cwd := t.TempDir()
	wikiRoot := t.TempDir()
	codeRoot := t.TempDir()
	chdirInitTest(t, cwd)
	stubWikiSkillResolver(t)

	err := runWikiInit(context.Background(), io.Discard, false, InitOptions{
		Mode:        InitModeLink,
		ProjectName: "Linked Local",
		SourceType:  common.WikiRepoSourceLocal,
		LocalPath:   wikiRoot,
		CodeDirs:    []string{codeRoot},
		Yes:         true,
	})
	if err != nil {
		t.Fatalf("runWikiInit link local error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(cwd, "wiki")); !os.IsNotExist(err) {
		t.Fatalf("link mode should not create workspace wiki dir, stat error = %v", err)
	}
	cfg, err := common.LoadWikiRepoConfig("linked-local")
	if err != nil {
		t.Fatalf("LoadWikiRepoConfig error = %v", err)
	}
	if cfg.ActiveSource != common.WikiRepoSourceLocal || cfg.Sources.Local == nil || !samePath(t, cfg.Sources.Local.Path, wikiRoot) {
		t.Fatalf("local source = %#v active=%q, want %s", cfg.Sources.Local, cfg.ActiveSource, wikiRoot)
	}
	if len(cfg.CodeRepos) != 1 {
		t.Fatalf("CodeRepos len = %d, want 1: %#v", len(cfg.CodeRepos), cfg.CodeRepos)
	}
	if !samePath(t, cfg.CodeRepos[0].Path, codeRoot) || cfg.CodeRepos[0].Slug != filepath.Base(codeRoot) || !cfg.CodeRepos[0].Default {
		t.Fatalf("CodeRepos[0] = %#v, want linked default repo for %s", cfg.CodeRepos[0], codeRoot)
	}
}

func TestRunWikiInitLinkUsesExplicitCodeRepoSlug(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cwd := t.TempDir()
	wikiRoot := t.TempDir()
	codeRoot := t.TempDir()
	chdirInitTest(t, cwd)
	stubWikiSkillResolver(t)

	err := runWikiInit(context.Background(), io.Discard, false, InitOptions{
		Mode:        InitModeLink,
		ProjectName: "Linked Explicit",
		SourceType:  common.WikiRepoSourceLocal,
		LocalPath:   wikiRoot,
		CodeRepos:   []InitCodeRepo{{Slug: "Main Repo", Path: codeRoot}},
		Yes:         true,
	})
	if err != nil {
		t.Fatalf("runWikiInit link explicit repo error = %v", err)
	}

	cfg, err := common.LoadWikiRepoConfig("linked-explicit")
	if err != nil {
		t.Fatalf("LoadWikiRepoConfig error = %v", err)
	}
	if len(cfg.CodeRepos) != 1 {
		t.Fatalf("CodeRepos len = %d, want 1: %#v", len(cfg.CodeRepos), cfg.CodeRepos)
	}
	if cfg.CodeRepos[0].Slug != "main-repo" || !samePath(t, cfg.CodeRepos[0].Path, codeRoot) {
		t.Fatalf("CodeRepos[0] = %#v, want slug main-repo path %s", cfg.CodeRepos[0], codeRoot)
	}
}

func TestRunWikiInitLinkRemoteWikiRepo(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cwd := t.TempDir()
	chdirInitTest(t, cwd)
	stubWikiSkillResolver(t)

	err := runWikiInit(context.Background(), io.Discard, false, InitOptions{
		Mode:        InitModeLink,
		ProjectName: "Linked Remote",
		SourceType:  common.WikiRepoSourceRemote,
		RemoteURL:   "https://example.test/wikimesh",
		CodeDirs:    []string{},
		Yes:         true,
	})
	if err != nil {
		t.Fatalf("runWikiInit link remote error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(cwd, "config")); !os.IsNotExist(err) {
		t.Fatalf("link mode should not create workspace config dir, stat error = %v", err)
	}
	cfg, err := common.LoadWikiRepoConfig("linked-remote")
	if err != nil {
		t.Fatalf("LoadWikiRepoConfig error = %v", err)
	}
	if cfg.ActiveSource != common.WikiRepoSourceRemote || cfg.Sources.Remote == nil || cfg.Sources.Remote.URL != "https://example.test/wikimesh" {
		t.Fatalf("remote source = %#v active=%q", cfg.Sources.Remote, cfg.ActiveSource)
	}
	if len(cfg.CodeRepos) != 0 {
		t.Fatalf("CodeRepos = %#v, want empty", cfg.CodeRepos)
	}
}

func TestRunWikiInitLinkInstallsSkillsForSelectedAgents(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cwd := t.TempDir()
	chdirInitTest(t, cwd)
	stubWikiSkillResolver(t)
	stubWikiSkillResolverWithSkill(t, "devwiki-query")

	err := runWikiInit(context.Background(), io.Discard, false, InitOptions{
		Mode:        InitModeLink,
		ProjectName: "Linked Skills",
		Agent:       "codex,cursor",
		SourceType:  common.WikiRepoSourceRemote,
		RemoteURL:   "https://example.test/wikimesh",
		Yes:         true,
	})
	if err != nil {
		t.Fatalf("runWikiInit link skills error = %v", err)
	}

	for _, rel := range []string{
		filepath.Join(".agents", "skills", "devwiki-query", "SKILL.md"),
		filepath.Join(".cursor", "skills", "devwiki-query", "SKILL.md"),
	} {
		if _, err := os.Stat(filepath.Join(cwd, rel)); err != nil {
			t.Fatalf("missing linked skill %s: %v", rel, err)
		}
	}
}

func TestCollectInitOptionsReadsLinkModeFromNonInteractiveInput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	input := strings.NewReader("link\nLinked Remote\ncodex,cursor\nremote\nhttps://example.test/wikimesh\n\nproject\n")
	var opts InitOptions
	if err := collectInitOptions(input, io.Discard, false, &opts, false, false); err != nil {
		t.Fatalf("collectInitOptions error = %v", err)
	}
	if opts.Mode != InitModeLink {
		t.Fatalf("Mode = %q, want %q", opts.Mode, InitModeLink)
	}
	if opts.ProjectName != "Linked Remote" || opts.SourceType != common.WikiRepoSourceRemote || opts.RemoteURL != "https://example.test/wikimesh" {
		t.Fatalf("opts = %#v", opts)
	}
	if len(opts.CodeDirs) != 0 {
		t.Fatalf("CodeDirs = %#v, want empty", opts.CodeDirs)
	}
	if strings.Join(opts.Agents, ",") != "codex,cursor" {
		t.Fatalf("Agents = %#v, want codex,cursor", opts.Agents)
	}
}

func TestCollectInitOptionsCreateInteractiveUsesCodeRepoChoiceLoop(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	codeRoot := t.TempDir()
	input := strings.NewReader("main-repo\n" + codeRoot + "\n")
	var opts InitOptions
	opts.Mode = InitModeCreate
	opts.ProjectName = "Create Interactive"
	opts.Agent = "codex"
	opts.ScopeProvided = true

	choices := []string{"link", "finish"}
	originalSelectOne := wikiSelectOne
	t.Cleanup(func() { wikiSelectOne = originalSelectOne })
	wikiSelectOne = func(options ui.SelectOneOptions) (string, bool, error) {
		if options.Message == ui.Messages().PromptWikiType {
			t.Fatalf("collectInitOptions should not ask skill type before skill installation")
		}
		if len(choices) == 0 {
			t.Fatalf("unexpected SelectOne call with message %q", options.Message)
		}
		choice := choices[0]
		choices = choices[1:]
		return choice, false, nil
	}

	if err := collectInitOptions(input, io.Discard, true, &opts, true, false); err != nil {
		t.Fatalf("collectInitOptions error = %v", err)
	}
	if len(opts.CodeDirs) != 0 {
		t.Fatalf("CodeDirs = %#v, want empty because interactive create uses explicit CodeRepos", opts.CodeDirs)
	}
	if len(opts.CodeRepos) != 1 {
		t.Fatalf("CodeRepos len = %d, want 1: %#v", len(opts.CodeRepos), opts.CodeRepos)
	}
	if opts.CodeRepos[0].Slug != "main-repo" || opts.CodeRepos[0].Path != codeRoot {
		t.Fatalf("CodeRepos[0] = %#v, want slug main-repo path %s", opts.CodeRepos[0], codeRoot)
	}
	if len(choices) != 0 {
		t.Fatalf("unused choices = %#v", choices)
	}
}

func TestNewCommandSupportsNonInteractiveRemoteLinkMode(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cwd := t.TempDir()
	chdirInitTest(t, cwd)
	stubWikiSkillResolver(t)

	cmd := NewCommand()
	cmd.SetIn(strings.NewReader(""))
	cmd.SetOut(io.Discard)
	cmd.SetArgs([]string{
		"--yes",
		"--mode", InitModeLink,
		"--source", common.WikiRepoSourceRemote,
		"--remote", "https://example.test/wiki",
		"Remote CLI",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute error = %v", err)
	}

	cfg, err := common.LoadWikiRepoConfig("remote-cli")
	if err != nil {
		t.Fatalf("LoadWikiRepoConfig error = %v", err)
	}
	if cfg.ActiveSource != common.WikiRepoSourceRemote || cfg.Sources.Remote == nil || cfg.Sources.Remote.URL != "https://example.test/wiki" {
		t.Fatalf("remote source = %#v active=%q", cfg.Sources.Remote, cfg.ActiveSource)
	}
}

func TestWikiInitValidationMessagesAreChinese(t *testing.T) {
	tests := []struct {
		name string
		opts InitOptions
	}{
		{name: "bad mode", opts: InitOptions{Mode: "bad", ProjectName: "Sample"}},
		{name: "missing project", opts: InitOptions{Mode: InitModeCreate}},
		{name: "bad agent", opts: InitOptions{ProjectName: "Sample", Agent: "bad"}},
		{name: "missing local path", opts: InitOptions{Mode: InitModeLink, ProjectName: "Sample", SourceType: common.WikiRepoSourceLocal}},
		{name: "missing remote url", opts: InitOptions{Mode: InitModeLink, ProjectName: "Sample", SourceType: common.WikiRepoSourceRemote}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := normalizeWikiInitOptions(tt.opts)
			if err == nil {
				t.Fatal("normalizeWikiInitOptions error = nil, want error")
			}
			message := err.Error()
			for _, forbidden := range []string{"unsupported", "required", "source", "path", "directory", "code-dir"} {
				if strings.Contains(message, forbidden) {
					t.Fatalf("error message %q contains English token %q", message, forbidden)
				}
			}
		})
	}
}

func TestWikiInitTypeCopyDescribesSkillType(t *testing.T) {
	msg := ui.Messages()
	for name, value := range map[string]string{
		"FlagWikiType":   msg.FlagWikiType,
		"PromptWikiType": msg.PromptWikiType,
		"WikiTypeLabel":  msg.WikiTypeLabel,
	} {
		if strings.Contains(value, "Wiki 类型") {
			t.Fatalf("%s = %q, should describe skill type instead of wiki type", name, value)
		}
		if !strings.Contains(value, "Skill") && !strings.Contains(value, "skill") {
			t.Fatalf("%s = %q, want skill type wording", name, value)
		}
	}
}

func TestNewCommandLeavesWikiTypeUnsetUntilSkillSelection(t *testing.T) {
	cmd := NewCommand()
	flag := cmd.Flags().Lookup("type")
	if flag == nil {
		t.Fatal("missing --type flag")
	}
	if flag.DefValue != "" {
		t.Fatalf("--type default = %q, want empty so interactive init prompts skill type", flag.DefValue)
	}
}

func TestWikiInitSummaryCopyNamesDocumentLibraryPath(t *testing.T) {
	msg := ui.Messages()
	if msg.SourceLabel != "文档库目录" {
		t.Fatalf("SourceLabel = %q, want 文档库目录", msg.SourceLabel)
	}
	if !strings.Contains(msg.StepCreatingWikiProject, "文档库") {
		t.Fatalf("StepCreatingWikiProject = %q, want document library wording", msg.StepCreatingWikiProject)
	}
	if !strings.Contains(msg.CreatedFmt, "Wikimesh 文档库") {
		t.Fatalf("CreatedFmt = %q, want document library wording", msg.CreatedFmt)
	}
}

func TestSelectedWikiInitSkillsPromptsTypeNextToSkillSelection(t *testing.T) {
	sourceDir := filepath.Join(t.TempDir(), "query")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(sourceDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "SKILL.md"), []byte("---\nname: devwiki-query\ndescription: query\n---\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(SKILL.md) error = %v", err)
	}

	originalResolver := common.ResolveWikimeshSkills
	t.Cleanup(func() { common.ResolveWikimeshSkills = originalResolver })
	common.ResolveWikimeshSkills = func(wikiType string, ref string) (common.WikiSkillBundle, error) {
		if wikiType != common.DefaultWikiSkillType() {
			t.Fatalf("ResolveWikimeshSkills wikiType = %q, want default skill type", wikiType)
		}
		return common.WikiSkillBundle{
			Source: common.NewWikimeshSkillsSource(wikiType, ref),
			Skills: []common.WikiSkill{{Name: "devwiki-query", Description: "query", Dir: sourceDir}},
		}, nil
	}

	var calls []string
	originalSelectOne := wikiSelectOne
	originalSearchMultiselect := wikiSearchMultiselect
	t.Cleanup(func() {
		wikiSelectOne = originalSelectOne
		wikiSearchMultiselect = originalSearchMultiselect
	})
	wikiSelectOne = func(options ui.SelectOneOptions) (string, bool, error) {
		calls = append(calls, options.Message)
		return common.DefaultWikiSkillType(), false, nil
	}
	wikiSearchMultiselect = func(options ui.SearchMultiselectOptions) ([]string, bool, error) {
		calls = append(calls, options.Message)
		return []string{"devwiki-query"}, false, nil
	}

	opts := InitOptions{Agents: []string{"codex"}}
	selected, cleanup, err := selectedWikiInitSkills(&opts, true)
	if err != nil {
		t.Fatalf("selectedWikiInitSkills error = %v", err)
	}
	if cleanup != nil {
		t.Cleanup(func() { _ = cleanup() })
	}
	if len(selected) != 1 || selected[0].Name != "devwiki-query" {
		t.Fatalf("selected = %#v, want devwiki-query", selected)
	}
	wantCalls := []string{ui.Messages().PromptWikiType, ui.Messages().PromptSelectWikiSkills}
	if strings.Join(calls, "|") != strings.Join(wantCalls, "|") {
		t.Fatalf("calls = %#v, want %#v", calls, wantCalls)
	}
}

func stubWikiSkillResolver(t *testing.T) {
	t.Helper()
	originalResolver := common.ResolveWikimeshSkills
	t.Cleanup(func() { common.ResolveWikimeshSkills = originalResolver })
	common.ResolveWikimeshSkills = func(wikiType string, ref string) (common.WikiSkillBundle, error) {
		return common.WikiSkillBundle{
			Source: common.NewWikimeshSkillsSource(wikiType, ref),
			Skills: nil,
		}, nil
	}
}

func stubWikiSkillResolverWithSkill(t *testing.T, name string) {
	t.Helper()
	sourceDir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(sourceDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "SKILL.md"), []byte("---\nname: "+name+"\ndescription: test\n---\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(SKILL.md) error = %v", err)
	}
	originalResolver := common.ResolveWikimeshSkills
	t.Cleanup(func() { common.ResolveWikimeshSkills = originalResolver })
	common.ResolveWikimeshSkills = func(wikiType string, ref string) (common.WikiSkillBundle, error) {
		return common.WikiSkillBundle{
			Source: common.NewWikimeshSkillsSource(wikiType, ref),
			Skills: []common.WikiSkill{{Name: name, Description: "test", Dir: sourceDir}},
		}, nil
	}
}

func samePath(t *testing.T, got, want string) bool {
	t.Helper()
	gotEval, err := filepath.EvalSymlinks(got)
	if err != nil {
		t.Fatalf("EvalSymlinks(%s) error = %v", got, err)
	}
	wantEval, err := filepath.EvalSymlinks(want)
	if err != nil {
		t.Fatalf("EvalSymlinks(%s) error = %v", want, err)
	}
	return gotEval == wantEval
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

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stdout
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe error = %v", err)
	}
	os.Stdout = write
	defer func() { os.Stdout = old }()

	fn()

	if err := write.Close(); err != nil {
		t.Fatalf("close stdout pipe writer error = %v", err)
	}
	data, err := io.ReadAll(read)
	if err != nil {
		t.Fatalf("ReadAll stdout pipe error = %v", err)
	}
	return string(data)
}
