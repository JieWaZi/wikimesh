package wikiapp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParsePageUsesTopLevelFrontmatterTitle(t *testing.T) {
	root := t.TempDir()
	rel := filepath.Join("wiki", "topics", "ntp-time-management.md")
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	body := `---
title: "集群时间管理"
slug: ntp-time-management
kind: topic
status: active
summary: "集群时间同步"
evidence:
  - path: "raw/designs/NTP功能梳理-1779329298916.md"
    title: "NTP功能梳理"
---

# 集群时间管理

<!-- wikimesh:section id=card -->
正文
<!-- /wikimesh:section -->
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	page, err := ParsePage(root, filepath.ToSlash(rel))
	if err != nil {
		t.Fatalf("ParsePage error = %v", err)
	}
	if page.Title != "集群时间管理" {
		t.Fatalf("Title = %q, want top-level frontmatter title", page.Title)
	}
}
