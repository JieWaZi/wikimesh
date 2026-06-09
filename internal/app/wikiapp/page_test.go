package wikiapp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParsePageUsesTopLevelFrontmatterTitle(t *testing.T) {
	root := t.TempDir()
	rel := filepath.Join("wiki", "topics", "user-profile-management.md")
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	body := `---
title: "集群用户管理"
slug: user-profile-management
kind: topic
status: active
summary: "用户资料维护"
evidence:
  - path: "raw/designs/用户管理设计-1779329298916.md"
    title: "用户管理设计"
---

# 集群用户管理

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
	if page.Title != "集群用户管理" {
		t.Fatalf("Title = %q, want top-level frontmatter title", page.Title)
	}
}

func TestParsePageAcceptsDevwikiSectionMarkers(t *testing.T) {
	root := t.TempDir()
	rel := filepath.Join("wiki", "topics", "access-control-management.md")
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	body := `---
title: "权限管理"
slug: access-control-management
kind: topic
status: active
summary: "权限管理"
---

# 权限管理

<!-- devwiki:section id=card -->
## 导航卡

- 主题定位：权限管理。
<!-- /devwiki:section -->
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	page, err := ParsePage(root, filepath.ToSlash(rel))
	if err != nil {
		t.Fatalf("ParsePage error = %v", err)
	}
	if section := page.Sections["card"]; !strings.Contains(section, "导航卡") {
		t.Fatalf("card section = %q, want devwiki marker content", section)
	}
}
