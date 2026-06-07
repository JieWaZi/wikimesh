package index

import (
	"database/sql"
	"fmt"
	"sort"
)

// ChunkEntry 是准备写入 chunk 索引的一段文本。
type ChunkEntry struct {
	ChunkID     string // ChunkID 是稳定 chunk ID，通常形如 <docID>:c<序号>。
	ChunkIndex  int    // ChunkIndex 是 chunk 在文档中的顺序。
	Heading     string // Heading 是 chunk 所属标题，便于返回结果时解释来源。
	Content     string // Content 是 chunk 正文，会写入 chunk FTS 表。
	StartOffset int    // StartOffset 是原文起始偏移，预留给高亮/定位。
	EndOffset   int    // EndOffset 是原文结束偏移，预留给高亮/定位。
}

// ChunkResult 是 chunk 级 BM25 命中。
type ChunkResult struct {
	ChunkID   string  // ChunkID 是命中的 chunk ID。
	DocID     string  // DocID 是 chunk 所属文档 ID。
	Heading   string  // Heading 是 chunk 所属标题。
	Content   string  // Content 是命中的 chunk 文本。
	BM25Score float64 // BM25Score 是文本相关性分。
	Rank      int     // Rank 是 chunk 文本检索排名。
}

// ChunkStore 管理 chunk 级 FTS5 表。
type ChunkStore struct {
	db *DB
}

// NewChunkStore 创建 chunk 索引存储封装。
func NewChunkStore(db *DB) *ChunkStore {
	return &ChunkStore{db: db}
}

// IndexChunks 在一个写事务内写入文档的所有 chunk。
// 调用方应先执行 DeleteDocChunks，确保旧 chunk 和旧向量不会残留。
func (s *ChunkStore) IndexChunks(tx *sql.Tx, docID string, chunks []ChunkEntry) error {
	for _, c := range chunks {
		if _, err := tx.Exec(
			"INSERT INTO chunks_meta (chunk_id, doc_id, chunk_index, heading, content, start_offset, end_offset) VALUES (?, ?, ?, ?, ?, ?, ?)",
			c.ChunkID, docID, c.ChunkIndex, c.Heading, c.Content, c.StartOffset, c.EndOffset,
		); err != nil {
			return fmt.Errorf("chunks.IndexChunks meta: %w", err)
		}
		if _, err := tx.Exec(
			"INSERT INTO chunks_fts (chunk_id, heading, content) VALUES (?, ?, ?)",
			c.ChunkID, c.Heading, c.Content,
		); err != nil {
			return fmt.Errorf("chunks.IndexChunks fts: %w", err)
		}
	}
	return nil
}

