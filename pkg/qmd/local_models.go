package qmd

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

type localTextGenerator interface {
	// Generate 使用本地文本模型按 prompt 生成输出。
	Generate(ctx context.Context, prompt string, maxTokens int) (string, error)
}

// NewLlamaCppQueryExpander 创建默认走 yzma/llama.cpp 的查询扩展模型。
func NewLlamaCppQueryExpander(cfg LlamaCppTextModelConfig) QueryExpander {
	if strings.TrimSpace(cfg.Provider) == "" || strings.TrimSpace(cfg.Provider) == "none" {
		return nil
	}
	if strings.TrimSpace(cfg.Provider) != "local" {
		return nil
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 600
	}
	return &llamaCppQueryExpander{cfg: cfg, generator: newLocalTextGenerator(cfg, "query expansion")}
}

// NewLlamaCppQueryReranker 创建默认走 yzma/llama.cpp 的查询重排模型。
func NewLlamaCppQueryReranker(cfg LlamaCppTextModelConfig) QueryReranker {
	if strings.TrimSpace(cfg.Provider) == "" || strings.TrimSpace(cfg.Provider) == "none" {
		return nil
	}
	if strings.TrimSpace(cfg.Provider) != "local" {
		return nil
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 8
	}
	return &llamaCppQueryReranker{cfg: cfg, generator: newLocalTextGenerator(cfg, "reranker")}
}

type llamaCppQueryExpander struct {
	// cfg 保存查询扩展模型的本地配置。
	cfg LlamaCppTextModelConfig
	// generator 是 yzma 或显式 command fallback 的统一生成入口。
	generator localTextGenerator
}

// Expand 调用本地生成模型产出 lex/vec/hyde 类型查询。
func (e *llamaCppQueryExpander) Expand(ctx context.Context, query string) ([]QueryExpansion, error) {
	return e.ExpandWithOptions(ctx, query, ExpandQueryOptions{})
}

// ExpandWithOptions 调用本地生成模型产出 lex/vec/hyde 类型查询。
// Intent 有值时写入 prompt，帮助模型按领域意图扩展查询。
func (e *llamaCppQueryExpander) ExpandWithOptions(ctx context.Context, query string, opts ExpandQueryOptions) ([]QueryExpansion, error) {
	prompt := "/no_think Expand this search query: " + query
	if strings.TrimSpace(opts.Intent) != "" {
		prompt += "\nQuery intent: " + strings.TrimSpace(opts.Intent)
	}
	out, err := e.generator.Generate(ctx, prompt, e.cfg.MaxTokens)
	if err != nil {
		return nil, err
	}
	expansions := parseQueryExpansionOutput(out)
	if len(expansions) > 0 {
		return expansions, nil
	}
	return []QueryExpansion{
		{Type: QueryExpansionHyDE, Text: "Information about " + query},
		{Type: QueryExpansionLex, Text: query},
		{Type: QueryExpansionVec, Text: query},
	}, nil
}

type llamaCppQueryReranker struct {
	// cfg 保存 reranker 模型的本地配置。
	cfg LlamaCppTextModelConfig
	// generator 是 yzma 或显式 command fallback 的统一生成入口。
	generator localTextGenerator
}

// Rerank 逐候选调用本地模型，把输出解析成 0..1 相关性分数。
func (r *llamaCppQueryReranker) Rerank(ctx context.Context, query string, docs []QueryRerankDocument) ([]QueryRerankScore, error) {
	scores := make([]QueryRerankScore, 0, len(docs))
	for _, doc := range docs {
		prompt := strings.Join([]string{
			"/no_think Score how relevant the document is to the query from 0 to 1.",
			"Return only one number.",
			"Query: " + query,
			"Document: " + doc.Text,
		}, "\n")
		out, err := r.generator.Generate(ctx, prompt, r.cfg.MaxTokens)
		if err != nil {
			return nil, err
		}
		scores = append(scores, QueryRerankScore{
			ID:    doc.ID,
			File:  doc.File,
			Score: parseRerankScore(out),
		})
	}
	return scores, nil
}

type commandTextGenerator struct {
	// cfg 保存显式外部命令的执行配置。
	cfg LlamaCppTextModelConfig
}

// newLocalTextGenerator 在默认 yzma 后端和显式 command fallback 之间做选择。
func newLocalTextGenerator(cfg LlamaCppTextModelConfig, purpose string) localTextGenerator {
	if strings.TrimSpace(cfg.Command) != "" {
		return commandTextGenerator{cfg: cfg}
	}
	return newGoLlamaCppTextGenerator(cfg, purpose)
}

// Generate 调用显式配置的 llama-cli 兼容命令。
func (g commandTextGenerator) Generate(ctx context.Context, prompt string, maxTokens int) (string, error) {
	cfg := g.cfg
	args := []string{}
	if strings.TrimSpace(cfg.Model) != "" {
		args = append(args, "-m", cfg.Model)
	}
	args = append(args, "-p", prompt)
	if maxTokens > 0 {
		args = append(args, "-n", strconv.Itoa(maxTokens))
	}
	cmd := exec.CommandContext(ctx, cfg.Command, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("local text command %q failed: %s", cfg.Command, msg)
	}
	return string(out), nil
}

func parseQueryExpansionOutput(out string) []QueryExpansion {
	var expansions []QueryExpansion
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		typ, text, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		switch strings.TrimSpace(strings.ToLower(typ)) {
		case "lex":
			expansions = append(expansions, QueryExpansion{Type: QueryExpansionLex, Text: text})
		case "vec":
			expansions = append(expansions, QueryExpansion{Type: QueryExpansionVec, Text: text})
		case "hyde":
			expansions = append(expansions, QueryExpansion{Type: QueryExpansionHyDE, Text: text})
		}
	}
	return expansions
}

var scorePattern = regexp.MustCompile(`[-+]?(?:\d+(?:\.\d*)?|\.\d+)`)

func parseRerankScore(out string) float64 {
	lower := strings.ToLower(out)
	if strings.Contains(lower, "yes") {
		return 1
	}
	if strings.Contains(lower, "no") {
		return 0
	}
	match := scorePattern.FindString(out)
	if match == "" {
		return 0
	}
	score, err := strconv.ParseFloat(match, 64)
	if err != nil || math.IsNaN(score) {
		return 0
	}
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}
