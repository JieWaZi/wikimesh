package embed

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/JieWaZi/wikimesh/pkg/qmd/qmdlog"
)

// Embedder 把文本转换成向量。
type Embedder interface {
	// Embed 返回输入文本的语义向量。
	Embed(text string) ([]float32, error)
	// Dimensions 返回向量维度。
	Dimensions() int
	// Name 返回 provider/model 名称。
	Name() string
}

// Config 是文档检索使用的 embedding 配置。
// 原始实现的配置入口比较大；这里只保留检索需要的字段，
// 让调用方可以直接在 collection 配置里选择 API、Ollama 或本地 GGUF。
type Config struct {
	Provider   string `yaml:"provider,omitempty"`
	Model      string `yaml:"model,omitempty"`
	Command    string `yaml:"command,omitempty"`
	Dimensions int    `yaml:"dimensions,omitempty"`
	APIKey     string `yaml:"api_key,omitempty"`
	BaseURL    string `yaml:"base_url,omitempty"`
	RateLimit  int    `yaml:"rate_limit,omitempty"`
}

// 各 provider 的默认 embedding 模型。
var defaultModels = map[string]string{
	"openai":  "text-embedding-3-small",
	"gemini":  "gemini-embedding-2-preview",
	"voyage":  "voyage-3-lite",
	"mistral": "mistral-embed",
}

// 各模型的默认向量维度。
var defaultDimensions = map[string]int{
	"text-embedding-3-small":     1536,
	"gemini-embedding-2-preview": 768,
	"voyage-3-lite":              1024,
	"mistral-embed":              1024,
	"nomic-embed-text":           768,
}

// EmbedOverride 是显式 provider 配置。
type EmbedOverride struct {
	Provider   string // Provider 是 openai、gemini、ollama、local 等。
	Model      string // Model 是模型名；local 模式下是 GGUF 路径。
	Command    string // Command 是 local 模式下的 embedding 可执行文件路径。
	Dimensions int    // Dimensions 是向量维度，未知时可为 0。
	APIKey     string // APIKey 是云端 provider 的访问令牌。
	BaseURL    string // BaseURL 是 OpenAI-compatible 服务地址。
	RateLimit  int    // RateLimit 是每分钟请求数限制，0 表示不限制。
}

// NewFromConfig 根据配置创建 Embedder。
// 这里保留“逐级选择 provider”的策略：显式配置优先，其次 API provider，
// 最后探测本地 Ollama；本项目额外支持 local GGUF provider。
func NewFromConfig(cfg Config) Embedder {
	if cfg.Provider == "" || cfg.Provider == "none" {
		return nil
	}
	return NewCascade(cfg.Provider, cfg.APIKey, cfg.BaseURL, &EmbedOverride{
		Provider:   cfg.Provider,
		Model:      cfg.Model,
		Command:    cfg.Command,
		Dimensions: cfg.Dimensions,
		APIKey:     cfg.APIKey,
		BaseURL:    cfg.BaseURL,
		RateLimit:  cfg.RateLimit,
	})
}

