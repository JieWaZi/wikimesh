package daemonapp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/JieWaZi/wikimesh/internal/app/wikiapp"
)

func TestResolveCwdPrefersExplicitCwd(t *testing.T) {
	dir := t.TempDir()
	got, err := ResolveCwd(dir, "missing-project")
	if err != nil {
		t.Fatalf("ResolveCwd: %v", err)
	}
	if got != dir {
		t.Fatalf("cwd = %q, want explicit cwd", got)
	}
}

func TestResolveCwdUsesProjectLocalRoot(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	wikiRoot := filepath.Join(t.TempDir(), "wiki")
	if err := os.MkdirAll(wikiRoot, 0o755); err != nil {
		t.Fatalf("mkdir wiki root: %v", err)
	}
	cfg := wikiapp.RepoConfig{
		ProjectName:  "Demo",
		ProjectSlug:  "demo",
		Language:     "zh",
		ActiveSource: wikiapp.SourceLocal,
		Sources: wikiapp.RepoSources{
			Local: &wikiapp.RepoSource{Type: wikiapp.SourceLocal, Path: wikiRoot},
		},
	}
	if err := wikiapp.SaveRepoConfig(cfg); err != nil {
		t.Fatalf("SaveRepoConfig: %v", err)
	}
	got, err := ResolveCwd("", "demo")
	if err != nil {
		t.Fatalf("ResolveCwd: %v", err)
	}
	if got != wikiRoot {
		t.Fatalf("cwd = %q, want %q", got, wikiRoot)
	}
}

func TestResolveRunCwdOrder(t *testing.T) {
	runDir := t.TempDir()
	sessionDir := t.TempDir()
	got, err := ResolveRunCwd("", "", sessionDir, "")
	if err != nil {
		t.Fatalf("ResolveRunCwd session cwd: %v", err)
	}
	if got != sessionDir {
		t.Fatalf("cwd = %q, want session cwd %q", got, sessionDir)
	}
	got, err = ResolveRunCwd(runDir, "", sessionDir, "")
	if err != nil {
		t.Fatalf("ResolveRunCwd run cwd: %v", err)
	}
	if got != runDir {
		t.Fatalf("cwd = %q, want run cwd %q", got, runDir)
	}
}
