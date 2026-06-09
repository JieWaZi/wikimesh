package qmd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

const queryDefaultCandidateLimit = 40
const queryBackendLimit = 20
const queryRRFK = 60.0

var queryIntentStopWords = map[string]struct{}{
	"am": {}, "an": {}, "as": {}, "at": {}, "be": {}, "by": {}, "do": {}, "he": {}, "if": {},
	"in": {}, "is": {}, "it": {}, "me": {}, "my": {}, "no": {}, "of": {}, "on": {}, "or": {}, "so": {},
	"to": {}, "up": {}, "us": {}, "we": {},
	"all": {}, "and": {}, "any": {}, "are": {}, "but": {}, "can": {}, "did": {}, "for": {}, "get": {},
	"has": {}, "her": {}, "him": {}, "his": {}, "how": {}, "its": {}, "let": {}, "may": {}, "not": {},
	"our": {}, "out": {}, "the": {}, "too": {}, "was": {}, "who": {}, "why": {}, "you": {},
	"also": {}, "does": {}, "find": {}, "from": {}, "have": {}, "into": {}, "more": {}, "need": {},
	"show": {}, "some": {}, "tell": {}, "that": {}, "them": {}, "this": {}, "want": {}, "what": {},
	"when": {}, "will": {}, "with": {}, "your": {},
	"about": {}, "looking": {}, "notes": {}, "search": {}, "where": {}, "which": {},
}

// queryList 是 qmd hybridQuery 中的一路后端检索列表。
// original FTS/Vector 使用 2x 权重，lex/vec/hyde expansion 使用 1x 权重。
type queryList struct {
	source    string
	queryType string
	query     string
	weight    float64
	results   []SearchResult
}

type queryVectorRequest struct {
	queryType string
	query     string
	weight    float64
}

// queryCandidate 是 RRF 融合后的文档候选。
// 它同时保存 qmd explain 需要的后端分、贡献项、最佳后端排名和最终位置分。
type queryCandidate struct {
	result        SearchResult
	baseScore     float64
	totalScore    float64
	positionScore float64
	rank          int
	topRank       int
	contributions []QueryContributionTrace
	ftsScores     []float64
	vectorScores  []float64
}