// NewCascade 按配置选择最合适的 embedding provider。
// 选择顺序是：local -> ollama -> 显式 API 配置 -> provider 默认 API -> 自动探测 Ollama。
func NewCascade(provider string, apiKey string, baseURL string, override *EmbedOverride) Embedder {
	if provider == "local" || (override != nil && override.Provider == "local") {
		if override == nil || override.Model == "" {
			qmdlog.Warn("local embedding requires embedding.model to point to a GGUF model")
			return nil
		}
		embedder, err := NewLlamaCppEmbedder(LlamaCppConfig{
			ProviderName: "local",
			Command:      override.Command,
			ModelPath:    override.Model,
			Dimensions:   override.Dimensions,
		})
		if err != nil {
			qmdlog.Warn("local embedding unavailable", "error", err)
			return nil
		}
		return embedder
	}
	if provider == "ollama" || (override != nil && override.Provider == "ollama") {
		model := "nomic-embed-text"
		if override != nil && override.Model != "" {
			model = override.Model
		}
		dims := defaultDimensions[model]
		if override != nil && override.Dimensions > 0 {
			dims = override.Dimensions
		}
		rateLimit := 0
		if override != nil {
			rateLimit = override.RateLimit
		}
		return &OllamaEmbedder{
			model:   model,
			dims:    dims,
			baseURL: overrideBaseURL(override),
			client:  newEmbedHTTPClient(),
			limiter: newEmbedLimiter(rateLimit),
		}
	}

	// 第 0 层：用户显式配置了模型或凭据。
	if override != nil && override.Model != "" {
		p := override.Provider
		if p == "" {
			p = provider
		}
		key := override.APIKey
		if key == "" {
			key = apiKey
		}
		url := override.BaseURL
		if url == "" {
			url = baseURL
		}
		if key != "" {
			dims := override.Dimensions
			if dims == 0 {
				dims = defaultDimensions[override.Model]
			}
			// 未知模型的维度可能为 0，会从第一次响应中自动推断。
			embedder := &APIEmbedder{
				provider: p,
				model:    override.Model,
				apiKey:   key,
				baseURL:  url,
				dims:     dims,
				client:   newEmbedHTTPClient(),
				limiter:  newEmbedLimiter(override.RateLimit),
			}
			if dims > 0 {
				qmdlog.Info("embedding provider detected", "tier", 0, "provider", p, "model", override.Model, "dims", dims)
			} else {
				qmdlog.Info("embedding provider detected", "tier", 0, "provider", p, "model", override.Model, "dims", "auto-detect")
			}
			return embedder
		}
	}

	// 计算后续层级使用的限速配置。
	var rateLimit int
	if override != nil {
		rateLimit = override.RateLimit
	}

	// 第 1 层：provider 的 embedding API。
	if model, ok := defaultModels[provider]; ok && apiKey != "" {
		dims := defaultDimensions[model]
		embedder := &APIEmbedder{
			provider: provider,
			model:    model,
			apiKey:   apiKey,
			baseURL:  baseURL,
			dims:     dims,
			client:   newEmbedHTTPClient(),
			limiter:  newEmbedLimiter(rateLimit),
		}
		qmdlog.Info("embedding provider detected", "tier", 1, "provider", provider, "model", model, "dims", dims)
		return embedder
	}

	// 第 2 层：本地 Ollama。
	if ollamaAvailable() {
		qmdlog.Info("embedding provider detected", "tier", 2, "provider", "ollama", "model", "nomic-embed-text", "dims", 768)
		return &OllamaEmbedder{
			model:   "nomic-embed-text",
			dims:    768,
			baseURL: "",
			client:  newEmbedHTTPClient(),
			limiter: newEmbedLimiter(rateLimit),
		}
	}

	qmdlog.Warn("no embedding provider available — vector search disabled. Install Ollama or configure an embedding-capable provider.")
	return nil
}

// sharedEmbedTransport 是进程级共享 HTTP transport，用于 embedding API 调用。
// 它覆盖 http.DefaultTransport 的 MaxIdleConnsPerHost=2，避免大量并发 Embed 调用
// 打到同一个 embedding endpoint 时频繁创建 TCP/TLS 连接；逻辑对齐 sharedTransport。
var sharedEmbedTransport http.RoundTripper = func() http.RoundTripper {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.MaxIdleConns = 512
	tr.MaxIdleConnsPerHost = 256
	tr.MaxConnsPerHost = 0
	tr.IdleConnTimeout = 90 * time.Second
	return tr
}()

// newEmbedHTTPClient 返回使用 sharedEmbedTransport 的标准库 http.Client。
func newEmbedHTTPClient() http.Client {
	return http.Client{
		Transport: sharedEmbedTransport,
		Timeout:   120 * time.Second,
	}
}

// APIEmbedder 调用云端或 OpenAI-compatible embedding API。
type APIEmbedder struct {
	provider string            // provider 标识，用于选择 API URL 和响应解析方式。
	model    string            // model 是请求体里的 embedding 模型名。
	apiKey   string            // apiKey 是 HTTP 鉴权 token。
	baseURL  string            // baseURL 允许接入自建 OpenAI-compatible 服务。
	dims     int               // dims 是向量维度，可能从第一次响应自动推断。
	client   http.Client       // client 使用共享连接池，减少大量 embedding 请求时的连接开销。
	limiter  *embedRateLimiter // limiter 用于本地限速，避免触发 provider 429。
}

func (e *APIEmbedder) Name() string    { return fmt.Sprintf("%s/%s", e.provider, e.model) }
func (e *APIEmbedder) Dimensions() int { return e.dims }

// maxEmbedChars 是 OpenAI-compatible embedding endpoint 的单次输入上限，单位为 rune。
// 5000 rune 约等于 4K token，为常见 8K token 限制保留余量。
// 更长文本会拆分后分别向量化，再做均值池化得到单个文档向量。
const maxEmbedChars = 5000

