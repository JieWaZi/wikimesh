package qmd

import (
	"context"

	"github.com/JieWaZi/wikimesh/pkg/qmd/embed"
)

// Config 是 Store 的总配置。
// 调用方只需要提供文档检索相关信息，不需要关心底层 SQLite/FTS/向量表的细节。
type Config struct {
	// DBPath 是 SQLite 数据库文件路径；为空时使用当前目录下的 wikimesh.db。
	DBPath string `yaml:"db_path"`

	// ChunkSize 是文档切片大小，单位近似为 token；小于等于 0 时使用默认值 900。
	ChunkSize int `yaml:"chunk_size"`

	// ChunkOverlap 是相邻 chunk 的重叠比例；小于等于 0 时使用 VSearch 默认值 0.15。
	ChunkOverlap float64 `yaml:"chunk_overlap"`

	// Embedding 是 API、Ollama 或本地 GGUF 的公开向量配置。
	Embedding EmbeddingConfig `yaml:"embedding"`

	// Models 保存 qmd 风格的默认模型路径。
	Models ModelsConfig `yaml:"models"`

	// Embedder 允许测试或业务方直接注入向量模型；设置后优先于 Embedding 配置。
	Embedder Embedder `yaml:"-"`

	// QueryExpander 生成查询扩展；VSearch 只消费 vec/hyde 类型。
	QueryExpander QueryExpander `yaml:"-"`

	// QueryReranker 对 query 候选 chunk 做交叉编码器式重排；未配置时 Query 按 qmd 的 no-rerank 路径返回 RRF 排名。
	QueryReranker QueryReranker `yaml:"-"`
}

// qmd 兼容的默认模型 URI。
const (
	// DefaultModelDir 是默认本地模型目录。
	DefaultModelDir = ".wikimesh/models"

	// DefaultEmbedModel 是默认 embedding 模型来源。
	DefaultEmbedModel = "hf:Qwen/Qwen3-Embedding-0.6B-GGUF/Qwen3-Embedding-0.6B-Q8_0.gguf"

	// DefaultRerankModel 是默认 reranker 模型来源。
	DefaultRerankModel = "hf:ggml-org/Qwen3-Reranker-0.6B-Q8_0-GGUF/qwen3-reranker-0.6b-q8_0.gguf"

	// DefaultGenerateModel 是默认 query expansion/generate 模型来源。
	DefaultGenerateModel = "hf:tobil/qmd-query-expansion-1.7B-gguf/qmd-query-expansion-1.7B-q4_k_m.gguf"
)

// ModelsConfig 描述 qmd 配置中的 models.embed/rerank/generate 三类默认模型来源。
type ModelsConfig struct {
	// Embed 是 embedding 模型来源 URI 或本地路径。
	Embed string `yaml:"embed"`

	// Rerank 是 query reranker 模型来源 URI 或本地路径。
	Rerank string `yaml:"rerank"`

	// Generate 是 query expansion/generate 模型来源 URI 或本地路径。
	Generate string `yaml:"generate"`
}

// EmbeddingConfig 是 SDK 对外暴露的 embedding provider 配置。
// 调用方通过它选择云端 API、Ollama 或本地 GGUF embedding 后端。
type EmbeddingConfig struct {
	// Provider 是 embedding 提供方，例如 local、ollama、openai、gemini、voyage、mistral 或 none。
	Provider string `yaml:"provider,omitempty"`

	// Model 是 provider 使用的模型名；local 模式下是本地 GGUF 模型路径。
	Model string `yaml:"model,omitempty"`

	// Command 是 local 模式下可选的外部 embedding 命令路径；为空时使用内置 llama.cpp 后端。
	Command string `yaml:"command,omitempty"`

	// Dimensions 是向量维度；未知时可以为 0，部分 provider 会在首次请求后自动推断。
	Dimensions int `yaml:"dimensions,omitempty"`

	// APIKey 是云端或 OpenAI-compatible provider 的访问令牌。
	APIKey string `yaml:"api_key,omitempty"`

	// BaseURL 是 provider API 的自定义基础地址，主要用于 OpenAI-compatible 服务或自托管服务。
	BaseURL string `yaml:"base_url,omitempty"`

	// RateLimit 是每分钟最大请求数；0 表示不做本地限速。
	RateLimit int `yaml:"rate_limit,omitempty"`
}