// queryQMD 实现 qmd src/store.ts 的 hybridQuery 设计。
// Go 侧不生成最终答案，只返回可供回答的检索上下文；当未配置 reranker 时走 qmd 的 skip-rerank 路径。
func (s *Store) queryQMD(ctx context.Context, collection string, question string, opts QueryOptions) (*QueryResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	limit := normalizeLimit(opts.Limit)
	candidateLimit := opts.CandidateLimit
	if candidateLimit <= 0 {
		candidateLimit = queryDefaultCandidateLimit
	}

	lists, primaryQuestion, err := s.queryRetrievalLists(ctx, collection, question, opts, queryBackendLimit)
	if err != nil {
		return nil, err
	}
	candidates, err := s.queryCandidates(ctx, lists, primaryQuestion, opts.Intent, opts.Explain)
	if err != nil {
		return nil, err
	}
	if len(candidates) > candidateLimit {
		candidates = candidates[:candidateLimit]
	}
	if len(candidates) == 0 {
		return &QueryResult{Question: question}, nil
	}

	if opts.SkipRerank || s.queryReranker == nil {
		results := make([]SearchResult, 0, minInt(limit, len(candidates)))
		for i := 0; i < len(candidates) && len(results) < limit; i++ {
			c := candidates[i]
			c.result.Score = c.positionScore
			if opts.MinScore == 0 || c.result.Score >= opts.MinScore {
				results = append(results, c.result)
			}
		}
		return &QueryResult{Question: question, Results: results}, nil
	}

	rerankDocs := make([]QueryRerankDocument, 0, minInt(candidateLimit, len(candidates)))
	for i := 0; i < len(candidates) && len(rerankDocs) < candidateLimit; i++ {
		r := candidates[i].result
		rerankDocs = append(rerankDocs, QueryRerankDocument{
			ID:   r.ID,
			File: r.File,
			Text: r.BestChunk,
		})
	}
	scores, err := s.queryReranker.Rerank(ctx, queryRerankQuestion(primaryQuestion, opts.Intent), rerankDocs)
	if err != nil {
		return nil, err
	}
	scoreByID := map[string]float64{}
	scoreByFile := map[string]float64{}
	for _, score := range scores {
		if score.ID != "" {
			scoreByID[score.ID] = score.Score
		}
		if score.File != "" {
			scoreByFile[score.File] = score.Score
		}
	}

	reranked := make([]SearchResult, 0, len(rerankDocs))
	for i := 0; i < len(candidates) && i < len(rerankDocs); i++ {
		c := candidates[i]
		rerankScore, ok := scoreByID[c.result.ID]
		if !ok {
			rerankScore = scoreByFile[c.result.File]
		}
		rrfWeight := queryRRFBlendWeight(c.rank)
		c.result.Score = rrfWeight*c.positionScore + (1-rrfWeight)*rerankScore
		if c.result.Explain != nil {
			c.result.Explain.RRF.Weight = rrfWeight
			c.result.Explain.RerankScore = rerankScore
			c.result.Explain.BlendedScore = c.result.Score
		}
		if opts.MinScore == 0 || c.result.Score >= opts.MinScore {
			reranked = append(reranked, c.result)
		}
	}
	sort.SliceStable(reranked, func(i, j int) bool {
		if reranked[i].Score == reranked[j].Score {
			return reranked[i].File < reranked[j].File
		}
		return reranked[i].Score > reranked[j].Score
	})
	if len(reranked) > limit {
		reranked = reranked[:limit]
	}
	return &QueryResult{Question: question, Results: reranked}, nil
}

// queryRetrievalLists 构造 qmd 的多路检索列表。
// 关键边界：QueryExpander 只在 query 层调用一次，lex 只进 FTS，vec/hyde 只进单次向量检索。
func (s *Store) queryRetrievalLists(ctx context.Context, collection, question string, opts QueryOptions, limit int) ([]queryList, string, error) {
	searchQueries := normalizeSearchQueries(opts.SearchQueries)
	if len(opts.Queries) > 0 && len(searchQueries) > 0 {
		return nil, "", errors.New("query options cannot set both Queries and SearchQueries")
	}
	if len(opts.Queries) > 0 {
		lists, primaryQuestion, err := s.queryStructuredRetrievalLists(ctx, collection, opts.Queries, limit)
		return lists, primaryQuestion, err
	}
	if len(searchQueries) > 0 {
		return s.querySearchRetrievalLists(ctx, collection, question, searchQueries, limit)
	}
	lists := []queryList{}
	addFTSResults := func(queryType, query string, weight float64, results []SearchResult) {
		lists = append(lists, queryList{source: "fts", queryType: queryType, query: query, weight: weight, results: results})
	}
	searchFTS := func(query string) ([]SearchResult, error) {
		results, err := s.Search(ctx, collection, query, SearchOptions{Limit: limit})
		if err != nil {
			return nil, err
		}
		return results, nil
	}
	initialFTS, err := searchFTS(question)
	if err != nil {
		return nil, "", err
	}
	addFTSResults("original", question, 2.0, initialFTS)
	vectorRequests := []queryVectorRequest{{queryType: "original", query: question, weight: 2.0}}
	if s.queryExpander == nil {
		vectorLists, err := s.queryVectorRetrievalLists(collection, vectorRequests, limit)
		if err != nil {
			return nil, "", err
		}
		lists = append(lists, vectorLists...)
		return lists, question, nil
	}

	// qmd 的 strong BM25 signal 会跳过昂贵的 query expansion，但仍保留原始 FTS/Vector 两路证据。
	if opts.Intent == "" && queryHasStrongSignal(initialFTS) {
		vectorLists, err := s.queryVectorRetrievalLists(collection, vectorRequests, limit)
		if err != nil {
			return nil, "", err
		}
		lists = append(lists, vectorLists...)
		return lists, question, nil
	}
	expansions, err := expandQueryWithOptions(ctx, s.queryExpander, question, ExpandQueryOptions{Intent: opts.Intent})
	if err != nil {
		return nil, "", err
	}
	for _, expansion := range expansions {
		text := queryExpansionText(expansion)
		if text == "" {
			continue
		}
		switch expansion.Type {
		case QueryExpansionLex:
			results, err := searchFTS(text)
			if err != nil {
				return nil, "", err
			}
			addFTSResults("lex", text, 1.0, results)
		case QueryExpansionVec, QueryExpansionHyDE:
			vectorRequests = append(vectorRequests, queryVectorRequest{queryType: string(expansion.Type), query: text, weight: 1.0})
		}
	}
	vectorLists, err := s.queryVectorRetrievalLists(collection, vectorRequests, limit)
	if err != nil {
		return nil, "", err
	}
	lists = append(lists, vectorLists...)
	return lists, question, nil
}

