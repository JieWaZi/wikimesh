package qmd

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// searchCandidateMultiplier 是 collection 查询时的 FTS 候选池倍数。
// collection 过滤发生在 FTS 候选之后，因此需要按最终 limit 放大候选池。
const searchCandidateMultiplier = 10
const searchManyRRFK = 60.0

// searchFTSRecord 是写入 documents_fts 的文档级全文索引记录。
type searchFTSRecord struct {
	// RowID 必须等于 collection_documents.rowid，用于 FTS 命中后直接回表读取文档元信息。
	RowID int64

	// FilePath 是带 collection 前缀的相对路径，路径里的关键词也会参与召回。
	FilePath string

	// Title 是文档标题；查询排序时该字段权重最高。
	Title string

	// Body 是文档正文；用于主要内容召回和摘要片段展示。
	Body string
}

// searchFTSHit 是 Search SQL 返回的一条命中记录。
type searchFTSHit struct {
	// ID 是稳定文档 ID，对外保持和 collection_documents.id 一致。
	ID string

	// Collection 是命中文档所属 collection。
	Collection string

	// Path 是 collection 内相对路径。
	Path string

	// Title 是文档标题。
	Title string

	// Body 是命中文档正文，用于构造摘要片段。
	Body string

	// BM25 是 FTS5 原始 BM25 分；数值越小越相关，通常为负数。
	BM25 float64
}

// searchDocuments 执行文档级关键词检索。
// 关键点：先取 FTS5 候选，再在 SQL 层完成 collection 和 active 过滤。
func (s *Store) searchDocuments(ctx context.Context, collection string, query string, opts SearchOptions) ([]SearchResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	limit := normalizeSearchLimit(opts.Limit)
	ftsQuery, err := buildSearchFTSQuery(query)
	if err != nil {
		return nil, err
	}
	if ftsQuery == "" {
		return nil, nil
	}

	candidateLimit := limit
	if collection != "" {
		candidateLimit = limit * searchCandidateMultiplier
	}

	// 先在 FTS5 内按 BM25 取候选，再回表过滤 active 和可选 collection。
	// 路径、标题、正文分别使用固定字段权重，排序完全由 FTS5 BM25 决定。
	sqlText := `
WITH fts_matches AS (
	SELECT rowid, bm25(documents_fts, 1.5, 4.0, 1.0) AS bm25_score
	FROM documents_fts
	WHERE documents_fts MATCH ?
	ORDER BY bm25_score ASC
	LIMIT ?
)
SELECT
	d.id,
	d.collection,
	d.rel_path,
	d.title,
	documents_fts.body,
	fts_matches.bm25_score
FROM fts_matches
JOIN collection_documents d ON d.rowid = fts_matches.rowid
JOIN documents_fts ON documents_fts.rowid = fts_matches.rowid
WHERE d.active = 1
`
	args := []any{ftsQuery, candidateLimit}
	if collection != "" {
		sqlText += `  AND d.collection = ?
`
		args = append(args, collection)
	}
	sqlText += `
ORDER BY fts_matches.bm25_score ASC
LIMIT ?
`
	args = append(args, limit)

	rows, err := s.db.ReadDB().QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, fmt.Errorf("search documents: %w", err)
	}
	defer rows.Close()

	results := make([]SearchResult, 0, limit)
	rank := 1
	for rows.Next() {
		var hit searchFTSHit
		if err := rows.Scan(&hit.ID, &hit.Collection, &hit.Path, &hit.Title, &hit.Body, &hit.BM25); err != nil {
			return nil, err
		}
		score := bm25ToScore(hit.BM25)
		if opts.MinScore > 0 && score < opts.MinScore {
			continue
		}
		results = append(results, SearchResult{
			ID:         hit.ID,
			Collection: hit.Collection,
			Path:       hit.Path,
			Title:      hit.Title,
			Snippet:    snippet(restoreSearchText(hit.Body)),
			Score:      score,
			BM25Rank:   rank,
		})
		rank++
	}
	return results, rows.Err()
}

func normalizeSearchLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	return limit
}

