package extract

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// SourceContent 是从一个源文件解析出的文本结果。
type SourceContent struct {
	Path       string  // Path 是源文件路径。
	Type       string  // Type 是文档类型，例如 article、paper、code。
	Text       string  // Text 是后续写入 FTS 和 chunk 的原始 Markdown 文本。
	Chunks     []Chunk // Chunks 是按标题或段落切分后的片段。
	ChunkCount int     // ChunkCount 是切片数量。
}

// Chunk 是大文档中的一段文本。
type Chunk struct {
	Index       int    // Index 是 chunk 在文档中的顺序。
	Text        string // Text 是 chunk 正文。
	Heading     string // Heading 是 chunk 所属标题，没有标题时为空。
	StartOffset int    // StartOffset 是 chunk 在源文本中的 byte offset，对齐 qmd bestChunkPos。
	EndOffset   int    // EndOffset 是 chunk 在源文本中的结束 byte offset。
}

// sourceExtractor 是文件解析器的扩展点。
// 当前只注册 Markdown；后续新增 PDF、Office 等格式时实现该接口并注册扩展名即可。
type sourceExtractor interface {
	Extract(path string, sourceType string) (*SourceContent, error)
}

// extractors 是按扩展名注册的解析器表。
// Store 只依赖 Extract 函数，不需要知道具体文件类型。
var extractors = map[string]sourceExtractor{
	".md": markdownExtractor{},
}

// Extract 根据文件扩展名选择解析器并返回文档文本。
// 当前仅支持 Markdown，避免在没有明确需求前引入 PDF、Office 等额外依赖。
func Extract(path string, sourceType string) (*SourceContent, error) {
	ext := strings.ToLower(filepath.Ext(path))
	extractor, ok := extractors[ext]
	if !ok {
		return nil, fmt.Errorf("unsupported source extension %q: only .md is currently supported", ext)
	}
	return extractor.Extract(path, sourceType)
}

// EstimateTokens 粗略估算 token 数。
// 英文按约 4 字符 1 token，CJK 按更高密度估算，避免中文长文切片过大。
func EstimateTokens(text string) int {
	var cjk, other int
	for _, r := range text {
		if isCJK(r) {
			cjk++
		} else {
			other++
		}
	}
	return int(float64(cjk)*1.5) + other/4
}

// isCJK 判断 rune 是否属于 CJK 表意文字或音节字符。
func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		unicode.Is(unicode.Hangul, r) ||
		unicode.Is(unicode.Katakana, r) ||
		unicode.Is(unicode.Hiragana, r)
}

// ChunkText 直接对原始文本做切片。
// Store 索引文档时使用它把全文拆成 chunk，再分别写入 chunk FTS 和向量表。
func ChunkText(text string, maxTokens int) []Chunk {
	return chunkTextWithBreakpoints(text, maxTokens, 0)
}

// ChunkTextWithOverlap 使用 Markdown 断点评分算法切片，并按源文本位置回退 overlap。
// overlapRatio 表示重叠比例，例如 0.15 会让下一片从上一片结束位置前约 15% token 处开始。
func ChunkTextWithOverlap(text string, maxTokens int, overlapRatio float64) []Chunk {
	return chunkTextWithBreakpoints(text, maxTokens, overlapRatio)
}

// ChunkIfNeeded 在文本超过阈值时切片。
// 切片算法包含默认窗口、重叠比例、Markdown 断点评分和代码围栏保护。
func ChunkIfNeeded(content *SourceContent, maxTokens int) {
	content.Chunks = ChunkText(content.Text, maxTokens)
	content.ChunkCount = len(content.Chunks)
}

const breakpointCharsPerToken = 4
const breakpointChunkWindowTokens = 200

// chunkBreakPoint 是候选切分点。
// pos 使用 Go 字符串 byte offset；所有候选都来自换行/Markdown 标记，因此不会落在 UTF-8 字符中间。
type chunkBreakPoint struct {
	pos   int    // pos 是候选切分位置。
	score int    // score 是断点质量分，标题高于段落，段落高于普通换行。
	kind  string // kind 记录断点类型，便于调试和后续解释。
}

// codeFenceRegion 表示 Markdown ``` 代码围栏范围。
// 查找切分点时会跳过围栏内部位置，避免把代码块拆成不完整片段。
type codeFenceRegion struct {
	start int
	end   int
}