// querySearchRetrievalLists 执行调用方显式传入的普通多 query 检索路径。
// 主问题仍走原始 FTS/Vector，辅助 query 只进 FTS，避免多关键词查询成倍触发向量检索。
func (s *Store) querySearchRetrievalLists(ctx context.Context, collection, question string, queries []string, limit int) ([]queryList, string, error) {
	lists := []queryList{}
	primaryQuestion := strings.TrimSpace(question)
	if primaryQuestion == "" && len(queries) > 0 {
		primaryQuestion = queries[0]
	}
	addList := func(source, queryType, query string, weight float64, results []SearchResult) {
		if len(results) == 0 {
			return
		}
		lists = append(lists, queryList{source: source, queryType: queryType, query: query, weight: weight, results: results})
	}
	if primaryQuestion != "" {
		results, err := s.Search(ctx, collection, primaryQuestion, SearchOptions{Limit: limit})
		if err != nil {
			return nil, "", err
		}
		addList("fts", "original", primaryQuestion, 2.0, results)
		vectorLists, err := s.queryVectorRetrievalLists(collection, []queryVectorRequest{{queryType: "original", query: primaryQuestion, weight: 2.0}}, limit)
		if err != nil {
			return nil, "", err
		}
		lists = append(lists, vectorLists...)
	}
	for _, query := range queries {
		results, err := s.Search(ctx, collection, query, SearchOptions{Limit: limit})
		if err != nil {
			return nil, "", err
		}
		addList("fts", "search", query, 1.0, results)
	}
	return lists, primaryQuestion, nil
}

// queryStructuredRetrievalLists 执行 qmd structuredSearch 的预展开查询路径。
// 它不调用 QueryExpander，按调用方提供的 lex/vec/hyde 精确分路。
func (s *Store) queryStructuredRetrievalLists(ctx context.Context, collection string, queries []QueryExpansion, limit int) ([]queryList, string, error) {
	lists := []queryList{}
	if err := validateStructuredQueries(queries); err != nil {
		return nil, "", err
	}
	primaryQuestion := primaryStructuredQuery(queries)
	addList := func(source, queryType, query string, results []SearchResult) {
		if len(results) == 0 {
			return
		}
		weight := 1.0
		if len(lists) == 0 {
			// qmd structuredSearch 认为第一路有效列表代表调用方最重要的检索意图。
			weight = 2.0
		}
		lists = append(lists, queryList{source: source, queryType: queryType, query: query, weight: weight, results: results})
	}
	vectorRequests := []queryVectorRequest{}
	for _, expansion := range queries {
		expansion = normalizeQueryExpansion(expansion)
		text := queryExpansionText(expansion)
		if text == "" {
			continue
		}
		switch expansion.Type {
		case QueryExpansionLex:
			results, err := s.Search(ctx, collection, text, SearchOptions{Limit: limit})
			if err != nil {
				return nil, "", err
			}
			addList("fts", "lex", text, results)
		case QueryExpansionVec, QueryExpansionHyDE:
			vectorRequests = append(vectorRequests, queryVectorRequest{queryType: string(expansion.Type), query: text})
		}
	}
	vectorLists, err := s.queryVectorRetrievalLists(collection, vectorRequests, limit)
	if err != nil {
		return nil, "", err
	}
	for _, list := range vectorLists {
		addList(list.source, list.queryType, list.query, list.results)
	}
	return lists, primaryQuestion, nil
}