func (e *APIEmbedder) Embed(text string) ([]float32, error) {
	// 防御性检查：部分 OpenAI-compatible embedding endpoint 会拒绝空白输入。
	// chunker 应负责不产出这种 chunk；这里保护其他调用方。
	// 如果切片后仍出现空输入，应显式报错暴露上游问题，而不是用零向量掩盖。
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("embed: empty or whitespace-only input text")
	}
	if e.limiter != nil {
		e.limiter.wait()
	}
	if e.provider == "gemini" {
		return retryableEmbed(func() ([]float32, error) {
			return e.embedGemini(text)
		})
	}
	runes := []rune(text)
	if len(runes) <= maxEmbedChars {
		return retryableEmbed(func() ([]float32, error) {
			return e.embedOpenAI(text)
		})
	}
	return e.embedOpenAILong(runes)
}

// embedOpenAILong 将超长输入按 rune 边界拆分、分别向量化，
// 再对结果做均值池化，使下游仍收到单个固定维度的文档向量。
func (e *APIEmbedder) embedOpenAILong(runes []rune) ([]float32, error) {
	var pooled []float32
	chunks := 0
	for i := 0; i < len(runes); i += maxEmbedChars {
		end := i + maxEmbedChars
		if end > len(runes) {
			end = len(runes)
		}
		seg := string(runes[i:end])
		// 按 rune 对齐的切分点可能落在连续空白中，例如章节间空行。
		// 这种全空白片段对均值池化没有贡献，还可能被 bge-m3 拒绝，因此跳过。
		if strings.TrimSpace(seg) == "" {
			continue
		}
		vec, err := retryableEmbed(func() ([]float32, error) {
			return e.embedOpenAI(seg)
		})
		if err != nil {
			return nil, fmt.Errorf("embed: chunk %d/%d: %w", chunks+1, (len(runes)+maxEmbedChars-1)/maxEmbedChars, err)
		}
		if pooled == nil {
			pooled = make([]float32, len(vec))
		} else if len(vec) != len(pooled) {
			return nil, fmt.Errorf("embed: inconsistent dimensions across chunks: %d vs %d", len(vec), len(pooled))
		}
		for j, v := range vec {
			pooled[j] += v
		}
		chunks++
	}
	if chunks == 0 {
		return nil, fmt.Errorf("embed: no chunks produced from input")
	}
	inv := 1.0 / float32(chunks)
	for j := range pooled {
		pooled[j] *= inv
	}
	qmdlog.Info("embed: mean-pooled long input", "model", e.model, "chunks", chunks, "chars", len(runes))
	return pooled, nil
}

// embedOpenAI 使用 OpenAI-compatible /embeddings endpoint。
func (e *APIEmbedder) embedOpenAI(text string) ([]float32, error) {
	url := e.embeddingURL()

	body, _ := json.Marshal(map[string]any{
		"model": e.model,
		"input": text,
	})

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("embed: create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, &embedHTTPError{
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
			RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
		}
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("embed: decode response: %w", err)
	}

	if len(result.Data) == 0 || len(result.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("embed: empty embedding in response")
	}

	embedding := result.Data[0].Embedding

	// 从第一次响应自动推断向量维度。
	if e.dims == 0 {
		e.dims = len(embedding)
		qmdlog.Info("auto-detected embedding dimensions", "model", e.model, "dims", e.dims)
	}

	return embedding, nil
}

