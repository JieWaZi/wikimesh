package qmd

import (
	"crypto/sha1"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

func scanCollection(c Collection) ([]collectionFile, error) {
	var files []collectionFile
	err := filepath.WalkDir(c.Path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(c.Path, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !matchesInclude(rel, c.Include) || matchesAny(rel, c.Ignore) {
			return nil
		}
		files = append(files, collectionFile{
			relPath: rel,
			absPath: path,
			info:    info,
		})
		return nil
	})
	sort.Slice(files, func(i, j int) bool {
		return files[i].relPath < files[j].relPath
	})
	return files, err
}

func matchesInclude(rel string, globs []string) bool {
	if len(globs) == 0 {
		ext := strings.ToLower(filepath.Ext(rel))
		return ext == ".md" || ext == ".markdown" || ext == ".txt" || ext == ".log"
	}
	return matchesAny(rel, globs)
}

func matchesAny(rel string, globs []string) bool {
	for _, glob := range globs {
		glob = strings.TrimSpace(filepath.ToSlash(glob))
		if glob == "" {
			continue
		}
		if glob == "**/*" {
			return true
		}
		if strings.HasPrefix(glob, "**/*.") {
			if strings.EqualFold(filepath.Ext(rel), strings.TrimPrefix(glob, "**/*")) {
				return true
			}
		}
		if ok, _ := filepath.Match(glob, rel); ok {
			return true
		}
		if strings.HasPrefix(glob, "**/") {
			if ok, _ := filepath.Match(strings.TrimPrefix(glob, "**/"), filepath.Base(rel)); ok {
				return true
			}
		}
	}
	return false
}

func splitLines(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	var out []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func encodeContext(contexts map[string]string) (string, error) {
	if len(contexts) == 0 {
		return "{}", nil
	}
	data, err := json.Marshal(contexts)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func decodeContext(raw string) map[string]string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var contexts map[string]string
	if err := json.Unmarshal([]byte(raw), &contexts); err != nil {
		return nil
	}
	return contexts
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return 10
	}
	return limit
}

func documentID(collection, relPath string) string {
	sum := sha1.Sum([]byte(collection + "\x00" + filepath.ToSlash(relPath)))
	return collection + ":" + hex.EncodeToString(sum[:])[:12]
}

func (s *Store) embeddingFingerprint() string {
	if s == nil || s.embedder == nil {
		return ""
	}
	overlapTokens := int(float64(s.chunkSize) * s.chunkOverlap)
	significant := strings.Join([]string{
		"model:" + s.embeddingModelName(),
		"query:" + s.formatQueryEmbeddingInput(embeddingFingerprintProbeQuery),
		"doc:" + s.formatDocumentEmbeddingInput(embeddingFingerprintProbeTitle, embeddingFingerprintProbeDoc),
		fmt.Sprintf("chunk_tokens:%d", s.chunkSize),
		fmt.Sprintf("chunk_overlap_tokens:%d", overlapTokens),
	}, "\n")
	sum := sha256.Sum256([]byte(significant))
	return hex.EncodeToString(sum[:])[:6]
}

func (s *Store) embeddingModelName() string {
	if s == nil || s.embedder == nil {
		return ""
	}
	return strings.TrimSpace(s.embedder.Name())
}

// EmbeddingModelName 返回当前 Store 使用的 embedding provider/model 名称。
// 未配置 embedding provider 时返回空字符串。
func (s *Store) EmbeddingModelName() string {
	return s.embeddingModelName()
}

// formatDocumentEmbeddingInput 根据 embedding 模型生成文档侧输入。
// Qwen 模型的文档侧使用原始 title/body，不加 instruct；其他模型保持 title/text 字段格式。
func (s *Store) formatDocumentEmbeddingInput(title string, text string) string {
	title = strings.TrimSpace(title)
	text = strings.TrimSpace(text)
	if isQwenEmbeddingModel(s.embeddingModelName()) {
		if title != "" {
			return title + "\n" + text
		}
		return text
	}
	if title == "" {
		title = "none"
	}
	return "title: " + title + " | text: " + text
}

// formatQueryEmbeddingInput 根据 embedding 模型生成查询侧输入。
// 默认格式使用 task/query 提示；Qwen 模型使用检索指令，保证 query/document 非对称格式一致。
func (s *Store) formatQueryEmbeddingInput(query string) string {
	query = strings.TrimSpace(query)
	if isQwenEmbeddingModel(s.embeddingModelName()) {
		return "Instruct: Retrieve relevant documents for the given query\nQuery: " + query
	}
	return "task: search result | query: " + query
}

// isQwenEmbeddingModel 判断当前模型是否需要 Qwen embedding instruct 格式。
// 只看服务方和模型名称，避免把格式选择耦合到具体向量模型实现。
func isQwenEmbeddingModel(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	qwen := strings.Index(lower, "qwen")
	embed := strings.Index(lower, "embed")
	return qwen >= 0 && embed >= 0
}

func ensureTextColumn(tx *sql.Tx, table string, column string, defaultValue string) error {
	rows, err := tx.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultVal any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultVal, &pk); err != nil {
			return err
		}
		if name == column {
			return rows.Err()
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = tx.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` TEXT NOT NULL DEFAULT ` + defaultValue)
	return err
}

func ensureIntegerColumn(tx *sql.Tx, table string, column string, defaultValue string) error {
	rows, err := tx.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultVal any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultVal, &pk); err != nil {
			return err
		}
		if name == column {
			return rows.Err()
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = tx.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` INTEGER NOT NULL DEFAULT ` + defaultValue)
	return err
}

func contentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func extractTitle(text, relPath string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	base := filepath.Base(relPath)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

func snippet(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len([]rune(s)) <= 180 {
		return s
	}
	return string([]rune(s)[:180]) + "..."
}
