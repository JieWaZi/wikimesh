package wikiinit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureCodeRepoLinkWritesManagedAgentsBlock(t *testing.T) {
	codeRoot := t.TempDir()
	wikiRoot := t.TempDir()

	if err := EnsureCodeRepoLink(codeRoot, wikiRoot, "example-project"); err != nil {
		t.Fatalf("EnsureCodeRepoLink error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(codeRoot, "AGENTS.md"))
	if err != nil {
		t.Fatalf("ReadFile(AGENTS.md) error = %v", err)
	}
	content := string(data)
	for _, want := range []string{
		codeLinkStartMarker,
		codeLinkEndMarker,
		"DevWiki project：`example-project`。",
		"统一查询命令使用：`--project example-project`。",
		"DevWiki 文档库根目录：`" + wikiRoot + "`。",
		"不要写入本代码库",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("AGENTS.md missing %q:\n%s", want, content)
		}
	}
	if strings.Contains(content, "必须先阅读") {
		t.Fatalf("AGENTS.md should not force loading document-library AGENTS.md:\n%s", content)
	}
}

func TestEnsureCodeRepoLinkUpdatesExistingManagedBlock(t *testing.T) {
	codeRoot := t.TempDir()
	agentsPath := filepath.Join(codeRoot, "AGENTS.md")
	oldContent := `# Existing

<!-- wikimesh:devwiki-link:start -->
old block
<!-- wikimesh:devwiki-link:end -->

tail
`
	if err := os.WriteFile(agentsPath, []byte(oldContent), 0o644); err != nil {
		t.Fatalf("WriteFile(AGENTS.md) error = %v", err)
	}

	if err := EnsureCodeRepoLink(codeRoot, "", "new-project"); err != nil {
		t.Fatalf("EnsureCodeRepoLink error = %v", err)
	}

	data, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("ReadFile(AGENTS.md) error = %v", err)
	}
	content := string(data)
	if strings.Count(content, codeLinkStartMarker) != 1 {
		t.Fatalf("managed block count mismatch:\n%s", content)
	}
	for _, want := range []string{"# Existing", "DevWiki project：`new-project`。", "tail"} {
		if !strings.Contains(content, want) {
			t.Fatalf("AGENTS.md missing %q:\n%s", want, content)
		}
	}
	if strings.Contains(content, "old block") || strings.Contains(content, "DevWiki 文档库根目录") {
		t.Fatalf("AGENTS.md kept stale or local-only guidance:\n%s", content)
	}
}
