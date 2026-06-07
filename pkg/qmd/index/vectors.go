package index

import (
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// VectorStore 管理 SQLite 中的向量数据。
// 向量以 float32 小端序 BLOB 保存，查询时逐条读出并计算 cosine 相似度。
type VectorStore struct {
	db *DB
}

// NewVectorStore 创建向量存储封装。
func NewVectorStore(db *DB) *VectorStore {
	return &VectorStore{db: db}
}

// Upsert 写入或替换文档级向量。
func (s *VectorStore) Upsert(id string, embedding []float32) error {
	blob := encodeFloat32s(embedding)
	return s.db.WriteTx(func(tx *sql.Tx) error {
		_, err := tx.Exec(
			`INSERT INTO vec_entries (id, embedding, dimensions) VALUES (?, ?, ?)
			 ON CONFLICT(id) DO UPDATE SET embedding=excluded.embedding, dimensions=excluded.dimensions`,
			id, blob, len(embedding),
		)
		return err
	})
}

// Get 按 ID 读取文档级向量；不存在时返回 nil。
func (s *VectorStore) Get(id string) ([]float32, error) {
	var blob []byte
	err := s.db.ReadDB().QueryRow("SELECT embedding FROM vec_entries WHERE id=?", id).Scan(&blob)
	if err != nil {
		return nil, nil // 不存在或读取失败时按未命中处理。
	}
	return decodeFloat32s(blob), nil
}

// Delete 按 ID 删除文档级向量。
func (s *VectorStore) Delete(id string) error {
	return s.db.WriteTx(func(tx *sql.Tx) error {
		_, err := tx.Exec("DELETE FROM vec_entries WHERE id=?", id)
		return err
	})
}

// VectorResult 是文档级向量检索命中。
type VectorResult struct {
	ID    string  // ID 是命中文档 ID。
	Score float64 // Score 是 cosine 相似度，越大越相似。
	Rank  int     // Rank 是向量检索排名，从 1 开始。
}

// Search 执行文档级暴力 cosine 检索。
// 文档级向量是兼容旧接口的轻量路径；VSearch 的 chunk 召回优先走 sqlite-vec。
func (s *VectorStore) Search(query []float32, limit int) ([]VectorResult, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := s.db.ReadDB().Query("SELECT id, embedding, dimensions FROM vec_entries")
	if err != nil {
		return nil, fmt.Errorf("index.VectorStore.Search: %w", err)
	}
	defer rows.Close()

	var results []VectorResult
	for rows.Next() {
		var id string
		var blob []byte
		var dims int
		if err := rows.Scan(&id, &blob, &dims); err != nil {
			return nil, err
		}

		vec := decodeFloat32s(blob)
		if len(vec) != len(query) {
			continue // 维度不一致时跳过，避免不同模型的旧向量影响结果。
		}

		score := CosineSimilarity(query, vec)
		results = insertSorted(results, VectorResult{ID: id, Score: score}, limit)
	}

	// 按当前排序结果补齐排名。
	for i := range results {
		results[i].Rank = i + 1
	}

	return results, rows.Err()
}

// UpsertChunk 在已有事务中写入 chunk 向量。
// model 和 fingerprint 记录向量生成条件，便于模型或格式变化后识别过期向量。
func (s *VectorStore) UpsertChunk(tx *sql.Tx, chunkID string, docID string, embedding []float32, model string, fingerprint string) error {
	if len(embedding) == 0 {
		return nil
	}
	// 覆盖已有 chunk 前先读取旧 rowid 和维度。
	// 如果 embedding 模型切换导致维度变化，需要同步删除旧维度 vec0 分表里的残留行。
	var oldRowID int64
	var oldDims int
	oldErr := tx.QueryRow("SELECT rowid, dimensions FROM vec_chunks WHERE chunk_id = ?", chunkID).Scan(&oldRowID, &oldDims)
	if oldErr != nil && oldErr != sql.ErrNoRows {
		return oldErr
	}

	blob := encodeFloat32s(embedding)
	res, err := tx.Exec(
		`INSERT INTO vec_chunks (chunk_id, doc_id, embedding, dimensions, model, embed_fingerprint) VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(chunk_id) DO UPDATE SET
			embedding=excluded.embedding,
			dimensions=excluded.dimensions,
			model=excluded.model,
			embed_fingerprint=excluded.embed_fingerprint`,
		chunkID, docID, blob, len(embedding), model, fingerprint,
	)
	if err != nil {
		return err
	}
	rowID, err := res.LastInsertId()
	if err != nil || rowID == 0 {
		if err := tx.QueryRow("SELECT rowid FROM vec_chunks WHERE chunk_id = ?", chunkID).Scan(&rowID); err != nil {
			return err
		}
	}
	if oldErr == nil {
		rowID = oldRowID
		if oldDims > 0 && oldDims != len(embedding) {
			if err := deleteVec0Row(tx, oldDims, oldRowID); err != nil {
				return err
			}
		}
	}
	return s.upsertChunkVec0(tx, rowID, embedding)
}

// SearchChunks 对所有 chunk 向量做语义检索。
// 正常路径使用 sqlite-vec 做近邻召回；扩展不可用时回退到 Go 内暴力 cosine。
func (s *VectorStore) SearchChunks(query []float32, limit int) ([]ChunkVectorResult, error) {
	return s.SearchChunksByFingerprint(query, "", limit)
}

// SearchChunksByFingerprint 对当前 embedding 指纹下的 chunk 做 cosine 检索。
// fingerprint 为空时不过滤指纹，主要供兼容路径和测试使用。
func (s *VectorStore) SearchChunksByFingerprint(query []float32, fingerprint string, limit int) ([]ChunkVectorResult, error) {
	if limit <= 0 {
		limit = 20
	}
	if len(query) > 0 {
		if results, err := s.searchChunksSQLiteVec(query, fingerprint, limit); err == nil {
			return results, nil
		}
	}
	return s.searchChunksBruteForce(query, fingerprint, limit)
}

// searchChunksBruteForce 是 sqlite-vec 不可用时的保底路径。
// 它逐条读取 vec_chunks 的 BLOB 并计算 cosine，保证检索功能不依赖扩展可用性。
func (s *VectorStore) searchChunksBruteForce(query []float32, fingerprint string, limit int) ([]ChunkVectorResult, error) {
	sqlQuery := "SELECT chunk_id, doc_id, embedding, dimensions FROM vec_chunks"
	var args []any
	if fingerprint != "" {
		sqlQuery += " WHERE embed_fingerprint = ?"
		args = append(args, fingerprint)
	}
	rows, err := s.db.ReadDB().Query(sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("index.VectorStore.SearchChunks: %w", err)
	}
	defer rows.Close()

	var results []ChunkVectorResult
	for rows.Next() {
		var chunkID, docID string
		var blob []byte
		var dims int
		if err := rows.Scan(&chunkID, &docID, &blob, &dims); err != nil {
			return nil, err
		}
		vec := decodeFloat32s(blob)
		if len(vec) != len(query) {
			continue
		}
		score := CosineSimilarity(query, vec)
		results = insertChunkSorted(results, ChunkVectorResult{ChunkID: chunkID, DocID: docID, Score: score}, limit)
	}

	for i := range results {
		results[i].Rank = i + 1
	}
	return results, rows.Err()
}

// searchChunksSQLiteVec 使用 sqlite-vec 虚表做 chunk 近邻召回。
// sqlite-vec 返回的是 distance；对外仍重新计算 cosine，保持 VSearch Score 语义稳定。
func (s *VectorStore) searchChunksSQLiteVec(query []float32, fingerprint string, limit int) ([]ChunkVectorResult, error) {
	vecQuery, err := jsonVector(query)
	if err != nil {
		return nil, err
	}
	table := vecTableName(len(query))
	sqlQuery := fmt.Sprintf(`
SELECT c.chunk_id, c.doc_id, c.embedding, v.distance
FROM %s AS v
JOIN vec_chunks AS c ON c.rowid = v.rowid
WHERE v.embedding MATCH ?
`, table)
	args := []any{vecQuery}
	if fingerprint != "" {
		sqlQuery += " AND c.embed_fingerprint = ?"
		args = append(args, fingerprint)
	}
	sqlQuery += " ORDER BY v.distance LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.ReadDB().Query(sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []ChunkVectorResult
	for rows.Next() {
		var chunkID, docID string
		var blob []byte
		var distance float64
		if err := rows.Scan(&chunkID, &docID, &blob, &distance); err != nil {
			return nil, err
		}
		vec := decodeFloat32s(blob)
		if len(vec) != len(query) {
			continue
		}
		score := CosineSimilarity(query, vec)
		results = append(results, ChunkVectorResult{ChunkID: chunkID, DocID: docID, Score: score})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range results {
		results[i].Rank = i + 1
	}
	return results, nil
}

// DocHasFingerprint 判断一个文档是否已经拥有当前 fingerprint 的 chunk 向量。
// UpdateCollection 用它在文件 hash 未变但 embedding 配置变化时触发重建。
func (s *VectorStore) DocHasFingerprint(docID string, fingerprint string) (bool, error) {
	if fingerprint == "" {
		return true, nil
	}
	var count int
	err := s.db.ReadDB().QueryRow(
		"SELECT COUNT(*) FROM vec_chunks WHERE doc_id = ? AND embed_fingerprint = ?",
		docID, fingerprint,
	).Scan(&count)
	return count > 0, err
}

// SearchChunksFiltered 只在指定文档集合内执行 chunk 向量检索。
// 这是 BM25 预过滤后的路径，用于限制需要比较的向量数量。
func (s *VectorStore) SearchChunksFiltered(query []float32, docIDs []string, limit int) ([]ChunkVectorResult, error) {
	if limit <= 0 {
		limit = 20
	}
	if len(docIDs) == 0 {
		return nil, nil
	}

	// 限制文档数量，避免拼出过大的 IN 查询。
	if len(docIDs) > 100 {
		docIDs = docIDs[:100]
	}

	// 构造 doc_id IN (...) 查询参数。
	ph := make([]string, len(docIDs))
	args := make([]any, len(docIDs))
	for i, id := range docIDs {
		ph[i] = "?"
		args[i] = id
	}
	sqlStr := fmt.Sprintf(
		"SELECT chunk_id, doc_id, embedding, dimensions FROM vec_chunks WHERE doc_id IN (%s)",
		strings.Join(ph, ","),
	)

	rows, err := s.db.ReadDB().Query(sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("index.VectorStore.SearchChunksFiltered: %w", err)
	}
	defer rows.Close()

	var results []ChunkVectorResult
	for rows.Next() {
		var chunkID, docID string
		var blob []byte
		var dims int
		if err := rows.Scan(&chunkID, &docID, &blob, &dims); err != nil {
			return nil, err
		}
		vec := decodeFloat32s(blob)
		if len(vec) != len(query) {
			continue
		}
		score := CosineSimilarity(query, vec)
		results = insertChunkSorted(results, ChunkVectorResult{ChunkID: chunkID, DocID: docID, Score: score}, limit)
	}

	for i := range results {
		results[i].Rank = i + 1
	}
	return results, rows.Err()
}

// DeleteDocChunkVectors 删除某个文档的全部 chunk 向量。
func (s *VectorStore) DeleteDocChunkVectors(docID string) error {
	return s.db.WriteTx(func(tx *sql.Tx) error {
		return s.DeleteDocChunkVectorsTx(tx, docID)
	})
}

// DeleteDocChunkVectorsTx 删除一个文档在元数据表和所有 sqlite-vec 分表中的向量。
func (s *VectorStore) DeleteDocChunkVectorsTx(tx *sql.Tx, docID string) error {
	rows, err := tx.Query("SELECT rowid, dimensions FROM vec_chunks WHERE doc_id = ?", docID)
	if err != nil {
		return err
	}
	type rowInfo struct {
		rowID int64
		dims  int
	}
	var rowsToDelete []rowInfo
	for rows.Next() {
		var item rowInfo
		if err := rows.Scan(&item.rowID, &item.dims); err != nil {
			rows.Close()
			return err
		}
		rowsToDelete = append(rowsToDelete, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, item := range rowsToDelete {
		if item.dims <= 0 {
			continue
		}
		if err := deleteVec0Row(tx, item.dims, item.rowID); err != nil {
			return err
		}
	}
	_, err = tx.Exec("DELETE FROM vec_chunks WHERE doc_id = ?", docID)
	return err
}

// ChunkVectorResult 是 chunk 级向量检索命中。
type ChunkVectorResult struct {
	ChunkID string  // ChunkID 是命中的 chunk ID。
	DocID   string  // DocID 是 chunk 所属文档 ID。
	Score   float64 // Score 是 cosine 相似度。
	Rank    int     // Rank 是向量检索排名。
}

// insertChunkSorted 维护按分数降序排列的 top-k chunk 结果。
func insertChunkSorted(results []ChunkVectorResult, item ChunkVectorResult, limit int) []ChunkVectorResult {
	pos := len(results)
	for pos > 0 && results[pos-1].Score < item.Score {
		pos--
	}
	if pos >= limit {
		return results
	}
	if len(results) < limit {
		results = append(results, ChunkVectorResult{})
	}
	copy(results[pos+1:], results[pos:])
	results[pos] = item
	if len(results) > limit {
		results = results[:limit]
	}
	return results
}

// Count 返回文档级向量数量。
func (s *VectorStore) Count() (int, error) {
	var count int
	err := s.db.ReadDB().QueryRow("SELECT COUNT(*) FROM vec_entries").Scan(&count)
	return count, err
}

// Dimensions 返回当前文档级向量维度；没有向量时返回 0。
func (s *VectorStore) Dimensions() (int, error) {
	var dims int
	err := s.db.ReadDB().QueryRow("SELECT COALESCE(MAX(dimensions), 0) FROM vec_entries").Scan(&dims)
	return dims, err
}

// CosineSimilarity 计算两个向量的 cosine 相似度。
// 维度不同直接返回 0，避免切换 embedding 模型后旧向量导致查询崩溃。
func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		ai, bi := float64(a[i]), float64(b[i])
		dot += ai * bi
		normA += ai * ai
		normB += bi * bi
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}

// encodeFloat32s 把 float32 切片编码成小端序字节。
func encodeFloat32s(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// upsertChunkVec0 把 chunk 向量同步写入 sqlite-vec 虚表。
// vec0 表按维度拆分，避免不同 embedding 模型维度混在同一张虚表里。
func (s *VectorStore) upsertChunkVec0(tx *sql.Tx, rowID int64, embedding []float32) error {
	if err := ensureVec0Table(tx, len(embedding)); err != nil {
		return err
	}
	vecJSON, err := jsonVector(embedding)
	if err != nil {
		return err
	}
	table := vecTableName(len(embedding))
	if _, err := tx.Exec("DELETE FROM "+table+" WHERE rowid = ?", rowID); err != nil {
		return err
	}
	_, err = tx.Exec("INSERT INTO "+table+" (rowid, embedding) VALUES (?, ?)", rowID, vecJSON)
	return err
}

// deleteVec0Row 删除指定维度虚表里的单行向量。
// 删除前先确保表存在，使重复删除或旧库迁移场景可以安全执行。
func deleteVec0Row(tx *sql.Tx, dims int, rowID int64) error {
	if dims <= 0 {
		return nil
	}
	if err := ensureVec0Table(tx, dims); err != nil {
		return err
	}
	_, err := tx.Exec("DELETE FROM "+vecTableName(dims)+" WHERE rowid = ?", rowID)
	return err
}

// ensureVec0Table 按向量维度创建 sqlite-vec 虚表。
// sqlite-vec 的列类型需要固定维度，因此表名包含维度信息。
func ensureVec0Table(tx *sql.Tx, dims int) error {
	if dims <= 0 {
		return nil
	}
	_, err := tx.Exec(fmt.Sprintf(
		"CREATE VIRTUAL TABLE IF NOT EXISTS %s USING vec0(embedding float[%d])",
		vecTableName(dims),
		dims,
	))
	return err
}

// vecTableName 返回 chunk 向量对应的 sqlite-vec 虚表名。
// dims 来自 embedding 长度，只生成内部表名，不接受外部字符串拼接。
func vecTableName(dims int) string {
	return fmt.Sprintf("vec_chunks_vec_%d", dims)
}

// jsonVector 把 Go float32 向量转成 sqlite-vec MATCH/INSERT 接受的 JSON 数组。
// 元数据表仍保留 BLOB，用于精确重算 cosine 和兼容无 sqlite-vec 的回退路径。
func jsonVector(v []float32) (string, error) {
	values := make([]float64, len(v))
	for i, item := range v {
		values[i] = float64(item)
	}
	data, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// decodeFloat32s 把小端序字节还原成 float32 切片。
func decodeFloat32s(buf []byte) []float32 {
	v := make([]float32, len(buf)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(buf[i*4:]))
	}
	return v
}

// insertSorted 维护按分数降序排列的 top-k 文档级结果。
func insertSorted(results []VectorResult, item VectorResult, limit int) []VectorResult {
	pos := len(results)
	for pos > 0 && results[pos-1].Score < item.Score {
		pos--
	}

	if pos >= limit {
		return results
	}

	if len(results) < limit {
		results = append(results, VectorResult{})
	}

	copy(results[pos+1:], results[pos:])
	results[pos] = item

	if len(results) > limit {
		results = results[:limit]
	}

	return results
}
