package embed

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// LlamaCppConfig 是本地 GGUF embedding 的配置。
type LlamaCppConfig struct {
	// ProviderName 是对外展示的 provider 名称。
	ProviderName string

	// Command 是 llama.cpp embedding 可执行文件路径；为空时使用 PATH 中的 llama-embedding。
	Command string

	// ModelPath 是已经下载好的 GGUF 模型文件路径。
	ModelPath string

	// Dimensions 是向量维度；为 0 时由模型返回结果推断。
	Dimensions int
}

// LlamaCppEmbedder 通过显式外部命令调用 GGUF embedding 模型。
type LlamaCppEmbedder struct {
	providerName string
	command      string
	modelPath    string
	dims         int
	mu           sync.Mutex
}

// NewLlamaCppEmbedder 创建本地 GGUF embedding provider。
// 默认使用 yzma 动态加载 llama.cpp；只有显式配置 command 时才调用外部 llama-embedding。
func NewLlamaCppEmbedder(cfg LlamaCppConfig) (Embedder, error) {
	if strings.TrimSpace(cfg.ModelPath) == "" {
		return nil, fmt.Errorf("local embedding model path is required")
	}
	providerName := strings.TrimSpace(cfg.ProviderName)
	if providerName == "" {
		providerName = "local"
	}
	command := strings.TrimSpace(cfg.Command)
	if command == "" {
		return newGoLlamaCppEmbedder(cfg, providerName)
	}
	return &LlamaCppEmbedder{
		providerName: providerName,
		command:      command,
		modelPath:    cfg.ModelPath,
		dims:         cfg.Dimensions,
	}, nil
}

// Name 返回 provider 和模型路径。
func (e *LlamaCppEmbedder) Name() string {
	return e.providerName + "/" + e.modelPath
}

// Dimensions 返回向量维度。
func (e *LlamaCppEmbedder) Dimensions() int {
	return e.dims
}

// Embed 调用 llama.cpp embedding CLI。
func (e *LlamaCppEmbedder) Embed(text string) ([]float32, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("local embed: empty text")
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	cmd := newEmbeddingCommand(
		e.command,
		"-m", e.modelPath,
		"-p", text,
		"--embd-output-format", "json",
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("local embed command %q failed: %s", e.command, msg)
	}
	vec, err := parseLlamaCppEmbedding(out)
	if err != nil {
		return nil, err
	}
	if e.dims == 0 {
		e.dims = len(vec)
	}
	return vec, nil
}

func parseLlamaCppEmbedding(out []byte) ([]float32, error) {
	payload, err := extractJSONPayload(bytes.TrimSpace(out))
	if err != nil {
		return nil, err
	}

	var object struct {
		Embedding  []float32   `json:"embedding"`
		Embeddings [][]float32 `json:"embeddings"`
		Data       []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &object); err == nil {
		switch {
		case len(object.Embedding) > 0:
			return object.Embedding, nil
		case len(object.Embeddings) > 0 && len(object.Embeddings[0]) > 0:
			return object.Embeddings[0], nil
		case len(object.Data) > 0 && len(object.Data[0].Embedding) > 0:
			return object.Data[0].Embedding, nil
		}
	}

	var rows []struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.Unmarshal(payload, &rows); err == nil && len(rows) > 0 && len(rows[0].Embedding) > 0 {
		return rows[0].Embedding, nil
	}

	var matrix [][]float32
	if err := json.Unmarshal(payload, &matrix); err == nil && len(matrix) > 0 && len(matrix[0]) > 0 {
		return matrix[0], nil
	}

	var vector []float32
	if err := json.Unmarshal(payload, &vector); err == nil && len(vector) > 0 {
		return vector, nil
	}

	return nil, fmt.Errorf("local embed: empty embedding in output")
}

func extractJSONPayload(out []byte) ([]byte, error) {
	if len(out) == 0 {
		return nil, fmt.Errorf("local embed: empty output")
	}
	startObj := bytes.IndexByte(out, '{')
	startArr := bytes.IndexByte(out, '[')
	start := startObj
	if start < 0 || (startArr >= 0 && startArr < start) {
		start = startArr
	}
	if start < 0 {
		return nil, fmt.Errorf("local embed: no JSON output: %s", string(out))
	}
	endObj := bytes.LastIndexByte(out, '}')
	endArr := bytes.LastIndexByte(out, ']')
	end := endObj
	if endArr > end {
		end = endArr
	}
	if end < start {
		return nil, fmt.Errorf("local embed: malformed JSON output: %s", string(out))
	}
	return out[start : end+1], nil
}