func normalizeSearchQueries(raw []string) []string {
	queries := make([]string, 0, len(raw))
	for _, query := range raw {
		query = strings.TrimSpace(query)
		if query == "" {
			continue
		}
		queries = append(queries, query)
	}
	return queries
}

func fuseSearchResultSets(resultSets [][]SearchResult, limit int, minScore float64) []SearchResult {
	type fusedResult struct {
		result SearchResult
		score  float64
		order  int
	}
	fused := map[string]*fusedResult{}
	order := 0
	for _, results := range resultSets {
		for rank, result := range results {
			key := result.ID
			if key == "" {
				key = result.Collection + "/" + result.Path
			}
			if key == "/" {
				continue
			}
			item, ok := fused[key]
			if !ok {
				item = &fusedResult{result: result, order: order}
				fused[key] = item
				order++
			}
			item.score += 1 / (searchManyRRFK + float64(rank+1))
		}
	}
	if len(fused) == 0 {
		return nil
	}
	items := make([]fusedResult, 0, len(fused))
	maxScore := 0.0
	for _, item := range fused {
		items = append(items, *item)
		if item.score > maxScore {
			maxScore = item.score
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].score == items[j].score {
			return items[i].order < items[j].order
		}
		return items[i].score > items[j].score
	})
	results := make([]SearchResult, 0, minInt(limit, len(items)))
	for _, item := range items {
		result := item.result
		if maxScore > 0 {
			result.Score = item.score / maxScore
		} else {
			result.Score = 0
		}
		if minScore > 0 && result.Score < minScore {
			continue
		}
		results = append(results, result)
		if len(results) >= limit {
			break
		}
	}
	return results
}

// indexSearchDocument 在同一个写事务内刷新 Search 专用 FTS 记录。
// 调用方需要先 upsert collection_documents，再传入该文档的 rowid。
func (s *Store) indexSearchDocument(tx *sql.Tx, record searchFTSRecord) error {
	if err := deleteSearchDocument(tx, record.RowID); err != nil {
		return err
	}
	_, err := tx.Exec(
		`INSERT INTO documents_fts(rowid, filepath, title, body) VALUES (?, ?, ?, ?)`,
		record.RowID,
		normalizeSearchText(record.FilePath),
		normalizeSearchText(record.Title),
		normalizeSearchText(record.Body),
	)
	if err != nil {
		return fmt.Errorf("index search document: %w", err)
	}
	return nil
}

// deleteSearchDocument 删除 Search 专用 FTS 记录。
// documents_fts 使用 collection_documents.rowid 作为 rowid，因此不能用业务 docID 删除。
func deleteSearchDocument(tx *sql.Tx, rowID int64) error {
	if rowID <= 0 {
		return nil
	}
	if _, err := tx.Exec(`DELETE FROM documents_fts WHERE rowid=?`, rowID); err != nil {
		return fmt.Errorf("delete search document: %w", err)
	}
	return nil
}

// collectionDocumentRowID 读取 collection_documents 的 SQLite rowid。
// 这个 rowid 是 documents_fts 和文档元信息之间的轻量连接键。
func collectionDocumentRowID(tx *sql.Tx, collection, relPath string) (int64, error) {
	var rowID int64
	err := tx.QueryRow(
		`SELECT rowid FROM collection_documents WHERE collection=? AND rel_path=?`,
		collection, relPath,
	).Scan(&rowID)
	if err != nil {
		return 0, fmt.Errorf("collection document rowid: %w", err)
	}
	return rowID, nil
}

// bm25ToScore 把 FTS5 原始 BM25 转成对外正向分数。
// FTS5 的 BM25 越小越好；这里映射到 [0,1)，越大表示越相关。
func bm25ToScore(bm25 float64) float64 {
	v := math.Abs(bm25)
	return v / (1 + v)
}

