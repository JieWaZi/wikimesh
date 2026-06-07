package index

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"strings"
	"time"
	"unicode"
)

// Entry 是一条文档级全文索引记录。
type Entry struct {
	ID          string    // ID 是稳定文档 ID，用来关联文档表、FTS 表和向量表。
	Content     string    // Content 是进入 FTS5 的全文内容。
	Tags        []string  // Tags 这里用于记录 collection 名或文件类型等轻量标签。
	ArticlePath string    // ArticlePath 在本项目里存 collection 内的相对路径。
	CreatedAt   time.Time // CreatedAt 预留给调用方记录时间。
}

// DocumentStore 管理文档级 FTS5 表。
type DocumentStore struct {
	db *DB
}

// NewDocumentStore 创建文档级全文索引存储封装。
func NewDocumentStore(db *DB) *DocumentStore {
	return &DocumentStore{db: db}
}

// Add 写入一条文档级 FTS5 索引。
func (s *DocumentStore) Add(e Entry) error {
	tags := strings.Join(e.Tags, ",")
	return s.db.WriteTx(func(tx *sql.Tx) error {
		_, err := tx.Exec(
			"INSERT INTO entries (id, content, tags, article_path) VALUES (?, ?, ?, ?)",
			e.ID, e.Content, tags, e.ArticlePath,
		)
		return err
	})
}

// Update 替换已有索引记录的正文、标签和路径。
func (s *DocumentStore) Update(e Entry) error {
	tags := strings.Join(e.Tags, ",")
	return s.db.WriteTx(func(tx *sql.Tx) error {
		_, err := tx.Exec(
			"UPDATE entries SET content=?, tags=?, article_path=? WHERE id=?",
			e.Content, tags, e.ArticlePath, e.ID,
		)
		return err
	})
}

// Delete 按文档 ID 删除索引记录。
func (s *DocumentStore) Delete(id string) error {
	return s.db.WriteTx(func(tx *sql.Tx) error {
		_, err := tx.Exec("DELETE FROM entries WHERE id=?", id)
		return err
	})
}

// Get 按文档 ID 读取索引记录。
func (s *DocumentStore) Get(id string) (*Entry, error) {
	row := s.db.ReadDB().QueryRow(
		"SELECT id, content, tags, article_path FROM entries WHERE id=?", id,
	)
	var e Entry
	var tags string
	if err := row.Scan(&e.ID, &e.Content, &tags, &e.ArticlePath); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if tags != "" {
		e.Tags = strings.Split(tags, ",")
	}
	return &e, nil
}

// SearchResult 是 BM25 文本检索命中。
type SearchResult struct {
	ID          string   // ID 是命中的文档 ID。
	Content     string   // Content 是命中文档的索引文本。
	Tags        []string // Tags 是写入索引时的标签。
	ArticlePath string   // ArticlePath 是文档相对路径。
	BM25Score   float64  // BM25Score 越大越相关；内部会把 FTS5 的负 rank 转成正分。
	Rank        int      // Rank 是文本检索排名，从 1 开始。
}

// Search 执行 BM25 搜索。
// 关键点：用户输入先转成安全的 FTS5 query，再交给 MATCH，避免特殊字符破坏查询语法。
func (s *DocumentStore) Search(query string, tags []string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}

	// 构造 FTS5 查询：多个前缀词用 OR 连接。
	ftsQuery := buildFTSQuery(query)
	if ftsQuery == "" {
		return nil, nil
	}

	var args []any
	var tagFilter string

	if len(tags) > 0 {
		// 标签预过滤：所有标签都必须存在。
		conditions := make([]string, len(tags))
		for i, tag := range tags {
			conditions[i] = "tags LIKE ?"
			args = append(args, "%"+tag+"%")
		}
		tagFilter = " AND " + strings.Join(conditions, " AND ")
	}

	sqlQuery := fmt.Sprintf(`
		SELECT id, content, tags, article_path, rank
		FROM entries
		WHERE entries MATCH ?%s
		ORDER BY rank
		LIMIT ?
	`, tagFilter)

	args = append([]any{ftsQuery}, args...)
	args = append(args, limit)

	rows, err := s.db.ReadDB().Query(sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("index.DocumentStore.Search: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	rank := 1
	for rows.Next() {
		var r SearchResult
		var tags string
		var bm25 float64
		if err := rows.Scan(&r.ID, &r.Content, &tags, &r.ArticlePath, &bm25); err != nil {
			return nil, err
		}
		r.BM25Score = -bm25 // FTS5 rank 为负数，越小越相关。
		r.Rank = rank
		rank++
		if tags != "" {
			r.Tags = strings.Split(tags, ",")
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// Count 返回文档级索引记录总数。
func (s *DocumentStore) Count() (int, error) {
	var count int
	err := s.db.ReadDB().QueryRow("SELECT COUNT(*) FROM entries").Scan(&count)
	return count, err
}

// ContentHash 返回内容的 SHA-256 hash，用于去重。
func ContentHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", h)
}

// buildFTSQuery 把用户输入转成 FTS5 可执行的查询。
// 多个词之间用 OR，提高召回；每个词加前缀匹配，便于搜到同词根或同前缀内容。
func buildFTSQuery(query string) string {
	words := strings.Fields(strings.ToLower(query))
	var terms []string
	for _, w := range words {
		w = SanitizeFTS(w)
		if w == "" {
			continue
		}
		if !isStopword(w) {
			terms = append(terms, "\""+w+"\"*")
		}
	}
	if len(terms) == 0 {
		// 如果所有词都是停用词，仍保留这些词，避免查询被清空。
		for _, w := range words {
			w = SanitizeFTS(w)
			if w == "" {
				continue
			}
			terms = append(terms, "\""+w+"\"*")
		}
	}
	return strings.Join(terms, " OR ")
}

// SanitizeFTS 清理 FTS5 特殊字符，避免用户输入被解释成查询语法。
// 中文、日文、韩文字符会保留，否则中文查询会被清空。
func SanitizeFTS(s string) string {
	var buf strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' ||
			isCJKOrKana(r) {
			buf.WriteRune(r)
		}
	}
	return buf.String()
}

func isCJKOrKana(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		unicode.Is(unicode.Hangul, r) ||
		(r >= 0x3040 && r <= 0x309F) || // 平假名区块
		(r >= 0x30A0 && r <= 0x30FF) // 片假名区块，包含长音符。
}

var stopwords = map[string]bool{
	"a": true, "an": true, "the": true, "is": true, "are": true,
	"was": true, "were": true, "be": true, "been": true, "being": true,
	"have": true, "has": true, "had": true, "do": true, "does": true,
	"did": true, "will": true, "would": true, "could": true, "should": true,
	"may": true, "might": true, "shall": true, "can": true,
	"of": true, "in": true, "to": true, "for": true, "with": true,
	"on": true, "at": true, "by": true, "from": true, "as": true,
	"and": true, "or": true, "not": true, "but": true,
	"it": true, "its": true, "this": true, "that": true, "these": true, "those": true,
}

func isStopword(w string) bool {
	return stopwords[w]
}
