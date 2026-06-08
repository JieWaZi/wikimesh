package skillapp

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Discover 扫描指定目录下包含 SKILL.md 的 skill 目录。
func Discover(root string) ([]Skill, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var found []Skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillDir := filepath.Join(root, entry.Name())
		// 只有带 SKILL.md 的目录才是可安装 skill；普通资源目录直接跳过。
		if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		skill, err := Parse(skillDir)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(skill.Name) == "" {
			continue
		}
		found = append(found, skill)
	}
	sort.Slice(found, func(i, j int) bool { return found[i].Name < found[j].Name })
	return found, nil
}

// Parse 读取 SKILL.md frontmatter 中的名称和说明。
func Parse(dir string) (Skill, error) {
	data, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		return Skill{}, err
	}
	meta, _, ok := splitFrontmatter(data)
	if !ok {
		return Skill{}, fmt.Errorf("%s: missing YAML frontmatter", filepath.Join(dir, "SKILL.md"))
	}
	var raw struct {
		// Name 是 skill frontmatter 中声明的稳定名称。
		Name string `yaml:"name"`
		// Description 是 skill frontmatter 中声明的简短说明。
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal(meta, &raw); err != nil {
		return Skill{}, err
	}
	name := strings.TrimSpace(raw.Name)
	if name == "" {
		name = filepath.Base(dir)
	}
	return Skill{Name: name, Description: strings.TrimSpace(raw.Description), Dir: dir}, nil
}

// Install 把选中的 Wikimesh skills 安装到指定 Agent 的目标目录。
func Install(agent string, global bool, selected []Skill) error {
	targetRoot, err := TargetRoot(agent, global)
	if err != nil {
		return err
	}
	for _, skill := range selected {
		// 安装时按 skill 名称覆盖目标目录，保证重复执行结果一致。
		if err := CopyDir(skill.Dir, filepath.Join(targetRoot, skill.Name)); err != nil {
			return err
		}
	}
	return nil
}

// TargetRoot 返回指定 Agent 和安装范围对应的 skill 安装目录。
func TargetRoot(agent string, global bool) (string, error) {
	if global {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		switch agent {
		case "", "codex":
			return filepath.Join(home, ".codex", "skills"), nil
		case "cursor":
			return filepath.Join(home, ".cursor", "skills"), nil
		case "claude":
			return filepath.Join(home, ".claude", "skills"), nil
		default:
			return "", fmt.Errorf("unsupported agent %q", agent)
		}
	}
	switch agent {
	case "", "codex":
		return filepath.Abs(filepath.Join(".agents", "skills"))
	case "cursor":
		return filepath.Abs(filepath.Join(".cursor", "skills"))
	case "claude":
		return filepath.Abs(filepath.Join(".claude", "skills"))
	default:
		return "", fmt.Errorf("unsupported agent %q", agent)
	}
}

// CopyDir 递归复制目录，用于安装 Wikimesh runtime skills。
func CopyDir(src, dst string) error {
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		// 复制时保留源目录层级，目录、符号链接和普通文件分别处理。
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		return copyFile(path, target, info.Mode())
	})
}

// splitFrontmatter 拆分 SKILL.md 的 YAML frontmatter 和正文。
func splitFrontmatter(data []byte) ([]byte, []byte, bool) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if !bytes.HasPrefix(data, []byte("---\n")) {
		return nil, data, false
	}
	rest := data[len("---\n"):]
	end := bytes.Index(rest, []byte("\n---"))
	if end < 0 {
		return nil, data, false
	}
	bodyStart := end + len("\n---")
	if len(rest) > bodyStart && rest[bodyStart] == '\n' {
		bodyStart++
	}
	return rest[:end], rest[bodyStart:], true
}

// copyFile 复制单个文件并保留原文件权限。
func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