// embedGemini 使用 Gemini 原生 /models/{model}:embedContent endpoint。
func (e *APIEmbedder) embedGemini(text string) ([]float32, error) {
	base := e.baseURL
	if base == "" {
		base = "https://generativelanguage.googleapis.com/v1beta"
	}
	url := fmt.Sprintf("%s/models/%s:embedContent?key=%s", base, e.model, e.apiKey)

	body, _ := json.Marshal(map[string]any{
		"model": "models/" + e.model,
		"content": map[string]any{
			"parts": []map[string]string{
				{"text": text},
			},
		},
	})

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("embed: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, &embedHTTPError{
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
			RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
		}
	}

	var result struct {
		Embedding struct {
			Values []float32 `json:"values"`
		} `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("embed: decode response: %w", err)
	}

	if len(result.Embedding.Values) == 0 {
		return nil, fmt.Errorf("embed: empty embedding in response")
	}

	e.dims = len(result.Embedding.Values)
	return result.Embedding.Values, nil
}

func (e *APIEmbedder) embeddingURL() string {
	base := e.baseURL
	if base == "" {
		switch e.provider {
		case "openai":
			base = "https://api.openai.com/v1"
		case "voyage":
			base = "https://api.voyageai.com/v1"
		case "mistral":
			base = "https://api.mistral.ai/v1"
		}
	}
	return base + "/embeddings"
}

// OllamaEmbedder 使用本地 Ollama 实例。
type OllamaEmbedder struct {
	model   string
	dims    int
	baseURL string
	client  http.Client
	limiter *embedRateLimiter
}

func (e *OllamaEmbedder) Name() string    { return fmt.Sprintf("ollama/%s", e.model) }
func (e *OllamaEmbedder) Dimensions() int { return e.dims }

func (e *OllamaEmbedder) Embed(text string) ([]float32, error) {
	if e.limiter != nil {
		e.limiter.wait()
	}
	return retryableEmbed(func() ([]float32, error) {
		return e.embedOllama(text)
	})
}

func (e *OllamaEmbedder) embedOllama(text string) ([]float32, error) {
	body, _ := json.Marshal(map[string]any{
		"model":  e.model,
		"prompt": text,
	})

	resp, err := e.client.Post(e.embeddingURL(), "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama embed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, &embedHTTPError{
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
			RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
		}
	}

	var result struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("ollama embed: decode: %w", err)
	}

	if len(result.Embedding) == 0 {
		return nil, fmt.Errorf("ollama embed: empty embedding")
	}

	e.dims = len(result.Embedding)
	return result.Embedding, nil
}

func (e *OllamaEmbedder) embeddingURL() string {
	base := strings.TrimRight(strings.TrimSpace(e.baseURL), "/")
	if base == "" {
		base = "http://localhost:11434"
	}
	return base + "/api/embeddings"
}

func overrideBaseURL(override *EmbedOverride) string {
	if override == nil {
		return ""
	}
	return override.BaseURL
}

// embedHTTPError 保存失败 embedding 请求的 HTTP 状态和重试元数据。
type embedHTTPError struct {
	StatusCode int
	Body       string
	RetryAfter time.Duration
}

func (e *embedHTTPError) Error() string {
	return fmt.Sprintf("embed: API returned %d: %s", e.StatusCode, e.Body)
}

func isRetryableStatus(code int) bool {
	return code == 429 || code == 503
}

func parseRetryAfter(header string) time.Duration {
	if header == "" {
		return 0
	}
	if secs, err := strconv.Atoi(header); err == nil {
		return time.Duration(secs) * time.Second
	}
	return 0
}

const maxEmbedRetries = 3

func retryableEmbed(fn func() ([]float32, error)) ([]float32, error) {
	var lastErr error
	for attempt := 0; attempt <= maxEmbedRetries; attempt++ {
		vec, err := fn()
		if err == nil {
			return vec, nil
		}

		var httpErr *embedHTTPError
		if !errors.As(err, &httpErr) || !isRetryableStatus(httpErr.StatusCode) {
			return nil, err
		}

		lastErr = err
		if attempt == maxEmbedRetries {
			break
		}

		delay := httpErr.RetryAfter
		if delay == 0 {
			base := time.Second * time.Duration(1<<uint(attempt))
			jitter := time.Duration(float64(base) * (0.75 + rand.Float64()*0.5))
			delay = jitter
		}
		time.Sleep(delay)
	}

	var httpErr *embedHTTPError
	if errors.As(lastErr, &httpErr) && httpErr.StatusCode == 429 {
		return nil, &RateLimitError{
			StatusCode: 429,
			Body:       httpErr.Body,
		}
	}
	return nil, lastErr
}

// RateLimitError 保留 429 错误可被上层识别的语义。
// 这里没有迁移完整 llm 包，只留下 embedding 重试后需要暴露的错误类型。
type RateLimitError struct {
	StatusCode int
	Body       string
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("rate limit: status=%d body=%s", e.StatusCode, e.Body)
}

// embedRateLimiter 通过预约时间槽控制 embedding API 调用节奏。
type embedRateLimiter struct {
	mu       sync.Mutex
	interval time.Duration
	nextSlot time.Time
}

func (r *embedRateLimiter) wait() {
	if r.interval == 0 {
		return
	}
	r.mu.Lock()
	now := time.Now()
	wakeAt := r.nextSlot
	if wakeAt.Before(now) {
		wakeAt = now
	}
	r.nextSlot = wakeAt.Add(r.interval)
	r.mu.Unlock()

	if d := time.Until(wakeAt); d > 0 {
		time.Sleep(d)
	}
}

func newEmbedLimiter(rpm int) *embedRateLimiter {
	if rpm <= 0 {
		return nil
	}
	return &embedRateLimiter{
		interval: time.Minute / time.Duration(rpm),
	}
}

// ollamaAvailable 探测 localhost:11434 上是否有运行中的 Ollama 实例。
func ollamaAvailable() bool {
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://localhost:11434/api/tags")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