func (s *Store) queryVectorRetrievalLists(collection string, requests []queryVectorRequest, limit int) ([]queryList, error) {
	if s.embedder == nil || len(requests) == 0 {
		return nil, nil
	}
	hasVectors, err := s.hasVectorIndexForCollection(collection)
	if err != nil {
		return nil, err
	}
	if !hasVectors {
		return nil, nil
	}
	inputs := make([]string, 0, len(requests))
	for _, req := range requests {
		inputs = append(inputs, s.formatQueryEmbeddingInput(req.query))
	}
	vectors, err := s.embedQueryInputs(inputs)
	if err != nil {
		return nil, err
	}
	lists := make([]queryList, 0, len(requests))
	for i, req := range requests {
		if i >= len(vectors) || len(vectors[i]) == 0 {
			continue
		}
		raw, err := s.searchVectorVariants(collection, [][]float32{vectors[i]}, limit)
		if err != nil {
			return nil, err
		}
		results, err := s.vectorResults(collection, raw, -2, limit)
		if err != nil {
			return nil, err
		}
		weight := req.weight
		if weight == 0 {
			weight = 1.0
		}
		lists = append(lists, queryList{source: "vec", queryType: req.queryType, query: req.query, weight: weight, results: results})
	}
	return lists, nil
}

// queryVectorSearchOnce 执行单个 query 文本的向量检索。
// 这里不能调用 VSearch，因为 VSearch 自己会消费 QueryExpander；qmd hybridQuery 要求 typed expansion 已在 query 层完成。
func (s *Store) queryVectorSearchOnce(collection, query string, limit int) ([]SearchResult, error) {
	if s.embedder == nil {
		return nil, nil
	}
	hasVectors, err := s.hasVectorIndexForCollection(collection)
	if err != nil {
		return nil, err
	}
	if !hasVectors {
		return nil, nil
	}
	vec, err := s.embedder.Embed(s.formatQueryEmbeddingInput(query))
	if err != nil {
		return nil, err
	}
	raw, err := s.searchVectorVariants(collection, [][]float32{vec}, limit)
	if err != nil {
		return nil, err
	}
	return s.vectorResults(collection, raw, -2, limit)
}

