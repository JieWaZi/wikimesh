package log

import (
	"fmt"
	"log/slog"
	"os"
)

var logger *slog.Logger

func init() {
	// 默认 WARN 级别，正常运行时保持安静。
	// SetVerbosity(1) 开启 INFO，SetVerbosity(2+) 开启 DEBUG。
	logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	}))
}

// SetVerbose 开启 info 级日志，对应单个 -v。
func SetVerbose(verbose bool) {
	if verbose {
		SetVerbosity(1)
	}
}

// SetVerbosity 按 verbose 计数设置日志级别。
// 0 表示 WARN，1 表示 INFO，2 及以上表示 DEBUG。
func SetVerbosity(level int) {
	var slogLevel slog.Level
	switch {
	case level >= 2:
		slogLevel = slog.LevelDebug
	case level == 1:
		slogLevel = slog.LevelInfo
	default:
		slogLevel = slog.LevelWarn
	}
	logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slogLevel,
	}))
}

func Debug(msg string, args ...any) { logger.Debug(msg, args...) }
func Info(msg string, args ...any)  { logger.Info(msg, args...) }
func Warn(msg string, args ...any)  { logger.Warn(msg, args...) }
func Error(msg string, args ...any) { logger.Error(msg, args...) }

// Op 表示错误发生时的操作上下文。
type Op string

// QMDError 是带操作名和路径上下文的 qmd 内部结构化错误。
type QMDError struct {
	Op   Op
	Path string
	Err  error
}

func (e *QMDError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("%s [%s]: %s", e.Op, e.Path, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Op, e.Err)
}

func (e *QMDError) Unwrap() error { return e.Err }

// E 创建一个带操作名的错误。
func E(op Op, err error) *QMDError {
	return &QMDError{Op: op, Err: err}
}

// EP 创建一个同时带操作名和路径的错误。
func EP(op Op, path string, err error) *QMDError {
	return &QMDError{Op: op, Path: path, Err: err}
}