// DeleteDocChunks 删除一个文档的 chunk FTS、chunk 元数据和 chunk 向量。
// 这是重新索引前的清理步骤，保证删除或修改文档后不会搜到旧内容。
func (s *ChunkStore) DeleteDocChunks(tx *sql.Tx, docID string) error {
	if _, err := tx.Exec(
		"DELETE FROM chunks_fts WHERE chunk_id IN (SELECT chunk_id FROM chunks_meta WHERE doc_id = ?)", docID,
	); err != nil {
		return fmt.Errorf("chunks.DeleteDocChunks fts: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM chunks_meta WHERE doc_id = ?", docID); err != nil {
		return fmt.Errorf("chunks.DeleteDocChunks meta: %w", err)
	}
	// 向量删除需要同时清理 vec_chunks 元数据表和 sqlite-vec 虚表。
	if err := NewVectorStore(s.db).DeleteDocChunkVectorsTx(tx, docID); err != nil {
		return fmt.Errorf("chunks.DeleteDocChunks vec: %w", err)
	}
	return nil
}

// SearchChunks 在 chunk 表上执行 BM25 查询。
// chunk 粒度比整篇文档更适合问答场景，因为返回的片段更接近答案位置。
func (s *ChunkStore) SearchChunks(query string, limit int) ([]ChunkResult, error) {
	if limit <= 0 {
		limit = 20
	}

	ftsQuery := buildFTSQuery(query)
	if ftsQuery == "" {
		return nil, nil
	}

	rows, err := s.db.ReadDB().Query(`
		SELECT f.chunk_id, m.doc_id, f.heading, f.content, f.rank
		FROM chunks_fts f
		JOIN chunks_meta m ON m.chunk_id = f.chunk_id
		WHERE chunks_fts MATCH ?
		ORDER BY f.rank
		LIMIT ?
	`, ftsQuery, limit)
	if err != nil {
		return nil, fmt.Errorf("chunks.SearchChunks: %w", err)
	}
	defer rows.Close()

	var results []ChunkResult
	rank := 1
	for rows.Next() {
		var r ChunkResult
		var bm25 float64
		if err := rows.Scan(&r.ChunkID, &r.DocID, &r.Heading, &r.Content, &bm25); err != nil {
			return nil, err
		}
		r.BM25Score = -bm25
		r.Rank = rank
		rank++
		results = append(results, r)
	}
	return results, rows.Err()
}

// Count 返回已经建立索引的 chunk 数量。
func (s *ChunkStore) Count() (int, error) {
	var count int
	err := s.db.ReadDB().QueryRow("SELECT COUNT(*) FROM chunks_meta").Scan(&count)
	return count, err
}

// DocIDs 从 chunk 命中结果里提取去重后的文档 ID。
func DocIDs(results []ChunkResult) []string {
	seen := make(map[string]bool)
	var ids []string
	for _, r := range results {
		if !seen[r.DocID] {
			seen[r.DocID] = true
			ids = append(ids, r.DocID)
		}
	}
	return ids
}

// SearchChunksMultiQuery 对多个查询变体分别搜索，再用 RRF 合并。
// RRF 只依赖排名，不依赖不同查询之间的原始分数是否可比。
func (s *ChunkStore) SearchChunksMultiQuery(queries []string, limit int) ([]ChunkResult, error) {
	if len(queries) == 0 {
		return nil, nil
	}
	if len(queries) == 1 {
		return s.SearchChunks(queries[0], limit)
	}

	// 每个查询变体单独检索，保留各自排名信号。
	type scoredChunk struct {
		result ChunkResult
		rrf    float64
	}
	chunkMap := make(map[string]*scoredChunk)
	const k = 60.0

	for _, q := range queries {
		results, err := s.SearchChunks(q, limit)
		if err != nil {
			return nil, err
		}
		for _, r := range results {
			sc, ok := chunkMap[r.ChunkID]
			if !ok {
				sc = &scoredChunk{result: r}
				chunkMap[r.ChunkID] = sc
			}
			sc.rrf += 1.0 / (k + float64(r.Rank))
		}
	}

	// 把同一 chunk 的多个变体排名合并为 RRF 分。
	sorted := make([]ChunkResult, 0, len(chunkMap))
	for _, sc := range chunkMap {
		sc.result.BM25Score = sc.rrf
		sorted = append(sorted, sc.result)
	}

	// 按 RRF 分降序排列，分数越高说明多个查询变体越共同支持该 chunk。
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].BM25Score > sorted[j].BM25Score
	})

	// 排序完成后重写 Rank，保证调用方看到的是融合后的排名。
	for i := range sorted {
		sorted[i].Rank = i + 1
	}

	if len(sorted) > limit {
		sorted = sorted[:limit]
	}
	return sorted, nil
}

// NeedsBackfill 判断是否需要从已有文档级索引回填 chunk 索引。
func (s *ChunkStore) NeedsBackfill(memStore *DocumentStore) bool {
	chunkCount, err := s.Count()
	if err != nil || chunkCount > 0 {
		return false
	}
	entryCount, err := memStore.Count()
	return err == nil && entryCount > 0
}