func (s *Store) hasVectorIndexForCollection(collection string) (bool, error) {
	query := `SELECT COUNT(*)
FROM vec_chunks AS v
JOIN collection_documents AS d ON d.id = v.doc_id
WHERE d.active = 1 AND v.embed_fingerprint = ?`
	args := []any{s.embeddingFingerprint()}
	if collection != "" {
		query += " AND d.collection = ?"
		args = append(args, collection)
	}
	var count int
	if err := s.db.ReadDB().QueryRow(query, args...).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// queryCandidates 对所有后端列表做 qmd RRF 融合。
// RRF contribution 使用 weight/(60+rank)，top-rank bonus 使用文档在任意后端列表内的最佳排名。
func (s *Store) queryCandidates(ctx context.Context, lists []queryList, question string, intent string, explain bool) ([]queryCandidate, error) {
	byID := map[string]*queryCandidate{}
	for listIndex, list := range lists {
		for i, result := range list.results {
			rank := i + 1
			candidate, ok := byID[result.ID]
			if !ok {
				enriched, err := s.enrichQueryResult(ctx, result, question, intent, "")
				if err != nil {
					return nil, err
				}
				candidate = &queryCandidate{result: enriched}
				byID[result.ID] = candidate
			}
			if candidate.topRank == 0 || rank < candidate.topRank {
				candidate.topRank = rank
			}
			contribution := list.weight / (queryRRFK + float64(rank))
			candidate.baseScore += contribution
			trace := QueryContributionTrace{
				ListIndex:       listIndex,
				Source:          list.source,
				QueryType:       list.queryType,
				Query:           list.query,
				Rank:            rank,
				Weight:          list.weight,
				BackendScore:    result.Score,
				RRFContribution: contribution,
			}
			candidate.contributions = append(candidate.contributions, trace)
			if list.source == "fts" {
				candidate.ftsScores = append(candidate.ftsScores, result.Score)
			} else {
				candidate.vectorScores = append(candidate.vectorScores, result.Score)
				if result.ChunkID != "" && candidate.result.ChunkID == "" {
					enriched, err := s.enrichQueryResult(ctx, result, question, intent, result.ChunkID)
					if err != nil {
						return nil, err
					}
					candidate.result = enriched
				}
			}
		}
	}

	candidates := make([]queryCandidate, 0, len(byID))
	for _, candidate := range byID {
		candidate.totalScore = candidate.baseScore + queryTopRankBonus(candidate.topRank)
		candidates = append(candidates, *candidate)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].totalScore == candidates[j].totalScore {
			return candidates[i].result.File < candidates[j].result.File
		}
		return candidates[i].totalScore > candidates[j].totalScore
	})
	for i := range candidates {
		rank := i + 1
		candidates[i].rank = rank
		candidates[i].positionScore = 1.0 / float64(rank)
		bonus := queryTopRankBonus(candidates[i].topRank)
		if explain {
			candidates[i].result.Explain = &QueryExplain{
				FTSScores:    candidates[i].ftsScores,
				VectorScores: candidates[i].vectorScores,
				RRF: QueryRRFExplain{
					Rank:          rank,
					TopRank:       candidates[i].topRank,
					PositionScore: candidates[i].positionScore,
					Weight:        1.0,
					BaseScore:     candidates[i].baseScore,
					TopRankBonus:  bonus,
					TotalScore:    candidates[i].totalScore,
					Contributions: candidates[i].contributions,
				},
				BlendedScore: candidates[i].positionScore,
			}
		}
	}
	return candidates, nil
}

// enrichQueryResult 补齐 query 输出需要的 qmd 字段：虚拟路径、短 docid、path context 和 best chunk。
func (s *Store) enrichQueryResult(ctx context.Context, result SearchResult, question string, intent string, preferredChunkID string) (SearchResult, error) {
	meta, err := s.getActiveDocByID(result.ID)
	if err != nil {
		return result, err
	}
	if meta == nil {
		return result, nil
	}
	result.Collection = meta.collection
	result.Path = meta.relPath
	result.Title = meta.title
	result.File = qmdURI(meta.collection, meta.relPath)
	result.DocID = qmdDocID(meta.hash)
	contextText, err := s.ContextForPath(ctx, meta.collection, meta.relPath)
	if err != nil {
		return result, err
	}
	result.Context = contextText
	chunk, chunkID, pos, err := s.bestQueryChunk(result.ID, question, intent, preferredChunkID)
	if err != nil {
		return result, err
	}
	result.BestChunk = chunk
	result.BestChunkPos = pos
	result.ChunkID = chunkID
	if result.Snippet == "" {
		result.Snippet = snippet(chunk)
	}
	return result, nil
}