// chunkTextWithBreakpoints 使用断点评分选择 chunk 边界。
// 关键步骤：字符窗口近似 token -> 扫描 Markdown 断点 -> 在目标窗口内按距离衰减选最佳断点 -> 按源位置 overlap。
func chunkTextWithBreakpoints(text string, maxTokens int, overlapRatio float64) []Chunk {
	if maxTokens <= 0 {
		return []Chunk{{Index: 0, Text: text, Heading: firstHeading(text), StartOffset: 0, EndOffset: len(text)}}
	}
	maxChars := maxTokens * breakpointCharsPerToken
	if len(text) <= maxChars {
		return []Chunk{{Index: 0, Text: text, Heading: firstHeading(text), StartOffset: 0, EndOffset: len(text)}}
	}

	overlapTokens := int(float64(maxTokens) * overlapRatio)
	overlapChars := overlapTokens * breakpointCharsPerToken
	windowChars := breakpointChunkWindowTokens * breakpointCharsPerToken
	breakPoints := scanChunkBreakPoints(text)
	codeFences := findCodeFences(text)

	var chunks []Chunk
	charPos := 0
	for charPos < len(text) {
		targetEndPos := safeBytePos(text, minInt(charPos+maxChars, len(text)))
		endPos := targetEndPos

		if endPos < len(text) {
			bestCutoff := findBestChunkCutoff(breakPoints, targetEndPos, windowChars, codeFences)
			if bestCutoff > charPos && bestCutoff <= targetEndPos {
				endPos = bestCutoff
			}
		}
		if endPos <= charPos {
			endPos = safeBytePos(text, minInt(charPos+maxChars, len(text)))
		}

		chunkText := text[charPos:endPos]
		if strings.TrimSpace(chunkText) != "" {
			chunks = append(chunks, Chunk{
				Index:       len(chunks),
				Text:        chunkText,
				Heading:     firstHeading(chunkText),
				StartOffset: charPos,
				EndOffset:   endPos,
			})
		}
		if endPos >= len(text) {
			break
		}

		nextPos := endPos - overlapChars
		nextPos = safeBytePos(text, nextPos)
		// 防停滞规则：overlap 不能让下一片回到当前片起点或更早。
		if nextPos <= charPos {
			nextPos = endPos
		}
		charPos = nextPos
	}
	if len(chunks) == 0 {
		return []Chunk{{Index: 0, Text: text, Heading: firstHeading(text), StartOffset: 0, EndOffset: len(text)}}
	}
	return chunks
}

// scanChunkBreakPoints 扫描 Markdown 断点。
// 分值规则：h1=100、h2=90、h3/code=80、hr=60、空行=20、列表=5、换行=1。
func scanChunkBreakPoints(text string) []chunkBreakPoint {
	seen := map[int]chunkBreakPoint{}
	add := func(pos int, score int, kind string) {
		if pos < 0 || pos >= len(text) {
			return
		}
		old, ok := seen[pos]
		if !ok || score > old.score {
			seen[pos] = chunkBreakPoint{pos: pos, score: score, kind: kind}
		}
	}

	for pos := 0; pos < len(text); {
		if text[pos] != '\n' {
			pos++
			continue
		}
		add(pos, 1, "newline")
		next := pos + 1
		switch {
		case hasHeadingBreak(text, next, 1):
			add(pos, 100, "h1")
		case hasHeadingBreak(text, next, 2):
			add(pos, 90, "h2")
		case hasHeadingBreak(text, next, 3):
			add(pos, 80, "h3")
		case hasHeadingBreak(text, next, 4):
			add(pos, 70, "h4")
		case hasHeadingBreak(text, next, 5):
			add(pos, 60, "h5")
		case hasHeadingBreak(text, next, 6):
			add(pos, 50, "h6")
		}
		if strings.HasPrefix(text[next:], "```") {
			add(pos, 80, "codeblock")
		}
		if hasHorizontalRule(text, next) {
			add(pos, 60, "hr")
		}
		if next < len(text) && text[next] == '\n' {
			add(pos, 20, "blank")
			for next < len(text) && text[next] == '\n' {
				next++
			}
			pos = next
			continue
		}
		if hasListBreak(text, next) {
			add(pos, 5, "list")
		}
		if hasNumberedListBreak(text, next) {
			add(pos, 5, "numlist")
		}
		pos++
	}

	points := make([]chunkBreakPoint, 0, len(seen))
	for _, point := range seen {
		points = append(points, point)
	}
	sort.Slice(points, func(i, j int) bool { return points[i].pos < points[j].pos })
	return points
}