func (c EmbeddingConfig) internalConfig() embed.Config {
	return embed.Config{
		Provider:   c.Provider,
		Model:      c.Model,
		Command:    c.Command,
		Dimensions: c.Dimensions,
		APIKey:     c.APIKey,
		BaseURL:    c.BaseURL,
		RateLimit:  c.RateLimit,
	}
}

// LlamaCppTextModelConfig 配置本地 llama.cpp 文本生成类模型。
// SDK 使用它创建 query expansion 和 reranker；Provider 为 none 或空时表示禁用对应能力。
type LlamaCppTextModelConfig struct {
	// Provider 是文本模型提供方；当前 SDK 内置支持 local，none 或空表示禁用。
	Provider string `yaml:"provider,omitempty"`

	// Model 是本地 GGUF 模型路径。
	Model string `yaml:"model,omitempty"`

	// Command 是可选的外部 llama-cli 兼容命令；为空时使用内置 yzma/llama.cpp 后端。
	Command string `yaml:"command,omitempty"`

	// MaxTokens 是单次生成的最大 token 数；小于等于 0 时按用途使用默认值。
	MaxTokens int `yaml:"max_tokens,omitempty"`
}

// Embedder 是向量模型的统一接口。
// API 服务、Ollama、本地 llama.cpp 和测试替身都实现这个接口。
type Embedder interface {
	// Embed 把文本转成 float32 向量。
	Embed(text string) ([]float32, error)

	// Dimensions 返回向量维度；未知时可以返回 0。
	Dimensions() int

	// Name 返回服务方和模型名称，主要用于日志和调试。
	Name() string
}

// QueryExpansionType 标识扩展查询的用途。
type QueryExpansionType string

const (
	// QueryExpansionLex 表示关键词检索扩展，VSearch 不使用。
	QueryExpansionLex QueryExpansionType = "lex"

	// QueryExpansionVec 表示语义向量检索扩展。
	QueryExpansionVec QueryExpansionType = "vec"

	// QueryExpansionHyDE 表示用假想答案文本扩展向量查询。
	QueryExpansionHyDE QueryExpansionType = "hyde"
)

// LexSearchOptions 控制 qmd searchLex 等价的 BM25 关键词检索。
type LexSearchOptions struct {
	// Limit 是最多返回多少条结果；小于等于 0 时使用 qmd FTS 默认 20。
	Limit int `json:"limit,omitempty"`

	// Collection 限定单个 collection；为空时搜索所有 collection。
	Collection string `json:"collection,omitempty"`
}

// VectorSearchOptions 控制 qmd searchVector 等价的原始向量检索。
type VectorSearchOptions struct {
	// Limit 是最多返回多少条结果；小于等于 0 时使用 qmd searchVec 默认 20。
	Limit int `json:"limit,omitempty"`

	// Collection 限定单个 collection；为空时搜索所有 collection。
	Collection string `json:"collection,omitempty"`

	// MinScore 是最低相似度阈值；0 表示不过滤。
	MinScore float64 `json:"minScore,omitempty"`
}

// ExpandQueryOptions 控制 qmd expandQuery 等价的手动查询扩展。
type ExpandQueryOptions struct {
	// Intent 是领域意图提示；当前 Go 注入式 QueryExpander 接口不强制消费该字段。
	Intent string `json:"intent,omitempty"`
}

