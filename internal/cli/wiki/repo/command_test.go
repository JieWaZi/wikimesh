package repocmd

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JieWaZi/wikimesh/internal/app/wikiapp"
)

func TestRunWikiRepoLinkWritesCodeRepoAgentsBlock(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	wikiRoot := t.TempDir()
	codeRoot := t.TempDir()
	if err := wikiapp.SaveRepoConfig(wikiapp.RepoConfig{
		ProjectName:  "Example Project",
		ProjectSlug:  "example-project",
		Language:     "zh",
		ActiveSource: wikiapp.SourceLocal,
		Sources: wikiapp.RepoSources{
			Local: &wikiapp.RepoSource{Type: wikiapp.SourceLocal, Path: wikiRoot},
		},
	}); err != nil {
		t.Fatalf("SaveRepoConfig error = %v", err)
	}

	if err := runWikiRepoLink(io.Discard, "example-project", "main-repo", codeRoot); err != nil {
		t.Fatalf("runWikiRepoLink error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(codeRoot, "AGENTS.md"))
	if err != nil {
		t.Fatalf("ReadFile(AGENTS.md) error = %v", err)
	}
	content := string(data)
	for _, want := range []string{
		"DevWiki project：`example-project`。",
		"统一查询命令使用：`--project example-project`。",
		"DevWiki 文档库根目录：`" + wikiRoot + "`。",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("AGENTS.md missing %q:\n%s", want, content)
		}
	}
}
