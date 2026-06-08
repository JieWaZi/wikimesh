package wikiapp

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var wikiSectionStart = regexp.MustCompile(`^<!--\s*wikimesh:section\s+id=([A-Za-z0-9_-]+)\s*-->\s*$`)

// Page 是从 Topic/Workflow Markdown 页面解析出的轻量页面模型。
type Page struct {
	// Rel 是页面相对工作区根目录的路径。
	Rel string
	// Title 是页面 frontmatter 中的标题。
	Title string
	// Slug 是页面稳定标识，默认来自文件名。
	Slug string
	// Kind 是页面类型，例如 topic 或 workflow。
	Kind string
	// Status 是页面状态字段。
	Status string
	// Summary 是页面摘要，用于搜索召回。
	Summary string
	// Sections 保存 card/core/explain 等 section 内容。
	Sections map[string]string
	// Text 是页面正文原文，用于轻量搜索。
	Text string
}

// ListPages 扫描指定类型目录下的全部 Markdown 页面。
func ListPages(root, kind string) ([]Page, error) {
	dir := filepath.Join(root, "wiki", kind+"s")
	var pages []Page
	if err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		page, err := ParsePage(root, filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		pages = append(pages, page)
		return nil
	}); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	sort.Slice(pages, func(i, j int) bool { return pages[i].Rel < pages[j].Rel })
	return pages, nil
}

// ParsePage 解析 frontmatter 和 Wikimesh section 标记。
func ParsePage(root, rel string) (Page, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return Page{}, err
	}
	meta, body, ok := splitFrontmatter(data)
	if !ok {
		return Page{}, fmt.Errorf("%s: missing YAML frontmatter", rel)
	}
	page := Page{Rel: rel, Sections: map[string]string{}, Text: string(body)}
	// 当前只消费检索和展示需要的轻量 frontmatter 字段。
	for _, line := range strings.Split(string(meta), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		switch strings.TrimSpace(key) {
		case "title":
			page.Title = value
		case "slug":
			page.Slug = value
		case "kind":
			page.Kind = value
		case "status":
			page.Status = value
		case "summary":
			page.Summary = value
		}
	}
	if page.Slug == "" {
		page.Slug = strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
	}
	if page.Title == "" {
		page.Title = page.Slug
	}
	sections, err := parseWikiSections(body)
	if err != nil {
		return Page{}, fmt.Errorf("%s: %w", rel, err)
	}
	page.Sections = sections
	return page, nil
}

// splitFrontmatter 拆分 Markdown YAML frontmatter 和正文。
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

// parseWikiSections 解析 card/core/explain 等非嵌套 section。
func parseWikiSections(body []byte) (map[string]string, error) {
	sections := map[string]string{}
	var current string
	var buf strings.Builder
	// 逐行处理开始/结束标记，避免用正则一次性跨段匹配导致错误吞内容。
	for _, line := range strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n") {
		if match := wikiSectionStart.FindStringSubmatch(strings.TrimSpace(line)); match != nil {
			if current != "" {
				return nil, fmt.Errorf("nested section %q", match[1])
			}
			current = match[1]
			buf.Reset()
			continue
		}
		if strings.TrimSpace(line) == "<!-- /wikimesh:section -->" {
			if current == "" {
				return nil, fmt.Errorf("unexpected section end")
			}
			sections[current] = strings.TrimRight(buf.String(), "\n")
			current = ""
			buf.Reset()
			continue
		}
		if current != "" {
			buf.WriteString(line)
			buf.WriteByte('\n')
		}
	}
	if current != "" {
		return nil, fmt.Errorf("unclosed section %q", current)
	}
	return sections, nil
}

// ReadMarkdownTable 读取文件中的第一个 Markdown 表格并返回数据行。
func ReadMarkdownTable(path string) ([][]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rows [][]string
	inTable := false
	for _, line := range strings.Split(string(data), "\n") {
		cells, ok := parseMarkdownTableLine(line)
		if !ok {
			if inTable {
				break
			}
			continue
		}
		if !inTable {
			inTable = true
			continue
		}
		if isMarkdownSeparator(cells) {
			continue
		}
		rows = append(rows, cells)
	}
	return rows, nil
}

// parseMarkdownTableLine 解析一行 pipe table。
func parseMarkdownTableLine(line string) ([]string, bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "|") || !strings.HasSuffix(line, "|") {
		return nil, false
	}
	line = strings.Trim(line, "|")
	raw := strings.Split(line, "|")
	cells := make([]string, len(raw))
	for i, cell := range raw {
		cells[i] = strings.TrimSpace(strings.ReplaceAll(cell, `\|`, "|"))
	}
	return cells, true
}

// isMarkdownSeparator 判断表格行是否为 Markdown 分隔行。
func isMarkdownSeparator(cells []string) bool {
	if len(cells) == 0 {
		return false
	}
	for _, cell := range cells {
		cell = strings.Trim(cell, " :-")
		if cell != "" {
			return false
		}
	}
	return true
}

// pipeCell 转义 pipe table 单元格，保证输出仍是合法表格。
func pipeCell(value string) string {
	value = strings.ReplaceAll(value, "\r\n", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "|", "\\|")
	return strings.TrimSpace(value)
}

// Slug 把项目名或标题转为稳定的小写连字符 slug。
func Slug(text string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(text) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