// QueryExpansion 是生成模型产出的带类型查询。
type QueryExpansion struct {
	// Type 是扩展查询类型，决定进入 FTS、向量还是 HyDE 路径。
	Type QueryExpansionType `json:"type"`

	// Text 是 Go 侧使用的扩展文本字段，保留兼容现有调用。
	Text string `json:"text,omitempty"`

	// Query 是 qmd SDK JSON 使用的扩展文本字段。
	Query string `json:"query,omitempty"`

	// Line 是 qmd structured search 可选的来源行号，仅用于错误定位和 JSON 兼容。
	Line int `json:"line,omitempty"`
}

// QueryExpander 是查询扩展的最小接口。
// 调用方可以注入测试替身；未注入时 VSearch 只使用原始查询。
type QueryExpander interface {
	// Expand 基于原始查询生成带类型的查询变体。
	Expand(ctx context.Context, query string) ([]QueryExpansion, error)
}

// QueryExpanderWithOptions 是支持 intent 等扩展参数的可选接口。
// 实现该接口的 expander 会优先收到 ExpandQueryOptions；未实现时回退到 QueryExpander。
type QueryExpanderWithOptions interface {
	// ExpandWithOptions 基于原始查询和扩展参数生成带类型的查询变体。
	ExpandWithOptions(ctx context.Context, query string, opts ExpandQueryOptions) ([]QueryExpansion, error)
}

// QueryRerankDocument 是送入 query reranker 的单个候选 chunk。
type QueryRerankDocument struct {
	// ID 是稳定文档 ID。
	ID string

	// File 是 qmd://collection/path 形式的虚拟路径。
	File string

	// Text 是用于重排的最佳 chunk 文本，不是全文。
	Text string
}

// QueryRerankScore 是 reranker 对候选 chunk 的相关性评分。
type QueryRerankScore struct {
	// ID 是稳定文档 ID；优先用于回连候选。
	ID string

	// File 是 qmd://collection/path 形式的虚拟路径；当 ID 为空时用于回连候选。
	File string

	// Score 是 reranker 归一化分数，qmd 按 0..1 解释。
	Score float64
}

// QueryReranker 是 query 层可注入的重排接口。
type QueryReranker interface {
	// Rerank 根据问题和候选 chunk 返回相关性分数。
	Rerank(ctx context.Context, query string, docs []QueryRerankDocument) ([]QueryRerankScore, error)
}

// Collection 描述一组需要收集和查询的文档。
type Collection struct {
	// Name 是 collection 的唯一名称。
	Name string `yaml:"name"`

	// Path 是文档根目录，可以是相对路径或绝对路径。
	Path string `yaml:"path"`

	// Pattern 是 qmd 风格的单个 glob 规则；为空时使用 Include 或默认文本规则。
	Pattern string `yaml:"pattern"`

	// Include 是要收集的 glob 规则；为空时默认收集常见文本/Markdown 文件。
	Include []string `yaml:"include"`

	// Ignore 是要排除的 glob 规则；匹配后即使 Include 命中也不会入库。
	Ignore []string `yaml:"ignore"`

	// Update 是 qmd collection update-cmd，对应更新前可执行的外部命令。
	Update string `yaml:"update"`

	// IncludeByDefault 控制默认查询是否包含该 collection；nil 表示 qmd 默认 true。
	IncludeByDefault *bool `yaml:"includeByDefault"`

	// Context 保存 qmd path context，key 是路径前缀，value 是说明文本。
	Context map[string]string `yaml:"context"`

	// DocCount 是该 collection 历史索引过的文档总数。
	DocCount int `yaml:"-"`

	// ActiveCount 是该 collection 当前 active 文档数。
	ActiveCount int `yaml:"-"`

	// LastModified 是该 collection 文档状态最后更新时间。
	LastModified string `yaml:"-"`
}

// CollectionSettings 是 qmd collection show/update-cmd/include/exclude 等管理命令的可变字段。
type CollectionSettings struct {
	Update           *string
	IncludeByDefault *bool
}

// CollectionContext 是 ListContexts 的返回项。
type CollectionContext struct {
	Collection string
	Path       string
	Context    string
}

