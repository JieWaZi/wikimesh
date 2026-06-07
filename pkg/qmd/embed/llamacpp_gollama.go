package embed

import (
	"fmt"
	"math"
	"os"
	"strings"
	"sync"

	"github.com/JieWaZi/wikimesh/pkg/qmd/llamaruntime"
	"github.com/hybridgroup/yzma/pkg/llama"
)

type goLlamaCppEmbedder struct {
	// providerName 是对外展示的 provider 名称。
	providerName string
	// modelPath 是本地 GGUF embedding 模型路径。
	modelPath string
	// model 是 yzma 加载后的 GGUF 模型句柄。
	model llama.Model
	// context 是启用 embedding 模式的 llama.cpp 推理上下文。
	context llama.Context
	// vocab 是模型 tokenizer 词表句柄。
	vocab llama.Vocab
	// dims 是向量维度；未知时由首次输出推断。
	dims int
	// mu 串行化同一个 llama.cpp 上下文的 embedding 调用。
	mu sync.Mutex
}

// newGoLlamaCppEmbedder 创建懒加载的 yzma 本地 GGUF embedding provider。
func newGoLlamaCppEmbedder(cfg LlamaCppConfig, providerName string) (Embedder, error) {
	return &goLlamaCppEmbedder{
		providerName: providerName,
		modelPath:    cfg.ModelPath,
		dims:         cfg.Dimensions,
	}, nil
}

// Name 返回 provider 和模型路径。
func (e *goLlamaCppEmbedder) Name() string {
	return e.providerName + "/" + e.modelPath
}

// Dimensions 返回当前已知向量维度。
func (e *goLlamaCppEmbedder) Dimensions() int {
	return e.dims
}

// Embed 使用 yzma/llama.cpp 直接生成文本向量。
func (e *goLlamaCppEmbedder) Embed(text string) ([]float32, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("local embed: empty text")
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := e.loadLocked(); err != nil {
		return nil, err
	}

	tokens := llama.Tokenize(e.vocab, text, true, true)
	if len(tokens) == 0 {
		return nil, fmt.Errorf("local embedding yzma backend: empty tokenized input")
	}
	if memory, err := llama.GetMemory(e.context); err == nil {
		_ = llama.MemoryClear(memory, true)
	}
	batch := llama.BatchGetOne(tokens)
	if _, err := llama.Decode(e.context, batch); err != nil {
		return nil, fmt.Errorf("local embedding yzma backend: decode %s: %w", e.modelPath, err)
	}
	nEmbd := llama.ModelNEmbd(e.model)
	vec, err := llama.GetEmbeddingsSeq(e.context, 0, nEmbd)
	if err != nil {
		return nil, fmt.Errorf("local embedding yzma backend: read embedding: %w", err)
	}
	if len(vec) == 0 {
		return nil, fmt.Errorf("local embedding yzma backend: empty embedding")
	}
	out := make([]float32, len(vec))
	copy(out, vec)
	normalizeEmbedding(out)
	if e.dims == 0 {
		e.dims = len(out)
	}
	return out, nil
}

// loadLocked 初始化 yzma 动态库、模型和 embedding 上下文；调用方必须持有 e.mu。
func (e *goLlamaCppEmbedder) loadLocked() error {
	if e.model != 0 && e.context != 0 {
		return nil
	}
	if e.modelPath == "" {
		return fmt.Errorf("local embedding yzma backend requires model path")
	}
	if err := llamaruntime.EnsureLoaded(); err != nil {
		return fmt.Errorf("local embedding yzma backend: %w; configure embedding.command only for an explicit external fallback", err)
	}
	modelParams := llama.ModelDefaultParams()
	modelParams.NGpuLayers = defaultYzmaGPULayers()
	model, err := llama.ModelLoadFromFile(e.modelPath, modelParams)
	if err != nil {
		return fmt.Errorf("local embedding yzma backend: load %s: %w", e.modelPath, err)
	}
	contextParams := llama.ContextDefaultParams()
	contextParams.NCtx = 2048
	contextParams.NBatch = 2048
	contextParams.NUbatch = 512
	contextParams.NSeqMax = 1
	contextParams.PoolingType = llama.PoolingTypeMean
	contextParams.AttentionType = llama.AttentionTypeUnspecified
	contextParams.FlashAttentionType = llama.FlashAttentionTypeAuto
	contextParams.Embeddings = 1
	contextParams.Offload_kqv = 1
	contextParams.KVUnified = 1
	context, err := llama.InitFromModel(model, contextParams)
	if err != nil {
		_ = llama.ModelFree(model)
		return fmt.Errorf("local embedding yzma backend: create context for %s: %w", e.modelPath, err)
	}
	e.model = model
	e.context = context
	e.vocab = llama.ModelGetVocab(model)
	return nil
}

// Close 释放当前 embedder 持有的模型和上下文。
func (e *goLlamaCppEmbedder) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.context != 0 {
		_ = llama.Free(e.context)
		e.context = 0
	}
	if e.model != 0 {
		_ = llama.ModelFree(e.model)
		e.model = 0
	}
	return nil
}

// defaultYzmaGPULayers 按 qmd 兼容环境变量决定 GPU offload 层数。
func defaultYzmaGPULayers() int32 {
	if isForceCPUEnv(stdEnv("WIKIMESH_FORCE_CPU")) || isForceCPUEnv(stdEnv("QMD_FORCE_CPU")) {
		return 0
	}
	if isGPUDisabledEnv(stdEnv("WIKIMESH_LLAMA_GPU")) || isGPUDisabledEnv(stdEnv("QMD_LLAMA_GPU")) {
		return 0
	}
	return -1
}

// normalizeEmbedding 对向量做 L2 归一化，便于余弦相似度检索。
func normalizeEmbedding(vec []float32) {
	var sum float64
	for _, v := range vec {
		sum += float64(v) * float64(v)
	}
	if sum <= 0 {
		return
	}
	scale := float32(1 / math.Sqrt(sum))
	for i := range vec {
		vec[i] *= scale
	}
}

// isForceCPUEnv 判断环境变量是否请求强制 CPU，语义对齐 qmd 的 QMD_FORCE_CPU。
func isForceCPUEnv(value string) bool {
	normalized := strings.TrimSpace(strings.ToLower(value))
	if normalized == "" {
		return false
	}
	return !isGPUDisabledValue(normalized)
}

// isGPUDisabledEnv 判断环境变量是否请求关闭 GPU。
func isGPUDisabledEnv(value string) bool {
	return isGPUDisabledValue(strings.TrimSpace(strings.ToLower(value)))
}

// isGPUDisabledValue 判断标准化后的字符串是否表示关闭 GPU。
func isGPUDisabledValue(value string) bool {
	switch value {
	case "false", "off", "none", "disable", "disabled", "0":
		return true
	default:
		return false
	}
}

// stdEnv 读取环境变量并统一做空白裁剪。
func stdEnv(name string) string {
	return strings.TrimSpace(os.Getenv(name))
}
