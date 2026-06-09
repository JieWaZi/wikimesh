package qmd

import (
	"context"
	"database/sql"
	"errors"
	"sort"

	"github.com/JieWaZi/wikimesh/pkg/qmd/index"
)

// Search 执行文档级 BM25 检索。
// 查询流程固定为：构造 FTS5 查询 -> 先从 documents_fts 取候选 -> 回表做 collection/active 过滤。
// collection 过滤必须在 SQL 中完成，避免先拿全局少量结果后再过滤导致目标 collection 召回丢失。
func (s *Store) Search(ctx context.Context, collection string, query string, opts SearchOptions) ([]SearchResult, error) {
	return s.searchDocuments(ctx, collection, query, opts)
}

// SearchMany 对多个普通 query 分别执行 Search，并用等权 RRF 融合结果。
func (s *Store) SearchMany(ctx context.Context, collection string, queries []string, opts SearchOptions) ([]SearchResult, error) {
	queries = normalizeSearchQueries(queries)
	if len(queries) == 0 {
		return nil, nil
	}
	if len(queries) == 1 {
		return s.Search(ctx, collection, queries[0], opts)
	}
	limit := normalizeSearchLimit(opts.Limit)
	searchOpts := opts
	searchOpts.Limit = limit
	resultSets := make([][]SearchResult, 0, len(queries))
	for _, query := range queries {
		results, err := s.Search(ctx, collection, query, searchOpts)
		if err != nil {
			return nil, err
		}
		resultSets = append(resultSets, results)
	}
	return fuseSearchResultSets(resultSets, limit, opts.MinScore), nil
}

// SearchLex 执行 qmd SDK searchLex 等价的 BM25 关键词检索。
// 它不调用 LLM、QueryExpander 或 reranker，只走 documents_fts。
func (s *Store) SearchLex(ctx context.Context, query string, opts LexSearchOptions) ([]SearchResult, error) {
	return s.Search(ctx, opts.Collection, query, SearchOptions{Limit: opts.Limit})
}

// SearchVector 执行 qmd SDK searchVector 等价的原始向量检索。
// 该方法只 embedding 原始 query，不消费 QueryExpander；需要 query expansion 时应使用 VSearch 或 Query。
func (s *Store) SearchVector(ctx context.Context, query string, opts VectorSearchOptions) ([]SearchResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.embedder == nil {
		return nil, errors.New("embedding provider is not configured")
	}
	queryVec, err := s.embedder.Embed(s.formatQueryEmbeddingInput(query))
	if err != nil {
		return nil, err
	}
	limit := normalizeSearchLimit(opts.Limit)
	raw, err := s.searchVectorVariants(opts.Collection, [][]float32{queryVec}, limit)
	if err != nil {
		return nil, err
	}
	minScore := opts.MinScore
	if minScore == 0 {
		minScore = -2
	}
	return s.vectorResults(opts.Collection, raw, minScore, limit)
}

// ExpandQuery 执行 qmd SDK expandQuery 等价的手动查询扩展。
// 支持 QueryExpanderWithOptions 的实现会收到 Intent 等扩展参数。
func (s *Store) ExpandQuery(ctx context.Context, query string, opts ExpandQueryOptions) ([]QueryExpansion, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.queryExpander == nil {
		return nil, nil
	}
	expanded, err := expandQueryWithOptions(ctx, s.queryExpander, query, opts)
	if err != nil {
		return nil, err
	}
	for i := range expanded {
		expanded[i] = normalizeQueryExpansion(expanded[i])
	}
	return expanded, nil
}

// VSearch 执行 chunk 级向量检索。
// 向量命中先按 chunk 排序，再按文档去重，保留每个文档最相关的 chunk。
func (s *Store) VSearch(ctx context.Context, collection string, query string, opts SearchOptions) ([]SearchResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.embedder == nil {
		return nil, errors.New("embedding provider is not configured")
	}
	queryInputs, err := s.vectorQueryInputs(ctx, query, opts.Intent)
	if err != nil {
		return nil, err
	}
	queryVecs := make([][]float32, 0, len(queryInputs))
	for _, input := range queryInputs {
		// 查询侧统一加任务前缀，让 embedding 模型更明确当前文本用于检索。
		queryVec, err := s.embedder.Embed(input)
		if err != nil {
			return nil, err
		}
		queryVecs = append(queryVecs, queryVec)
	}
	limit := normalizeLimit(opts.Limit)
	raw, err := s.searchVectorVariants(collection, queryVecs, limit)
	if err != nil {
		return nil, err
	}
	minScore := opts.MinScore
	if minScore == 0 {
		// 默认过滤弱语义命中，避免低相似度 chunk 干扰关键词检索结果。
		minScore = defaultVSearchMinScore
	}
	return s.vectorResults(collection, raw, minScore, limit)
}