// BoolPtr 返回 bool 指针，用于区分未设置和显式 false。
func BoolPtr(v bool) *bool {
	return &v
}

// UpdateOptions 控制 collection update 行为。
type UpdateOptions struct {
	// RebuildVectors 保留兼容旧调用；collection update 不再生成向量。
	RebuildVectors bool

	// RunUpdateCommand 控制是否执行 collection 的 update 命令。
	// 顶层 update 默认只重建索引；需要对齐 qmd update --pull 时才启用。
	RunUpdateCommand bool

	// Progress 在每个文件处理后被调用，供 CLI 更新进度条。
	Progress func(UpdateProgress)
}

// UpdateProgress 描述 collection update 的当前进度。
type UpdateProgress struct {
	Current     int
	Total       int
	CurrentPath string
}

// UpdateResult 是 collection update 的统计结果。
type UpdateResult struct {
	// Scanned 是扫描到并参与判断的文件数量。
	Scanned int

	// Indexed 是新增或内容变化后重新写入索引的文件数量。
	Indexed int

	// Skipped 是 hash 未变化且无需重建向量的文件数量。
	Skipped int

	// Removed 是本次扫描发现已不存在并标记为 inactive 的文件数量。
	Removed int

	// Embedded 是成功写入向量的 chunk 数量。
	Embedded int
}

// StatusResult 描述 qmd 索引的整体状态。
type StatusResult struct {
	DBPath         string
	TotalDocuments int
	VectorCount    int
	NeedsEmbedding int
	Collections    []Collection
}

// EmbedOptions 控制已索引文档的向量生成行为。
type EmbedOptions struct {
	// Force 为 true 时，即使已有当前模型指纹的向量也会重新生成。
	Force bool

	// Progress 在每个文档处理后被调用，供 CLI 更新进度条。
	Progress func(EmbedProgress)
}

// EmbedProgress 描述 embed 命令的当前进度。
type EmbedProgress struct {
	Current     int
	Total       int
	Embedded    int
	CurrentPath string
}

// EmbedResult 是 embed 命令的统计结果。
type EmbedResult struct {
	// Scanned 是检查过的 active 文档数量。
	Scanned int

	// Skipped 是已有当前 embedding 指纹、无需重建的文档数量。
	Skipped int

	// Embedded 是成功写入向量的 chunk 数量。
	Embedded int
}

// SearchOptions 控制 search 和 vsearch 的返回规模。
type SearchOptions struct {
	// Limit 是最多返回多少条结果；小于等于 0 时 Search 默认 20，VSearch 默认 10。
	Limit int

	// MinScore 是最低分阈值；0 表示不过滤。
	MinScore float64

	// Intent 是语义检索意图提示；当前仅为 qmd 选项兼容保留。
	Intent string
}

// SearchResult 是统一的检索结果。
type SearchResult struct {
	// ID 是稳定文档 ID，由 collection 名和相对路径 hash 生成。
	ID string

	// Collection 是命中的 collection 名称。
	Collection string

	// Path 是文档相对 collection 根目录的路径。
	Path string

	// Title 是从 Markdown 标题或文件名提取出的标题。
	Title string

	// Snippet 是命中的片段，优先取 chunk 文本。
	Snippet string

	// ChunkID 是命中的 chunk ID；文档级命中时可能为空。
	ChunkID string

	// Score 是当前检索模式下的排序分。
	Score float64

	// BM25Rank 是文本检索中的排名；0 表示没有文本命中。
	BM25Rank int

	// VectorRank 是向量检索中的排名；0 表示没有向量命中。
	VectorRank int

	// File 是 qmd://collection/path 形式的虚拟路径，主要用于 query 对齐 qmd 输出。
	File string

	// Context 是 qmd path context 解析出的最具体上下文。
	Context string

	// DocID 是 qmd 风格短文档 ID，来自内容 hash 前缀。
	DocID string

	// BestChunk 是 query 送入 reranker 的最佳 chunk。
	BestChunk string

	// BestChunkPos 是 BestChunk 在全文中的 byte offset，对齐 qmd bestChunkPos。
	BestChunkPos int

	// Explain 保存 query 的 RRF 和 rerank 打分明细；未请求 Explain 时为空。
	Explain *QueryExplain
}

