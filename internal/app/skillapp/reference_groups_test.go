package skillapp

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestDevwikiReferenceGroupsAreSyncable(t *testing.T) {
	t.Parallel()

	repoRoot := filepath.Clean(filepath.Join("..", "..", ".."))
	updated, err := SyncDevwikiReferenceGroups(repoRoot)
	if err != nil {
		t.Fatalf("SyncDevwikiReferenceGroups error = %v", err)
	}
	if len(updated) > 0 {
		t.Fatalf("reference groups were not already synchronized: %#v", updated)
	}
}

func TestDevwikiSharedReferencesAreComplete(t *testing.T) {
	t.Parallel()

	sharedRoot := filepath.Clean(filepath.Join("..", "..", "..", "skills", "devwiki", "share-references"))
	expected := []string{
		"code-tracing.md",
		"common-file-format.md",
		"evidence-grounding.md",
		"knowledge-placement.md",
		"mutation-safety.md",
		"devwiki.md",
	}
	entries, err := os.ReadDir(sharedRoot)
	if err != nil {
		t.Fatalf("ReadDir(share-references) error = %v", err)
	}
	var got []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		got = append(got, entry.Name())
	}
	sort.Strings(got)
	sort.Strings(expected)
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("share-references = %#v, want %#v", got, expected)
	}
}

func TestDevwikiSkillReferencesAreMinimal(t *testing.T) {
	t.Parallel()

	skillsRoot := filepath.Clean(filepath.Join("..", "..", "..", "skills", "devwiki"))
	expected := map[string][]string{
		"code": {
			"code-tracing.md",
			"devwiki.md",
		},
		"code-to-doc": {
			"code-tracing.md",
			"common-file-format.md",
			"evidence-grounding.md",
			"mutation-safety.md",
			"devwiki.md",
		},
		"ingest": {
			"code-tracing.md",
			"common-file-format.md",
			"evidence-grounding.md",
			"knowledge-placement.md",
			"mutation-safety.md",
			"devwiki.md",
		},
		"maintain": {
			"code-tracing.md",
			"common-file-format.md",
			"evidence-grounding.md",
			"mutation-safety.md",
			"devwiki.md",
		},
		"query": {
			"devwiki.md",
		},
		"topic": {
			"common-file-format.md",
			"evidence-grounding.md",
			"knowledge-placement.md",
			"mutation-safety.md",
			"topic_template.md",
			"devwiki.md",
		},
		"workflow": {
			"code-tracing.md",
			"common-file-format.md",
			"evidence-grounding.md",
			"knowledge-placement.md",
			"mutation-safety.md",
			"workflow_template.md",
			"devwiki.md",
		},
	}

	for skill, want := range expected {
		entries, err := os.ReadDir(filepath.Join(skillsRoot, skill, "references"))
		if err != nil {
			t.Fatalf("ReadDir(%s references) error = %v", skill, err)
		}
		var got []string
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			got = append(got, entry.Name())
		}
		sort.Strings(got)
		sort.Strings(want)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s references = %#v, want %#v", skill, got, want)
		}
	}
}