func hasHeadingBreak(text string, pos int, level int) bool {
	if pos+level > len(text) {
		return false
	}
	for i := 0; i < level; i++ {
		if text[pos+i] != '#' {
			return false
		}
	}
	return pos+level >= len(text) || text[pos+level] != '#'
}

func hasHorizontalRule(text string, pos int) bool {
	lineEnd := strings.IndexByte(text[pos:], '\n')
	if lineEnd < 0 {
		return false
	}
	line := strings.TrimSpace(text[pos : pos+lineEnd])
	return line == "---" || line == "***" || line == "___"
}

func hasListBreak(text string, pos int) bool {
	if pos+2 > len(text) {
		return false
	}
	return (text[pos] == '-' || text[pos] == '*') && text[pos+1] == ' '
}

func hasNumberedListBreak(text string, pos int) bool {
	start := pos
	for pos < len(text) && text[pos] >= '0' && text[pos] <= '9' {
		pos++
	}
	return pos > start && pos+2 <= len(text) && text[pos] == '.' && text[pos+1] == ' '
}

// findCodeFences 查找 ``` 围栏范围。
// 未闭合围栏按延伸到文档末尾处理，避免在剩余文本内部切开代码。
func findCodeFences(text string) []codeFenceRegion {
	var regions []codeFenceRegion
	inFence := false
	fenceStart := 0
	for pos := 0; pos < len(text); pos++ {
		if text[pos] != '\n' || !strings.HasPrefix(text[pos+1:], "```") {
			continue
		}
		if !inFence {
			fenceStart = pos
			inFence = true
		} else {
			regions = append(regions, codeFenceRegion{start: fenceStart, end: pos + len("\n```")})
			inFence = false
		}
	}
	if inFence {
		regions = append(regions, codeFenceRegion{start: fenceStart, end: len(text)})
	}
	return regions
}

// findBestChunkCutoff 在目标位置前的搜索窗口中选择最佳切分点。
// 距离衰减使用平方衰减，保证远处标题仍能压过近处普通换行。
func findBestChunkCutoff(points []chunkBreakPoint, targetCharPos int, windowChars int, codeFences []codeFenceRegion) int {
	windowStart := targetCharPos - windowChars
	bestScore := -1.0
	bestPos := targetCharPos
	for _, point := range points {
		if point.pos < windowStart {
			continue
		}
		if point.pos > targetCharPos {
			break
		}
		if isInsideCodeFence(point.pos, codeFences) {
			continue
		}
		distance := targetCharPos - point.pos
		normalized := float64(distance) / float64(windowChars)
		multiplier := 1.0 - (normalized * normalized * 0.7)
		finalScore := float64(point.score) * multiplier
		if finalScore > bestScore {
			bestScore = finalScore
			bestPos = point.pos
		}
	}
	return bestPos
}

func isInsideCodeFence(pos int, fences []codeFenceRegion) bool {
	for _, fence := range fences {
		if pos > fence.start && pos < fence.end {
			return true
		}
	}
	return false
}

func firstHeading(text string) string {
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			return stripHeadingPrefix(trimmed)
		}
	}
	return ""
}

func safeBytePos(text string, pos int) int {
	if pos <= 0 {
		return 0
	}
	if pos >= len(text) {
		return len(text)
	}
	for pos > 0 && !utf8.RuneStart(text[pos]) {
		pos--
	}
	return pos
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

// markdownExtractor 读取 Markdown 原文。
type markdownExtractor struct{}

func (markdownExtractor) Extract(path string, sourceType string) (*SourceContent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("extract markdown: %w", err)
	}

	if sourceType == "" || sourceType == "auto" {
		sourceType = "article"
	}

	return &SourceContent{
		Path: path,
		Type: sourceType,
		Text: string(data),
	}, nil
}

// stripHeadingPrefix 去掉 Markdown 标题前缀。
func stripHeadingPrefix(line string) string {
	i := 0
	for i < len(line) && line[i] == '#' {
		i++
	}
	for i < len(line) && line[i] == ' ' {
		i++
	}
	return line[i:]
}