// bestQueryChunk 选择送入 reranker 的 chunk。
// qmd hybridQuery 按原始问题词和 intent 词在 chunk 中的重叠数量选择，未命中时回退第一段 chunk。
func (s *Store) bestQueryChunk(docID, question, intent, preferredChunkID string) (string, string, int, error) {
	if preferredChunkID != "" {
		chunk, idx, err := s.chunkByID(preferredChunkID)
		if err != nil {
			return "", "", 0, err
		}
		if chunk != "" {
			return chunk, preferredChunkID, idx, nil
		}
	}
	rows, err := s.db.ReadDB().Query(`
SELECT chunk_id, content, chunk_index, start_offset
FROM chunks_meta
WHERE doc_id=?
ORDER BY chunk_index
`, docID)
	if err != nil {
		return "", "", 0, err
	}
	defer rows.Close()
	var fallbackChunk, fallbackID string
	var fallbackPos int
	bestScore := -1
	for rows.Next() {
		var chunkID, content string
		var index int
		var startOffset sql.NullInt64
		if err := rows.Scan(&chunkID, &content, &index, &startOffset); err != nil {
			return "", "", 0, err
		}
		pos := index
		if startOffset.Valid {
			pos = int(startOffset.Int64)
		}
		if fallbackChunk == "" {
			fallbackChunk, fallbackID, fallbackPos = content, chunkID, pos
		}
		score := queryChunkOverlapScore(question, intent, content)
		if score > bestScore {
			bestScore = score
			fallbackChunk, fallbackID, fallbackPos = content, chunkID, pos
		}
	}
	if err := rows.Err(); err != nil {
		return "", "", 0, err
	}
	return fallbackChunk, fallbackID, fallbackPos, nil
}

// chunkByID 读取指定 chunk 的正文和源文本位置。
func (s *Store) chunkByID(chunkID string) (string, int, error) {
	var content string
	var index int
	var startOffset sql.NullInt64
	err := s.db.ReadDB().QueryRow(`SELECT content, chunk_index, start_offset FROM chunks_meta WHERE chunk_id=?`, chunkID).Scan(&content, &index, &startOffset)
	if err == sql.ErrNoRows {
		return "", 0, nil
	}
	if startOffset.Valid {
		index = int(startOffset.Int64)
	}
	return content, index, err
}

// queryChunkOverlapScore 计算问题词、intent 词和 chunk 的简单重叠分。
// qmd 中 intent terms 按 0.5 权重参与 best chunk 选择；这里用 2/1 整数权重避免浮点比较噪声。
func queryChunkOverlapScore(query string, intent string, content string) int {
	questionTerms := queryTermList(query)
	intentTerms := queryIntentTermList(intent)
	contentLower := strings.ToLower(content)
	score := 0
	for _, term := range questionTerms {
		if strings.Contains(contentLower, term) {
			score += 2
		}
	}
	for _, term := range intentTerms {
		if strings.Contains(contentLower, term) {
			score++
		}
	}
	return score
}

func queryTermList(text string) []string {
	raw := strings.Fields(strings.ToLower(text))
	terms := make([]string, 0, len(raw))
	for _, term := range raw {
		term = strings.Trim(term, "_./,;:!?()[]{}\"'")
		if len(term) >= 3 {
			terms = append(terms, term)
		}
	}
	return terms
}

func queryIntentTermList(text string) []string {
	raw := strings.Fields(strings.ToLower(text))
	terms := make([]string, 0, len(raw))
	for _, term := range raw {
		term = strings.TrimFunc(term, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r)
		})
		if len(term) > 1 {
			if _, stop := queryIntentStopWords[term]; !stop {
				terms = append(terms, term)
			}
		}
	}
	return terms
}

func queryRerankQuestion(question string, intent string) string {
	intent = strings.TrimSpace(intent)
	if intent == "" {
		return question
	}
	return intent + "\n\n" + question
}

