package qmdcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	qmd "github.com/JieWaZi/wikimesh/pkg/qmd"
)

func TestRunQMDUpdateSkipsCollectionUpdateCommandUnlessPull(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, ".wikimesh", "qmd.yaml")
	cfg := qmd.DefaultFileConfig()
	cfg.DBPath = filepath.Join(dir, ".wikimesh", "wiki.db")
	cfg.Collections = []qmd.Collection{{
		Name:    "docs",
		Path:    docs,
		Pattern: "**/*.md",
		Update:  `printf '# Generated\n\npull marker\n' > generated.md`,
	}}
	if err := qmd.SaveConfigFile(configPath, cfg); err != nil {
		t.Fatalf("SaveConfigFile: %v", err)
	}

	var out bytes.Buffer
	if err := runQMDUpdate(ctx, &out, &bytes.Buffer{}, configPath, qmdUpdateOptions{}); err != nil {
		t.Fatalf("runQMDUpdate without pull: %v", err)
	}
	if _, err := os.Stat(filepath.Join(docs, "generated.md")); !os.IsNotExist(err) {
		t.Fatalf("generated.md exists after update without pull, stat err=%v", err)
	}

	out.Reset()
	if err := runQMDUpdate(ctx, &out, &bytes.Buffer{}, configPath, qmdUpdateOptions{pull: true}); err != nil {
		t.Fatalf("runQMDUpdate with pull: %v", err)
	}
	if _, err := os.Stat(filepath.Join(docs, "generated.md")); err != nil {
		t.Fatalf("generated.md missing after update --pull: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "集合：docs") || !strings.Contains(output, "全部集合已更新") {
		t.Fatalf("update output = %q, want collection and completion text", output)
	}
}

func TestRunQMDStatusPrintsIndexAndCollections(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "guide.md"), []byte("# Guide\n\nstatus marker\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, ".wikimesh", "qmd.yaml")
	cfg := qmd.DefaultFileConfig()
	cfg.DBPath = filepath.Join(dir, ".wikimesh", "wiki.db")
	cfg.Collections = []qmd.Collection{{
		Name:    "docs",
		Path:    docs,
		Pattern: "**/*.md",
		Context: map[string]string{"/": "Project docs"},
	}}
	if err := qmd.SaveConfigFile(configPath, cfg); err != nil {
		t.Fatalf("SaveConfigFile: %v", err)
	}
	if err := runQMDUpdate(ctx, &bytes.Buffer{}, &bytes.Buffer{}, configPath, qmdUpdateOptions{}); err != nil {
		t.Fatalf("runQMDUpdate: %v", err)
	}

	var out bytes.Buffer
	if err := runQMDStatus(ctx, &out, configPath); err != nil {
		t.Fatalf("runQMDStatus: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"QMD 状态",
		"索引：",
		"文档：1 个文件已索引",
		"待向量化：",
		"集合：",
		"docs",
		"上下文：1",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("status output missing %q:\n%s", want, output)
		}
	}
}
