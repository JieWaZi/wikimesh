package qmd

import (
	"context"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/JieWaZi/wikimesh/pkg/qmd/internal/llamaruntime"
	"github.com/hybridgroup/yzma/pkg/download"
)

// LlamaLibProgressTracker 接收 llama.cpp 动态库下载进度。
// 它的接口形状与底层下载器兼容，但不会把 qmd/internal 包暴露给调用方。
type LlamaLibProgressTracker interface {
	// TrackProgress 包装下载流并报告当前字节数和总字节数。
	TrackProgress(path string, currentSize, totalSize int64, stream io.ReadCloser) io.ReadCloser
}

// LlamaLibInstallOptions 控制 llama.cpp 动态库安装行为。
type LlamaLibInstallOptions struct {
	// LibPath 是动态库安装目录；为空时使用 DefaultLlamaLibDir。
	LibPath string

	// Processor 是目标硬件后端，例如 auto、cpu、metal、cuda、vulkan、rocm。
	Processor string

	// Version 是 llama.cpp release 版本；为空时使用 latest。
	Version string

	// OS 是目标操作系统；为空时使用当前 runtime.GOOS。
	OS string

	// Progress 是可选的下载进度回调。
	Progress LlamaLibProgressTracker
}

// DefaultLlamaLibDir 返回 qmd 默认保存 llama.cpp 动态库的目录。
func DefaultLlamaLibDir() string {
	return llamaruntime.DefaultLibDir()
}

// ResolveLlamaLibProcessor 按 qmd 默认规则解析 llama.cpp 下载后端。
// auto 会在 Apple Silicon macOS 上解析为 metal，其他平台默认解析为 cpu。
func ResolveLlamaLibProcessor(processor string) string {
	processor = strings.TrimSpace(strings.ToLower(processor))
	if processor != "" && processor != "auto" {
		return processor
	}
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		return "metal"
	}
	return "cpu"
}

// LlamaLibAlreadyInstalled 判断指定目录是否已经安装了 llama.cpp 动态库。
// libPath 为空时检查 DefaultLlamaLibDir。
func LlamaLibAlreadyInstalled(libPath string) bool {
	libPath = strings.TrimSpace(libPath)
	if libPath == "" {
		libPath = DefaultLlamaLibDir()
	}
	return download.AlreadyInstalled(libPath)
}

// InstallLlamaLib 下载并安装 llama.cpp 动态库。
// 该函数只负责安装运行时动态库，不会加载模型或初始化 Store。
func InstallLlamaLib(ctx context.Context, opts LlamaLibInstallOptions) error {
	libPath := strings.TrimSpace(opts.LibPath)
	if libPath == "" {
		libPath = DefaultLlamaLibDir()
	}
	processor := ResolveLlamaLibProcessor(opts.Processor)
	version := strings.TrimSpace(opts.Version)
	if version == "" {
		version = "latest"
	}
	osName := strings.TrimSpace(opts.OS)
	if osName == "" {
		osName = runtime.GOOS
	}
	if err := os.MkdirAll(libPath, 0o755); err != nil {
		return err
	}
	return download.GetWithContext(ctx, runtime.GOARCH, osName, processor, version, libPath, opts.Progress)
}
