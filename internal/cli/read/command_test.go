package readcmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JieWaZi/wikimesh/internal/cli/common"
)

func TestRunWikiReadUsesProjectLocalSource(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := newReadProjectRoot(t)
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
	if err := runWikiRead(&out, "/missing-root", "sample", "topic", "sample-topic", "card", "text"); err != nil {
		t.Fatalf("runWikiRead error = %v", err)
	}
	if !strings.Contains(out.String(), "Sample Card") {
		t.Fatalf("output = %q, want Sample Card", out.String())
	}
}

func newReadProjectRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "wiki/topics")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	page := `---
title: Sample Topic
slug: sample-topic
kind: topic
summary: sample
---
<!-- wikimesh:section id=card -->
Sample Card
<!-- /wikimesh:section -->
<!-- wikimesh:section id=core -->
Sample Core
<!-- /wikimesh:section -->
<!-- wikimesh:section id=explain -->
Sample Explain
<!-- /wikimesh:section -->
`
	if err := os.WriteFile(filepath.Join(dir, "sample-topic.md"), []byte(page), 0o644); err != nil {
		t.Fatalf("WriteFile page error = %v", err)
	}
	return root
}
