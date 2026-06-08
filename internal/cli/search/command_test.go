package searchcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JieWaZi/wikimesh/internal/cli/common"
)

func TestRunWikiSearchUsesProjectLocalSource(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := newSearchProjectRoot(t)
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
	if err := runWikiSearch(context.Background(), &out, "/missing-root", "sample", "index", []string{"sample"}); err != nil {
		t.Fatalf("runWikiSearch error = %v", err)
	}
	if !strings.Contains(out.String(), "|topic|sample topic|sample-topic|") {
		t.Fatalf("output = %q, want project index row", out.String())
	}
}

func newSearchProjectRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "wiki"), 0o755); err != nil {
		t.Fatalf("MkdirAll wiki error = %v", err)
	}
	index := "# Wiki Index\n\n| type | description | slug |\n|---|---|---|\n| topic | sample topic | sample-topic |\n"
	if err := os.WriteFile(filepath.Join(root, "wiki/index.md"), []byte(index), 0o644); err != nil {
		t.Fatalf("WriteFile index error = %v", err)
	}
	return root
}