// buildSearchFTSQuery 把用户输入转换成 FTS5 MATCH 语法。
// 多个正向条件使用 AND，排除条件使用 NOT；只有排除条件时返回空查询。
func buildSearchFTSQuery(query string) (string, error) {
	tokens, err := scanSearchTokens(query)
	if err != nil {
		return "", err
	}
	var positive []string
	var negative []string
	for _, token := range tokens {
		parts := searchTokenClauses(token.Text, token.Quoted)
		if len(parts) == 0 {
			continue
		}
		clause := strings.Join(parts, " AND ")
		if token.Negated {
			if len(parts) > 1 {
				// 负向多条件必须整体加括号，否则 NOT 只会绑定到第一段条件。
				clause = "(" + clause + ")"
			}
			negative = append(negative, clause)
		} else {
			positive = append(positive, clause)
		}
	}
	if len(positive) == 0 {
		return "", nil
	}
	ftsQuery := strings.Join(positive, " AND ")
	for _, clause := range negative {
		// FTS5 的 NOT 是二元操作符，语法形如 `正向条件 NOT 排除条件`。
		// 不能拼成 `正向条件 AND NOT 排除条件`，否则部分 SQLite 实现会报语法错误。
		ftsQuery += " NOT " + clause
	}
	return ftsQuery, nil
}

// searchToken 是用户查询里的一个逻辑 token。
type searchToken struct {
	// Text 是 token 的原始文本，不包含外层引号和前置负号。
	Text string

	// Quoted 表示该 token 来自双引号短语，需要保留词序。
	Quoted bool

	// Negated 表示该 token 是排除条件。
	Negated bool
}

// scanSearchTokens 将原始查询切成短语、普通词和排除词。
// 这里不用 strings.Fields，因为双引号短语里允许包含空格。
func scanSearchTokens(query string) ([]searchToken, error) {
	var tokens []searchToken
	runes := []rune(strings.TrimSpace(query))
	for i := 0; i < len(runes); {
		for i < len(runes) && unicode.IsSpace(runes[i]) {
			i++
		}
		if i >= len(runes) {
			break
		}

		negated := false
		if runes[i] == '-' {
			negated = true
			i++
		}
		if i >= len(runes) {
			break
		}

		if runes[i] == '"' {
			i++
			start := i
			for i < len(runes) && runes[i] != '"' {
				i++
			}
			text := strings.TrimSpace(string(runes[start:i]))
			if i < len(runes) && runes[i] == '"' {
				i++
			}
			if text != "" {
				tokens = append(tokens, searchToken{Text: text, Quoted: true, Negated: negated})
			}
			continue
		}

		start := i
		for i < len(runes) && !unicode.IsSpace(runes[i]) && runes[i] != '"' {
			i++
		}
		text := strings.TrimSpace(string(runes[start:i]))
		if text != "" {
			tokens = append(tokens, searchToken{Text: text, Negated: negated})
		}
	}
	return tokens, nil
}

// searchTokenClauses 把一个逻辑 token 转成一个或多个 FTS5 clause。
// 连字符词会变成短语，点分隔词会拆成多个前缀条件，CJK 会变成字符短语。
func searchTokenClauses(text string, quoted bool) []string {
	text = strings.TrimSpace(strings.ToLower(text))
	if text == "" {
		return nil
	}
	if quoted {
		phrase := sanitizeSearchPhrase(text)
		if phrase == "" {
			return nil
		}
		return []string{quoteFTS(phrase)}
	}
	if isSearchHyphenatedToken(text) {
		phrase := sanitizeSearchHyphenatedTerm(text)
		if phrase == "" {
			return nil
		}
		return []string{quoteFTS(phrase)}
	}
	if isSearchDottedToken(text) {
		var clauses []string
		for _, part := range strings.Split(text, ".") {
			part = sanitizeSearchTerm(part)
			if part != "" {
				clauses = append(clauses, quoteFTS(part)+"*")
			}
		}
		return clauses
	}
	if containsCJK(text) {
		phrase := sanitizeSearchPhrase(text)
		if phrase == "" {
			return nil
		}
		return []string{quoteFTS(phrase)}
	}
	term := sanitizeSearchTerm(text)
	if term == "" {
		return nil
	}
	return []string{quoteFTS(term) + "*"}
}

