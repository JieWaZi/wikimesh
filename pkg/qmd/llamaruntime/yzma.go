package llamaruntime

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/hybridgroup/yzma/pkg/llama"
)

const defaultLibDir = ".wikimesh/lib"

var (
	// mu 保护 yzma 动态库加载状态，避免并发模型初始化重复加载。
	mu sync.Mutex
	// loadedPath 是已经加载成功的 llama.cpp 动态库目录。
	loadedPath string
	// initialized 表示 llama.cpp backend 是否已经初始化。
	initialized bool
)

// DefaultLibDir 返回 wikimesh 默认保存 llama.cpp 动态库的目录。
func DefaultLibDir() string {
	return defaultLibDir
}

// ResolveLibDir 按 wikimesh 优先级解析 yzma 动态库目录。
func ResolveLibDir() string {
	for _, value := range []string{os.Getenv("WIKIMESH_YZMA_LIB"), os.Getenv("YZMA_LIB"), defaultLibDir} {
		value = strings.TrimSpace(value)
		if value != "" {
			return filepath.Clean(value)
		}
	}
	return defaultLibDir
}

// EnsureLoaded 加载并初始化 yzma 使用的 llama.cpp 动态库。
func EnsureLoaded() error {
	mu.Lock()
	defer mu.Unlock()

	libDir := ResolveLibDir()
	if initialized && loadedPath == libDir {
		return nil
	}
	if !libraryExists(libDir) {
		return fmt.Errorf("llama.cpp libraries not found in %s; run `wikimesh model lib install` or `make install-llama`, or set WIKIMESH_YZMA_LIB/YZMA_LIB", libDir)
	}
	configureProcessEnvironment()
	if err := llama.Load(libDir); err != nil {
		return fmt.Errorf("load llama.cpp libraries from %s: %w", libDir, err)
	}
	llama.LogSet(llama.LogSilent())
	llama.Init()
	loadedPath = libDir
	initialized = true
	return nil
}

// libraryExists 检查当前平台的 libllama 动态库是否已经安装。
func libraryExists(libDir string) bool {
	name := "libllama.so"
	switch runtime.GOOS {
	case "darwin":
		name = "libllama.dylib"
	case "windows":
		name = "llama.dll"
	}
	_, err := os.Stat(filepath.Join(libDir, name))
	return err == nil
}

// configureProcessEnvironment 设置 qmd 同款短进程 Metal 运行时保护。
func configureProcessEnvironment() {
	if runtime.GOOS == "darwin" && os.Getenv("WIKIMESH_METAL_KEEP_RESIDENCY") != "1" {
		if os.Getenv("GGML_METAL_NO_RESIDENCY") == "" {
			_ = os.Setenv("GGML_METAL_NO_RESIDENCY", "1")
		}
	}
}
