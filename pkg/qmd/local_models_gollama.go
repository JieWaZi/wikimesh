package qmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/JieWaZi/wikimesh/pkg/qmd/llamaruntime"
	"github.com/hybridgroup/yzma/pkg/llama"
)

type goLlamaCppTextGenerator struct {
	// purpose 标识当前文本模型用途，用于错误信息。
	purpose string
	// model 是 yzma 加载后的 GGUF 模型句柄。
	model llama.Model
	// context 是 llama.cpp 文本生成上下文。
	context llama.Context
	// vocab 是模型 tokenizer 词表句柄。
	vocab llama.Vocab
	// sampler 是 greedy 采样链，保证 query expansion/reranker 输出稳定。
	sampler llama.Sampler
	// mu 串行化 llama.cpp 推理调用，避免同一上下文并发访问。
	mu sync.Mutex
}

// newGoLlamaCppTextGenerator 创建懒加载的 yzma 文本生成器。
func newGoLlamaCppTextGenerator(cfg LlamaCppTextModelConfig, purpose string) localTextGenerator {
	modelPath := strings.TrimSpace(cfg.Model)
	return &lazyGoLlamaCppTextGenerator{cfg: cfg, purpose: purpose, modelPath: modelPath}
}

type lazyGoLlamaCppTextGenerator struct {
	// cfg 保存文本模型配置。
	cfg LlamaCppTextModelConfig
	// purpose 标识当前文本模型用途。
	purpose string
	// modelPath 是本地 GGUF 模型文件路径。
	modelPath string
	// model 是首次调用后缓存的生成器实例。
	model *goLlamaCppTextGenerator
	// mu 保护懒加载过程。
	mu sync.Mutex
}

// Generate 在首次调用时加载模型，然后执行文本生成。
func (g *lazyGoLlamaCppTextGenerator) Generate(ctx context.Context, prompt string, maxTokens int) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	model, err := g.load()
	if err != nil {
		return "", err
	}
	return model.Generate(ctx, prompt, maxTokens)
}

// load 负责把 GGUF 模型加载成 yzma 文本生成上下文。
func (g *lazyGoLlamaCppTextGenerator) load() (*goLlamaCppTextGenerator, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.model != nil {
		return g.model, nil
	}
	if g.modelPath == "" {
		return nil, fmt.Errorf("local text yzma backend for %s requires model path", g.purpose)
	}
	if err := llamaruntime.EnsureLoaded(); err != nil {
		return nil, fmt.Errorf("local text yzma backend for %s: %w; configure command only for an explicit external fallback", g.purpose, err)
	}
	modelParams := llama.ModelDefaultParams()
	modelParams.NGpuLayers = defaultTextYzmaGPULayers()
	model, err := llama.ModelLoadFromFile(g.modelPath, modelParams)
	if err != nil {
		return nil, fmt.Errorf("local text yzma backend for %s: load %s: %w", g.purpose, g.modelPath, err)
	}
	contextParams := llama.ContextDefaultParams()
	contextParams.NCtx = textContextSize(g.purpose)
	contextParams.NBatch = contextParams.NCtx
	contextParams.NUbatch = 512
	contextParams.NSeqMax = 1
	context, err := llama.InitFromModel(model, contextParams)
	if err != nil {
		_ = llama.ModelFree(model)
		return nil, fmt.Errorf("local text yzma backend for %s: create context for %s: %w", g.purpose, g.modelPath, err)
	}
	sampler := llama.SamplerChainInit(llama.SamplerChainDefaultParams())
	llama.SamplerChainAdd(sampler, llama.SamplerInitGreedy())
	g.model = &goLlamaCppTextGenerator{
		purpose: g.purpose,
		model:   model,
		context: context,
		vocab:   llama.ModelGetVocab(model),
		sampler: sampler,
	}
	return g.model, nil
}

// Generate 调用 yzma/llama.cpp 输出短文本结果。
func (g *goLlamaCppTextGenerator) Generate(ctx context.Context, prompt string, maxTokens int) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	if maxTokens <= 0 {
		maxTokens = 128
	}
	tokens := llama.Tokenize(g.vocab, prompt, true, true)
	if len(tokens) == 0 {
		return "", fmt.Errorf("local text yzma backend for %s: empty tokenized prompt", g.purpose)
	}
	if memory, err := llama.GetMemory(g.context); err == nil {
		_ = llama.MemoryClear(memory, true)
	}
	llama.SamplerReset(g.sampler)
	batch := llama.BatchGetOne(tokens)
	var out strings.Builder
	for generated := 0; generated < maxTokens; generated++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if _, err := llama.Decode(g.context, batch); err != nil {
			return "", fmt.Errorf("local text yzma backend for %s: decode: %w", g.purpose, err)
		}
		token := llama.SamplerSample(g.sampler, g.context, -1)
		if llama.VocabIsEOG(g.vocab, token) {
			break
		}
		llama.SamplerAccept(g.sampler, token)
		buf := make([]byte, 256)
		n := llama.TokenToPiece(g.vocab, token, buf, 0, false)
		if n > 0 {
			out.Write(buf[:n])
		}
		batch = llama.BatchGetOne([]llama.Token{token})
	}
	return strings.TrimSpace(out.String()), nil
}

// textContextSize 按用途设置 qmd 对齐的上下文大小。
func textContextSize(purpose string) uint32 {
	if strings.Contains(purpose, "reranker") {
		return 4096
	}
	return 2048
}

// defaultTextYzmaGPULayers 按 qmd 兼容环境变量决定 GPU offload 层数。
func defaultTextYzmaGPULayers() int32 {
	if textForceCPUEnv(os.Getenv("WIKIMESH_FORCE_CPU")) || textForceCPUEnv(os.Getenv("QMD_FORCE_CPU")) {
		return 0
	}
	if textGPUDisabledEnv(os.Getenv("WIKIMESH_LLAMA_GPU")) || textGPUDisabledEnv(os.Getenv("QMD_LLAMA_GPU")) {
		return 0
	}
	return -1
}

// textForceCPUEnv 判断环境变量是否请求强制 CPU，语义对齐 qmd 的 QMD_FORCE_CPU。
func textForceCPUEnv(value string) bool {
	normalized := strings.TrimSpace(strings.ToLower(value))
	if normalized == "" {
		return false
	}
	return !textGPUDisabledValue(normalized)
}

// textGPUDisabledEnv 判断环境变量是否请求关闭 GPU。
func textGPUDisabledEnv(value string) bool {
	return textGPUDisabledValue(strings.TrimSpace(strings.ToLower(value)))
}

// textGPUDisabledValue 判断标准化后的字符串是否表示关闭 GPU。
func textGPUDisabledValue(value string) bool {
	switch value {
	case "false", "off", "none", "disable", "disabled", "0":
		return true
	default:
		return false
	}
}