// isSearchHyphenatedToken 判断 token 是否是内部连字符复合词。
// 只有连字符两侧都有字母或数字时才按短语处理，避免 `cache-` 被误判。
func isSearchHyphenatedToken(token string) bool {
	runes := []rune(token)
	if len(runes) < 3 || !isSearchWordRune(runes[0]) || !isSearchWordRune(runes[len(runes)-1]) {
		return false
	}
	hasHyphen := false
	for _, r := range runes[1 : len(runes)-1] {
		switch {
		case r == '-':
			hasHyphen = true
		case isSearchWordRune(r) || r == '\'':
		default:
			return false
		}
	}
	return hasHyphen
}

// sanitizeSearchHyphenatedTerm 把连字符复合词清理成 FTS5 短语内部文本。
func sanitizeSearchHyphenatedTerm(term string) string {
	var parts []string
	for _, part := range strings.Split(term, "-") {
		part = sanitizeSearchTerm(part)
		if part != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, " ")
}

// isSearchDottedToken 判断 token 是否是版本号一类的点分隔词。
// 只有每段都非空且只包含字母、数字或下划线时才拆成 AND 条件。
func isSearchDottedToken(token string) bool {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, r := range part {
			if !isSearchWordRune(r) && r != '_' {
				return false
			}
		}
	}
	return true
}

// normalizeSearchText 让写入索引的文本和查询侧使用同一种 CJK 分词方式。
// CJK 连续片段会在片段前后和字符之间补空格，避免中英文相邻时被 tokenizer 合成一个混合 token。
func normalizeSearchText(s string) string {
	var b strings.Builder
	inCJKRun := false
	for i, r := range s {
		if !isSearchCJK(r) {
			if inCJKRun {
				b.WriteRune(' ')
				inCJKRun = false
			}
			b.WriteRune(r)
			continue
		}
		if !inCJKRun {
			if i > 0 {
				b.WriteRune(' ')
			}
			inCJKRun = true
		} else {
			b.WriteRune(' ')
		}
		b.WriteRune(r)
	}
	if inCJKRun {
		b.WriteRune(' ')
	}
	return b.String()
}

// restoreSearchText 把索引里的 CJK 字符间隔还原成适合展示的文本。
// CJK 与拉丁字符之间保留空格，避免摘要里重新变成难读的混合文本。
func restoreSearchText(s string) string {
	runes := []rune(s)
	var b strings.Builder
	for i, r := range runes {
		if r == ' ' && i > 0 && i+1 < len(runes) && isSearchCJK(runes[i-1]) && isSearchCJK(runes[i+1]) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// containsCJK 判断查询里是否包含中文、日文或韩文字符。
func containsCJK(s string) bool {
	for _, r := range s {
		if isSearchCJK(r) {
			return true
		}
	}
	return false
}

// isSearchCJK 判断 rune 是否属于需要按字切分的 CJK 范围。
func isSearchCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		unicode.Is(unicode.Hangul, r) ||
		(r >= 0x3040 && r <= 0x309F) ||
		(r >= 0x30A0 && r <= 0x30FF)
}

// sanitizeSearchTerm 清理普通词，只保留 FTS5 tokenizer 能稳定处理的字符。
func sanitizeSearchTerm(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			b.WriteRune(r)
		case r == '\'':
			b.WriteRune(r)
		case r == '_':
			b.WriteRune(r)
		}
	}
	return b.String()
}

// sanitizeSearchPhrase 清理短语；先按 CJK 规则补空格，再按空白切词并清理标点。
func sanitizeSearchPhrase(s string) string {
	fields := strings.Fields(normalizeSearchText(strings.ToLower(s)))
	cleaned := make([]string, 0, len(fields))
	for _, field := range fields {
		field = sanitizeSearchTerm(field)
		if field != "" {
			cleaned = append(cleaned, field)
		}
	}
	return strings.Join(cleaned, " ")
}

// isSearchWordRune 判断 rune 是否可作为连字符或点分隔 token 的主体字符。
func isSearchWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r)
}

// quoteFTS 给 FTS5 token 或 phrase 加双引号，并转义内部双引号。
func quoteFTS(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// searchFilePath 生成进入 filepath 字段的路径文本。
// collection 前缀可以让同名文件在不同 collection 下仍具备可解释的路径召回。
func searchFilePath(collection, relPath string) string {
	return collection + "/" + filepath.ToSlash(relPath)
}