// queryExpansionText 读取扩展查询文本，兼容 Go Text 字段和 qmd JSON query 字段。
func queryExpansionText(expansion QueryExpansion) string {
	if text := strings.TrimSpace(expansion.Text); text != "" {
		return text
	}
	return strings.TrimSpace(expansion.Query)
}

// normalizeQueryExpansion 补齐 Text/Query 两套字段，方便 Go 调用和 qmd JSON 输入互通。
func normalizeQueryExpansion(expansion QueryExpansion) QueryExpansion {
	text := queryExpansionText(expansion)
	expansion.Text = text
	expansion.Query = text
	return expansion
}

// primaryStructuredQuery 对齐 qmd structuredSearch 的 best-chunk 主查询选择。
func primaryStructuredQuery(queries []QueryExpansion) string {
	for _, expansion := range queries {
		expansion = normalizeQueryExpansion(expansion)
		if expansion.Type == QueryExpansionLex && expansion.Text != "" {
			return expansion.Text
		}
	}
	for _, expansion := range queries {
		expansion = normalizeQueryExpansion(expansion)
		if expansion.Type == QueryExpansionVec && expansion.Text != "" {
			return expansion.Text
		}
	}
	for _, expansion := range queries {
		expansion = normalizeQueryExpansion(expansion)
		if expansion.Text != "" {
			return expansion.Text
		}
	}
	return ""
}

func validateStructuredQueries(queries []QueryExpansion) error {
	for _, expansion := range queries {
		expansion = normalizeQueryExpansion(expansion)
		text := queryExpansionText(expansion)
		location := "Structured search"
		if expansion.Line > 0 {
			location = fmt.Sprintf("Line %d", expansion.Line)
		}
		prefix := fmt.Sprintf("%s (%s): ", location, expansion.Type)
		if strings.ContainsAny(text, "\r\n") {
			return errors.New(prefix + "queries must be single-line")
		}
		switch expansion.Type {
		case QueryExpansionLex:
			if strings.Count(text, "\"")%2 == 1 {
				return errors.New(prefix + "lex query has an unmatched double quote")
			}
		case QueryExpansionVec, QueryExpansionHyDE:
			if semanticQueryHasNegation(text) {
				return errors.New(prefix + "Negation (-term) is not supported in vec/hyde queries")
			}
		}
	}
	return nil
}

func semanticQueryHasNegation(query string) bool {
	atTokenStart := true
	for _, r := range query {
		if r == '-' && atTokenStart {
			return true
		}
		atTokenStart = unicode.IsSpace(r)
	}
	return false
}

// qmdURI 返回 qmd://collection/path 虚拟路径。
func qmdURI(collection, relPath string) string {
	return "qmd://" + collection + "/" + relPath
}

// qmdDocID 返回 qmd 输出里展示的短内容 hash。
func qmdDocID(hash string) string {
	if len(hash) >= 6 {
		return hash[:6]
	}
	return hash
}

// queryTopRankBonus 是 qmd RRF 的 top-rank bonus：任一列表 rank1 加 0.05，rank2-3 加 0.02。
func queryTopRankBonus(rank int) float64 {
	switch rank {
	case 1:
		return 0.05
	case 2, 3:
		return 0.02
	default:
		return 0
	}
}

// queryRRFBlendWeight 是 qmd rerank 后的位置感知融合权重。
// 排名前 3 更信任检索，4-10 适中，10 名以后更信任 reranker。
func queryRRFBlendWeight(rank int) float64 {
	if rank <= 3 {
		return 0.75
	}
	if rank <= 10 {
		return 0.60
	}
	return 0.40
}

// queryHasStrongSignal 判断原始 BM25 是否足够强，强信号时跳过 query expansion。
func queryHasStrongSignal(results []SearchResult) bool {
	if len(results) == 0 {
		return false
	}
	top := results[0].Score
	second := 0.0
	if len(results) > 1 {
		second = results[1].Score
	}
	return top >= 0.85 && top-second >= 0.15
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
