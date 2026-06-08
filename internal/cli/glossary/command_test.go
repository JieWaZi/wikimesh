package glossarycmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JieWaZi/wikimesh/internal/cli/common"
)

func TestRunWikiGlossaryKeywordsUsesProjectLocalSource(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "wiki"), 0o755); err != nil {
		t.Fatalf("MkdirAll wiki error = %v", err)
	}
	glossary := "# Glossary\n\n| glossary | type | description | slug |\n|---|---|---|---|\n| Sample Term | topic | sample | sample-topic |\n"
	if err := os.WriteFile(filepath.Join(root, "wiki/glossary.md"), []byte(glossary), 0o644); err != nil {
		t.Fatalf("WriteFile glossary error = %v", err)
	}
	if err := common.SaveWikiRepoConfig(common.WikiRepoConfig{
		ProjectName:  "Sample",
		ProjectSlug:  "sample",
		Language:     "zh",
		ActiveSource: common.WikiRepoSourceLocal,
		Sources: common.WikiRepoSources{
			Local: &common.WikiRepoSource{Type: common.WikiRepoSourceLocal, Path: root},
		},
	}); err != nil {
		t.Fatalf("SaveWikiRepoConfig error = %v", err)
	}

	var out bytes.Buffer
	if err := runWikiGlossaryKeywords(&out, "/missing-root", "sample"); err != nil {
		t.Fatalf("runWikiGlossaryKeywords error = %v", err)
	}
	if strings.TrimSpace(out.String()) != "Sample Term" {
		t.Fatalf("output = %q, want Sample Term", out.String())
	}
}
