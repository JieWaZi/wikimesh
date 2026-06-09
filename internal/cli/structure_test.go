package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWikiCommandsAreGroupedUnderWikiPackage(t *testing.T) {
	for _, dir := range []string{"check", "glossary", "init", "read", "repo", "search"} {
		if _, err := os.Stat(filepath.Join("..", "cli", dir)); !os.IsNotExist(err) {
			t.Fatalf("wiki command package %q should live under internal/cli/wiki", dir)
		}
	}
	for _, dir := range []string{"check", "glossary", "init", "read", "repo", "search"} {
		if info, err := os.Stat(filepath.Join("..", "cli", "wiki", dir)); err != nil || !info.IsDir() {
			t.Fatalf("missing grouped wiki command package %q under internal/cli/wiki", dir)
		}
	}
}

func TestWikiInitBusinessLogicLivesOutsideCLI(t *testing.T) {
	for _, file := range []string{"workspace.go", "repo_config.go", "gitignore.go", "qmd.go"} {
		if _, err := os.Stat(filepath.Join("wiki", "init", file)); !os.IsNotExist(err) {
			t.Fatalf("init business file %q should live outside internal/cli", file)
		}
	}
	for _, file := range []string{"service.go", "workspace.go", "repo.go"} {
		if _, err := os.Stat(filepath.Join("..", "app", "wikiinit", file)); err != nil {
			t.Fatalf("missing wiki init app file %q: %v", file, err)
		}
	}
	for _, file := range []string{"types.go", "templates.go", "qmd.go", "skills.go", "gitignore.go", "repo_config.go"} {
		if _, err := os.Stat(filepath.Join("..", "app", "wikiinit", file)); !os.IsNotExist(err) {
			t.Fatalf("wikiinit file %q is too fragmented; keep init app logic grouped by responsibility", file)
		}
	}
}

func TestAppPackagesAreGroupedByResponsibility(t *testing.T) {
	for _, file := range []string{"config.go", "page.go", "search.go"} {
		if _, err := os.Stat(filepath.Join("..", "app", "wikiapp", file)); err != nil {
			t.Fatalf("missing wikiapp responsibility file %q: %v", file, err)
		}
	}
	for _, file := range []string{"types.go", "markdown.go"} {
		if _, err := os.Stat(filepath.Join("..", "app", "wikiapp", file)); !os.IsNotExist(err) {
			t.Fatalf("wikiapp file %q is too fragmented; merge it into the responsibility file", file)
		}
	}
	for _, file := range []string{"source.go", "skill.go", "reference_groups.go"} {
		if _, err := os.Stat(filepath.Join("..", "app", "skillapp", file)); err != nil {
			t.Fatalf("missing skillapp responsibility file %q: %v", file, err)
		}
	}
	for _, file := range []string{"types.go", "discover.go", "install.go"} {
		if _, err := os.Stat(filepath.Join("..", "app", "skillapp", file)); !os.IsNotExist(err) {
			t.Fatalf("skillapp file %q is too fragmented; merge it into the responsibility file", file)
		}
	}
}

func TestWikiCommandPackagesAvoidTinyFiles(t *testing.T) {
	for _, file := range []string{"command.go", "options.go", "prompt.go"} {
		if _, err := os.Stat(filepath.Join("wiki", "init", file)); err != nil {
			t.Fatalf("missing init responsibility file %q: %v", file, err)
		}
	}
	for _, file := range []string{"skills.go"} {
		if _, err := os.Stat(filepath.Join("wiki", "init", file)); !os.IsNotExist(err) {
			t.Fatalf("init file %q is too fragmented; merge it into a responsibility file", file)
		}
	}
	if _, err := os.Stat(filepath.Join("wiki", "repo", "command.go")); err != nil {
		t.Fatalf("missing repo command file: %v", err)
	}
	for _, file := range []string{"add.go", "info.go", "link.go", "use.go"} {
		if _, err := os.Stat(filepath.Join("wiki", "repo", file)); !os.IsNotExist(err) {
			t.Fatalf("repo file %q is too fragmented; keep small repo subcommands in command.go", file)
		}
	}
}
