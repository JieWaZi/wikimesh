package common

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const devwikiSkillsRelativeRoot = "skills/devwiki"

// ReferenceGroupConfig maps shared reference files to their per-skill copies.
type ReferenceGroupConfig struct {
	References map[string]ReferenceTargets `yaml:"references"`
}

// ReferenceTargets lists copy destinations for one shared reference file.
type ReferenceTargets struct {
	Targets []string `yaml:"targets"`
}

// FindWikimeshRepoRoot walks upward from start and returns the wikimesh repository root.
func FindWikimeshRepoRoot(start string) (string, error) {
	if strings.TrimSpace(start) == "" {
		start = "."
	}
	current, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(current)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		current = filepath.Dir(current)
	}

	for {
		goMod := filepath.Join(current, "go.mod")
		groups := filepath.Join(current, devwikiSkillsRelativeRoot, "reference-groups.yaml")
		if data, err := os.ReadFile(goMod); err == nil && strings.Contains(string(data), "module github.com/JieWaZi/wikimesh") {
			if _, err := os.Stat(groups); err == nil {
				return current, nil
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return "", fmt.Errorf("current directory is not inside a wikimesh repository with %s/reference-groups.yaml", devwikiSkillsRelativeRoot)
}

// LoadReferenceGroupConfig reads skills/devwiki/reference-groups.yaml from repoRoot.
func LoadReferenceGroupConfig(repoRoot string) (ReferenceGroupConfig, error) {
	var cfg ReferenceGroupConfig
	data, err := os.ReadFile(filepath.Join(repoRoot, devwikiSkillsRelativeRoot, "reference-groups.yaml"))
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	if cfg.References == nil {
		cfg.References = map[string]ReferenceTargets{}
	}
	return cfg, nil
}

// SyncDevwikiReferenceGroups copies files from share-references to configured skill reference targets.
func SyncDevwikiReferenceGroups(repoRoot string) ([]string, error) {
	cfg, err := LoadReferenceGroupConfig(repoRoot)
	if err != nil {
		return nil, err
	}
	skillsRoot := filepath.Join(repoRoot, devwikiSkillsRelativeRoot)
	var updated []string
	for _, name := range sortedReferenceNames(cfg.References) {
		sourceName := filepath.ToSlash(filepath.Clean(name))
		if sourceName == "." || strings.HasPrefix(sourceName, "../") || strings.HasPrefix(sourceName, "/") {
			return nil, fmt.Errorf("unsafe shared reference name %q", name)
		}
		sourcePath := filepath.Join(skillsRoot, "share-references", filepath.FromSlash(sourceName))
		sourceData, err := os.ReadFile(sourcePath)
		if err != nil {
			return nil, fmt.Errorf("read shared reference %s: %w", sourceName, err)
		}
		for _, target := range cfg.References[name].Targets {
			normalized := filepath.ToSlash(filepath.Clean(target))
			if normalized == "." || strings.HasPrefix(normalized, "../") || strings.HasPrefix(normalized, "/") {
				return nil, fmt.Errorf("unsafe reference target %q", target)
			}
			targetPath := filepath.Join(skillsRoot, filepath.FromSlash(normalized))
			current, err := os.ReadFile(targetPath)
			if err != nil && !os.IsNotExist(err) {
				return nil, fmt.Errorf("read reference target %s: %w", normalized, err)
			}
			if err == nil && bytes.Equal(current, sourceData) {
				continue
			}
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return nil, err
			}
			if err := os.WriteFile(targetPath, sourceData, 0o644); err != nil {
				return nil, fmt.Errorf("write reference target %s: %w", normalized, err)
			}
			updated = append(updated, normalized)
		}
	}
	sort.Strings(updated)
	return updated, nil
}

func sortedReferenceNames(refs map[string]ReferenceTargets) []string {
	names := make([]string, 0, len(refs))
	for name := range refs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
