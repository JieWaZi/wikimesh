package qmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ModelRole 标识模型在 SDK 中承担的职责。
// 不同职责会从 FileConfig 的不同字段读取模型来源和目标路径。
type ModelRole string

const (
	// ModelRoleEmbed 表示 embedding 向量模型。
	ModelRoleEmbed ModelRole = "embed"

	// ModelRoleRerank 表示 query reranker 重排模型。
	ModelRoleRerank ModelRole = "rerank"

	// ModelRoleGenerate 表示 query expansion/generate 文本生成模型。
	ModelRoleGenerate ModelRole = "generate"
)

// ModelDownload 描述一次模型下载或复用已有模型文件的结果。
type ModelDownload struct {
	// Role 是本次下载对应的模型角色。
	Role ModelRole
	// Source 是配置里的模型来源 URI。
	Source string
	// Destination 是模型下载后的本地路径。
	Destination string
	// Downloaded 表示本次调用是否实际写入了新文件。
	Downloaded bool
}

// ModelDownloadProgress 是模型下载过程中的字节级进度事件。
type ModelDownloadProgress struct {
	// Role 是正在下载的模型角色。
	Role ModelRole
	// Source 是正在下载的模型来源 URI。
	Source string
	// Destination 是正在写入的本地文件路径。
	Destination string
	// Current 是已经写入的字节数。
	Current int64
	// Total 是可预期的总字节数；未知时为 0。
	Total int64
}

// ModelDownloadOptions 控制模型下载行为。
type ModelDownloadOptions struct {
	// Progress 在下载过程中收到字节级进度。
	Progress func(ModelDownloadProgress)
}

// LocalModelPath 把模型来源 URI 映射到默认 `.wikimesh/models` 本地路径。
// `hf:` 来源会取 HuggingFace 路径最后一段文件名；`file://` 和普通路径会取 basename。
func LocalModelPath(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return ""
	}
	name := source
	if strings.HasPrefix(source, "hf:") {
		name = path.Base(strings.TrimPrefix(source, "hf:"))
	} else if u, err := url.Parse(source); err == nil && u.Scheme == "file" {
		name = filepath.Base(u.Path)
	} else {
		name = filepath.Base(source)
	}
	return filepath.ToSlash(filepath.Join(DefaultModelDir, name))
}

// ModelSourceURL 把 SDK 支持的模型来源转换为可下载 URL。
// 目前支持 `hf:owner/repo/file.gguf` 简写、`file://` 本地文件和普通 URL。
func ModelSourceURL(source string) (string, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return "", fmt.Errorf("model source is empty")
	}
	if strings.HasPrefix(source, "hf:") {
		ref := strings.TrimPrefix(source, "hf:")
		parts := strings.Split(ref, "/")
		if len(parts) < 3 {
			return "", fmt.Errorf("invalid HuggingFace model source: %s", source)
		}
		repo := strings.Join(parts[:len(parts)-1], "/")
		file := parts[len(parts)-1]
		return "https://huggingface.co/" + repo + "/resolve/main/" + file, nil
	}
	return source, nil
}

// DownloadModel 下载指定职责的模型到目标路径。
// destination 为空时会使用 LocalModelPath(source)；目标文件已存在时不会重复下载。
func DownloadModel(ctx context.Context, role ModelRole, source string, destination string) (ModelDownload, error) {
	return DownloadModelWithOptions(ctx, role, source, destination, ModelDownloadOptions{})
}

// DownloadModelWithOptions 下载指定职责的模型，并可通过 Progress 接收下载进度。
// 该函数只负责文件获取，不会加载模型或初始化 llama.cpp 运行时。
func DownloadModelWithOptions(ctx context.Context, role ModelRole, source string, destination string, opts ModelDownloadOptions) (ModelDownload, error) {
	result := ModelDownload{Role: role, Source: source, Destination: destination}
	if strings.TrimSpace(destination) == "" {
		destination = LocalModelPath(source)
		result.Destination = destination
	}
	if _, err := os.Stat(destination); err == nil {
		return result, nil
	} else if err != nil && !os.IsNotExist(err) {
		return result, err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return result, err
	}
	url, err := ModelSourceURL(source)
	if err != nil {
		return result, err
	}
	if strings.HasPrefix(url, "file://") {
		return downloadFileModel(result, url, opts)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return result, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return result, fmt.Errorf("download model %s: HTTP %d", source, resp.StatusCode)
	}
	total := resp.ContentLength
	if total < 0 {
		total = 0
	}
	return writeModelFile(result, resp.Body, total, opts.Progress)
}

func downloadFileModel(result ModelDownload, sourceURL string, opts ModelDownloadOptions) (ModelDownload, error) {
	u, err := url.Parse(sourceURL)
	if err != nil {
		return result, err
	}
	in, err := os.Open(u.Path)
	if err != nil {
		return result, err
	}
	defer in.Close()
	var total int64
	if stat, err := in.Stat(); err == nil {
		total = stat.Size()
	}
	return writeModelFile(result, in, total, opts.Progress)
}

func writeModelFile(result ModelDownload, in io.Reader, total int64, progress func(ModelDownloadProgress)) (ModelDownload, error) {
	tmp := result.Destination + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return result, err
	}
	reader := &modelProgressReader{
		reader:   in,
		result:   result,
		total:    total,
		progress: progress,
	}
	reader.report()
	_, copyErr := io.Copy(out, reader)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return result, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return result, closeErr
	}
	if err := os.Rename(tmp, result.Destination); err != nil {
		_ = os.Remove(tmp)
		return result, err
	}
	result.Downloaded = true
	return result, nil
}

type modelProgressReader struct {
	reader   io.Reader
	result   ModelDownload
	current  int64
	total    int64
	progress func(ModelDownloadProgress)
}

func (r *modelProgressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		r.current += int64(n)
		r.report()
	}
	return n, err
}

func (r *modelProgressReader) report() {
	if r.progress == nil {
		return
	}
	r.progress(ModelDownloadProgress{
		Role:        r.result.Role,
		Source:      r.result.Source,
		Destination: r.result.Destination,
		Current:     r.current,
		Total:       r.total,
	})
}