// QueryOptions 控制混合查询。
type QueryOptions struct {
	// Limit 是最多返回多少条上下文结果；小于等于 0 时默认 10。
	Limit int

	// MinScore 是最低融合分阈值；0 表示不过滤。
	MinScore float64

	// CandidateLimit 是送入 rerank 前的候选数；小于等于 0 时使用 qmd 默认 40。
	CandidateLimit int

	// Explain 为 true 时返回 qmd RRF/rerank 打分明细。
	Explain bool

	// SkipRerank 为 true 时跳过 reranker，按 qmd no-rerank 路径使用 RRF 位置分。
	SkipRerank bool

	// Intent 是领域意图提示；有值时不使用 strong BM25 shortcut。
	Intent string

	// Queries 是预先展开的 typed queries；设置后跳过自动 QueryExpander。
	Queries []QueryExpansion

	// SearchQueries 是调用方显式提供的辅助关键词 query 列表。
	// 主问题仍走原始 FTS/Vector；这些 query 只做 FTS 召回并通过 RRF 融合，设置后跳过自动 QueryExpander。
	SearchQueries []string
}

// QueryResult 是 Query 的返回值。
type QueryResult struct {
	// Question 是调用方传入的问题。
	Question string

	// Answer 是可选的最终回答；当前没有配置 LLM 时保持为空。
	Answer string

	// Results 是用于回答问题的检索上下文。
	Results []SearchResult
}

// QueryContributionTrace 描述某个检索列表对单个文档的 RRF 贡献。
type QueryContributionTrace struct {
	// ListIndex 是第几个检索列表。
	ListIndex int

	// Source 是检索来源：fts 或 vec。
	Source string

	// QueryType 是 qmd typed query 类型：original、lex、vec 或 hyde。
	QueryType string

	// Query 是该列表实际执行的查询文本。
	Query string

	// Rank 是该文档在列表内的 1-based 排名。
	Rank int

	// Weight 是该列表权重；qmd 中 original FTS/vec 为 2.0，其余为 1.0。
	Weight float64

	// BackendScore 是后端检索器原始归一化分。
	BackendScore float64

	// RRFContribution 是 weight / (60 + rank)。
	RRFContribution float64
}

// QueryRRFExplain 是 query 的 RRF 解释信息。
type QueryRRFExplain struct {
	// Rank 是 RRF 融合后的 1-based 排名。
	Rank int

	// TopRank 是该文档在所有后端列表中出现过的最佳 1-based 排名。
	TopRank int

	// PositionScore 是 qmd no-rerank 和 blend 使用的 1/rank。
	PositionScore float64

	// Weight 是 position-aware blend 中 RRF 侧权重。
	Weight float64

	// BaseScore 是所有 RRF contribution 的和。
	BaseScore float64

	// TopRankBonus 是 qmd top-rank bonus：rank1 +0.05，rank2-3 +0.02。
	TopRankBonus float64

	// TotalScore 是 BaseScore + TopRankBonus。
	TotalScore float64

	// Contributions 是每个后端列表的贡献明细。
	Contributions []QueryContributionTrace
}

// QueryExplain 是 query 结果的打分解释。
type QueryExplain struct {
	// FTSScores 是该文档来自 FTS 列表的后端分。
	FTSScores []float64

	// VectorScores 是该文档来自向量列表的后端分。
	VectorScores []float64

	// RRF 是融合阶段解释。
	RRF QueryRRFExplain

	// RerankScore 是 reranker 返回分；跳过 rerank 时为 0。
	RerankScore float64

	// BlendedScore 是最终输出分。
	BlendedScore float64
}
