package qmdcmd

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/JieWaZi/wikimesh/internal/cli/common"
	"github.com/JieWaZi/wikimesh/internal/ui"
	qmd "github.com/JieWaZi/wikimesh/pkg/qmd"
	"github.com/schollz/progressbar/v3"
	"golang.org/x/term"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func openDefaultStore(ctx context.Context) (*qmd.Store, qmd.FileConfig, error) {
	return common.OpenDefaultQMDStore(ctx)
}

func openStoreForProject(ctx context.Context, project string) (*qmd.Store, qmd.FileConfig, string, error) {
	root, configPath, err := resolveProjectQMDConfigPath(project)
	if err != nil {
		return nil, qmd.FileConfig{}, "", err
	}
	cfg, err := common.LoadQMDConfig(configPath)
	if err != nil {
		return nil, qmd.FileConfig{}, "", err
	}
	absolutizeQMDConfig(root, &cfg)
	store, err := common.OpenQMDStoreFromConfig(ctx, cfg)
	if err != nil {
		return nil, qmd.FileConfig{}, "", err
	}
	return store, cfg, configPath, nil
}

func resolveProjectQMDConfigPath(project string) (string, string, error) {
	if strings.TrimSpace(project) == "" {
		if root, ok := qmdConfigRoot(common.DefaultQMDConfigPath); ok {
			return root, common.DefaultQMDConfigPath, nil
		}
		return "", common.DefaultQMDConfigPath, nil
	}
	root, err := common.ResolveWikiRoot(".", project)
	if err != nil {
		return "", "", err
	}
	return root, common.QMDConfigPathForRoot(root), nil
}

func qmdConfigRoot(configPath string) (string, bool) {
	configPath = strings.TrimSpace(configPath)
	if configPath == "" {
		return "", false
	}
	return filepath.Dir(filepath.Dir(configPath)), true
}

func absolutizeQMDConfig(root string, cfg *qmd.FileConfig) {
	if strings.TrimSpace(root) == "" {
		return
	}
	if cfg.DBPath != "" && !filepath.IsAbs(cfg.DBPath) {
		cfg.DBPath = filepath.Join(root, cfg.DBPath)
	}
	for i := range cfg.Collections {
		if cfg.Collections[i].Path != "" && !filepath.IsAbs(cfg.Collections[i].Path) {
			cfg.Collections[i].Path = filepath.Join(root, cfg.Collections[i].Path)
		}
	}
}

// downloadConfiguredModel 按配置解析模型来源和落盘路径，并执行下载。
func downloadConfiguredModel(ctx context.Context, cfg qmd.FileConfig, role qmd.ModelRole, out, errOut io.Writer) (qmd.ModelDownload, error) {
	source, destination := modelSourceAndDestination(cfg, role)
	if strings.TrimSpace(destination) == "" {
		destination = qmd.LocalModelPath(source)
	}
	if _, err := os.Stat(destination); err == nil {
		return qmd.DownloadModel(ctx, role, source, destination)
	} else if !os.IsNotExist(err) {
		return qmd.ModelDownload{Role: role, Source: source, Destination: destination}, err
	}
	fmt.Fprintf(out, ui.Messages().OutputQMDDownloadingFmt, source, destination)
	progress := newModelDownloadProgress(errOut, role)
	result, err := qmd.DownloadModelWithOptions(ctx, role, source, destination, qmd.ModelDownloadOptions{Progress: progress.report})
	progress.finish()
	return result, err
}

// newModelDownloadProgress 创建模型下载进度状态。
func newModelDownloadProgress(errOut io.Writer, role qmd.ModelRole) *modelDownloadProgress {
	return &modelDownloadProgress{errOut: errOut, role: role}
}

// modelDownloadProgress 封装模型下载进度条状态。
type modelDownloadProgress struct {
	// errOut 是进度条输出目标。
	errOut io.Writer
	// role 是当前下载的模型角色。
	role qmd.ModelRole
	// bar 是延迟创建的终端进度条。
	bar *progressbar.ProgressBar
}

// report 根据 qmd 下载回调更新进度条。
func (p *modelDownloadProgress) report(info qmd.ModelDownloadProgress) {
	if !writerIsTerminal(p.errOut) || info.Total <= 0 {
		return
	}
	if p.bar == nil {
		p.bar = progressbar.NewOptions64(info.Total,
			progressbar.OptionSetWriter(p.errOut),
			progressbar.OptionSetDescription(fmt.Sprintf("Downloading %s", p.role)),
			progressbar.OptionSetWidth(28),
			progressbar.OptionShowBytes(true),
			progressbar.OptionShowCount(),
			progressbar.OptionClearOnFinish(),
			progressbar.OptionThrottle(100*time.Millisecond),
		)
	}
	_ = p.bar.Set64(info.Current)
}

// finish 收尾终端进度条，非终端输出不会产生额外内容。
func (p *modelDownloadProgress) finish() {
	if p.bar != nil {
		_ = p.bar.Finish()
	}
}

// modelSourceAndDestination 从配置中解析指定模型角色的来源和目标路径。
func modelSourceAndDestination(cfg qmd.FileConfig, role qmd.ModelRole) (string, string) {
	fallback := func(source, destination string) (string, string) {
		if source == "" {
			source = destination
		}
		if destination == "" {
			destination = qmd.LocalModelPath(source)
		}
		return source, destination
	}
	switch role {
	case qmd.ModelRoleEmbed:
		return fallback(cfg.Models.Embed, cfg.Embedding.Model)
	case qmd.ModelRoleRerank:
		return fallback(cfg.Models.Rerank, cfg.Reranker.Model)
	case qmd.ModelRoleGenerate:
		return fallback(cfg.Models.Generate, cfg.QueryExpansion.Model)
	default:
		return "", ""
	}
}

// uniqueNonEmpty 去重并剔除空字符串。
func uniqueNonEmpty(items []string) []string {
	seen := map[string]bool{}
	var result []string
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		result = append(result, item)
	}
	return result
}

// maxInt 返回两个整数中的较大值。
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// writerIsTerminal 判断输出目标是否为终端，用于决定是否显示进度条。
func writerIsTerminal(w io.Writer) bool {
	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(file.Fd()))
}

// printJSON 以缩进 JSON 输出命令结果。
func printJSON(out io.Writer, v any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