// Query 执行 qmd 风格混合查询。
// 具体 pipeline 放在 query_index.go：typed query expansion、FTS/向量分路、RRF、chunk 选择和可选 rerank。
func (s *Store) Query(ctx context.Context, collection string, question string, opts QueryOptions) (*QueryResult, error) {
	return s.queryQMD(ctx, collection, question, opts)
}

func (s *Store) getDocByCollectionPath(collection, relPath string) (*docMeta, error) {
	row := s.db.ReadDB().QueryRow(`
SELECT id, collection, rel_path, abs_path, title, hash, embedding_model
FROM collection_documents
WHERE collection=? AND rel_path=?
`, collection, relPath)
	return scanDoc(row)
}

func (s *Store) getActiveDocByID(id string) (*docMeta, error) {
	row := s.db.ReadDB().QueryRow(`
SELECT id, collection, rel_path, abs_path, title, hash, embedding_model
FROM collection_documents
WHERE id=? AND active=1
`, id)
	return scanDoc(row)
}

func scanDoc(row *sql.Row) (*docMeta, error) {
	var m docMeta
	err := row.Scan(&m.id, &m.collection, &m.relPath, &m.absPath, &m.title, &m.hash, &m.embeddingModel)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *Store) vectorQueryInputs(ctx context.Context, query string, intent string) ([]string, error) {
	inputs := []string{s.formatQueryEmbeddingInput(query)}
	if s.queryExpander == nil {
		return inputs, nil
	}
	expansions, err := expandQueryWithOptions(ctx, s.queryExpander, query, ExpandQueryOptions{Intent: intent})
	if err != nil {
		return nil, err
	}
	for _, item := range expansions {
		// 向量检索只消费语义扩展和假想答案扩展；关键词扩展留给文本检索使用。
		if item.Type != QueryExpansionVec && item.Type != QueryExpansionHyDE {
			continue
		}
		text := queryExpansionText(item)
		if text != "" {
			inputs = append(inputs, s.formatQueryEmbeddingInput(text))
		}
	}
	return inputs, nil
}

// expandQueryWithOptions 优先调用支持参数的 expander，保持旧 QueryExpander 实现可继续使用。
func expandQueryWithOptions(ctx context.Context, expander QueryExpander, query string, opts ExpandQueryOptions) ([]QueryExpansion, error) {
	if withOptions, ok := expander.(QueryExpanderWithOptions); ok {
		return withOptions.ExpandWithOptions(ctx, query, opts)
	}
	return expander.Expand(ctx, query)
}

