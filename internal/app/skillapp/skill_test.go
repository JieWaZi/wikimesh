package skillapp

import (
	"os"
	"testing"
)

func TestNewSourceDefaultsToGitHub(t *testing.T) {
	t.Setenv("WIKIMESH_SKILLS_REF", "")

	source := NewSource("devwiki", "")

	if source.Type != "github" {
		t.Fatalf("Type = %q, want github", source.Type)
	}
	if source.RepoURL != "https://github.com/JieWaZi/wikimesh.git" {
		t.Fatalf("RepoURL = %q, want GitHub wikimesh repo", source.RepoURL)
	}
	if source.Subpath != "skills/devwiki" {
		t.Fatalf("Subpath = %q, want skills/devwiki", source.Subpath)
	}
	if source.Ref != "main" {
		t.Fatalf("Ref = %q, want main", source.Ref)
	}
	if source.Original != "JieWaZi/wikimesh/skills/devwiki#main" {
		t.Fatalf("Original = %q, want JieWaZi/wikimesh/skills/devwiki#main", source.Original)
	}
}

func TestNewSourceUsesEnvRef(t *testing.T) {
	t.Setenv("WIKIMESH_SKILLS_REF", "v0.2.0")

	source := NewSource("devwiki", "")

	if source.Ref != "v0.2.0" {
		t.Fatalf("Ref = %q, want v0.2.0", source.Ref)
	}
	if source.Original != "JieWaZi/wikimesh/skills/devwiki#v0.2.0" {
		t.Fatalf("Original = %q, want JieWaZi/wikimesh/skills/devwiki#v0.2.0", source.Original)
	}
}

func TestDiscoverFindsDevwikiSkills(t *testing.T) {
	root := t.TempDir()
	skillDir := root + "/query"
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(skillDir) error = %v", err)
	}
	if err := os.WriteFile(skillDir+"/SKILL.md", []byte("---\nname: devwiki-query\ndescription: query\n---\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(SKILL.md) error = %v", err)
	}

	found, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover error = %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("len(found) = %d, want 1: %#v", len(found), found)
	}
	if found[0].Name != "devwiki-query" {
		t.Fatalf("found[0].Name = %q, want devwiki-query", found[0].Name)
	}
}