func (s *Store) searchVectorVariants(collection string, queryVecs [][]float32, limit int) ([]index.ChunkVectorResult, error) {
	if len(queryVecs) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}
	type bestDocChunk struct {
		result       index.ChunkVectorResult
		variantIndex int
	}
	allResults := map[string]*bestDocChunk{}
	perVariantLimit := limit * 3
	for variantIndex, queryVec := range queryVecs {
		raw, err := s.vecStore.SearchChunksByFingerprint(queryVec, s.embeddingFingerprint(), perVariantLimit)
		if err != nil {
			return nil, err
		}
		// qmd 的 searchVec 在单个 query variant 内先回表过滤 collection/active，
		// 再按文件保留最佳 chunk 并 slice(limit)。这里保持同样顺序。
		variantDocs := map[string]index.ChunkVectorResult{}
		for _, item := range raw {
			meta, err := s.getActiveDocByID(item.DocID)
			if err != nil || meta == nil || (collection != "" && meta.collection != collection) {
				continue
			}
			best, ok := variantDocs[item.DocID]
			if !ok || item.Score > best.Score || (item.Score == best.Score && item.ChunkID < best.ChunkID) {
				variantDocs[item.DocID] = item
			}
		}
		variantRanked := make([]index.ChunkVectorResult, 0, len(variantDocs))
		for _, item := range variantDocs {
			variantRanked = append(variantRanked, item)
		}
		sort.Slice(variantRanked, func(i, j int) bool {
			if variantRanked[i].Score == variantRanked[j].Score {
				if variantRanked[i].DocID == variantRanked[j].DocID {
					return variantRanked[i].ChunkID < variantRanked[j].ChunkID
				}
				return variantRanked[i].DocID < variantRanked[j].DocID
			}
			return variantRanked[i].Score > variantRanked[j].Score
		})
		if len(variantRanked) > limit {
			variantRanked = variantRanked[:limit]
		}
		for _, item := range variantRanked {
			best, ok := allResults[item.DocID]
			if !ok || item.Score > best.result.Score {
				allResults[item.DocID] = &bestDocChunk{result: item, variantIndex: variantIndex}
				continue
			}
			if item.Score == best.result.Score && variantIndex < best.variantIndex {
				allResults[item.DocID] = &bestDocChunk{result: item, variantIndex: variantIndex}
			}
		}
	}
	results := make([]index.ChunkVectorResult, 0, len(allResults))
	for _, item := range allResults {
		results = append(results, item.result)
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			if results[i].DocID == results[j].DocID {
				return results[i].ChunkID < results[j].ChunkID
			}
			return results[i].DocID < results[j].DocID
		}
		return results[i].Score > results[j].Score
	})
	for i := range results {
		results[i].Rank = i + 1
	}
	return results, nil
}

func (s *Store) vectorResults(collection string, raw []index.ChunkVectorResult, minScore float64, limit int) ([]SearchResult, error) {
	seenDocs := map[string]bool{}
	var results []SearchResult
	for _, r := range raw {
		if seenDocs[r.DocID] {
			continue
		}
		if r.Score < minScore {
			continue
		}
		meta, err := s.getActiveDocByID(r.DocID)
		if err != nil || meta == nil || (collection != "" && meta.collection != collection) {
			continue
		}
		chunk, err := s.getChunk(r.ChunkID)
		if err != nil {
			return nil, err
		}
		seenDocs[r.DocID] = true
		vectorRank := len(results) + 1
		results = append(results, SearchResult{
			ID:         r.DocID,
			Collection: meta.collection,
			Path:       meta.relPath,
			Title:      meta.title,
			Snippet:    snippet(chunk),
			ChunkID:    r.ChunkID,
			Score:      r.Score,
			VectorRank: vectorRank,
		})
		if len(results) >= limit {
			break
		}
	}
	return results, nil
}

func (s *Store) getChunk(chunkID string) (string, error) {
	var content string
	err := s.db.ReadDB().QueryRow(`SELECT content FROM chunks_meta WHERE chunk_id=?`, chunkID).Scan(&content)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return content, err
}

func fuseResults(bm25, vec []SearchResult, minScore float64, limit int) []SearchResult {
	type fused struct {
		result SearchResult
		score  float64
	}
	byID := map[string]*fused{}
	getFused := func(r SearchResult) *fused {
		if f, ok := byID[r.ID]; ok {
			return f
		}
		byID[r.ID] = &fused{result: r}
		return byID[r.ID]
	}

	// RRF 的核心是只使用排名，不直接比较 BM25 分和 cosine 分，
	// 这样两个分布完全不同的检索器也能稳定融合。
	for i, r := range bm25 {
		f := getFused(r)
		rank := r.BM25Rank
		if rank <= 0 {
			rank = i + 1
		}
		f.score += 1 / (rrfK + float64(rank))
		f.result.BM25Rank = rank
	}
	for i, r := range vec {
		f := getFused(r)
		rank := r.VectorRank
		if rank <= 0 {
			rank = i + 1
		}
		f.score += 1 / (rrfK + float64(rank))
		f.result.VectorRank = rank
		if f.result.Snippet == "" {
			f.result.Snippet = r.Snippet
			f.result.ChunkID = r.ChunkID
		}
	}

	results := make([]SearchResult, 0, len(byID))
	for _, f := range byID {
		f.result.Score = f.score
		if minScore == 0 || f.score >= minScore {
			results = append(results, f.result)
		}
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results
}
