package qmdcmd

import (
	"context"
	"fmt"
	"github.com/schollz/progressbar/v3"
	"io"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/JieWaZi/wikimesh/internal/cli/common"
	"github.com/JieWaZi/wikimesh/internal/ui"
	qmd "github.com/JieWaZi/wikimesh/pkg/qmd"
)

func newModelCommand() *cobra.Command {
	msg := ui.Messages()
	cmd := &cobra.Command{
		Use:   "model",
		Short: msg.ModelShort,
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}
	cmd.AddCommand(newModelDownloadCommand())
	cmd.AddCommand(newModelLibCommand())
	return cmd
}

func newModelDownloadCommand() *cobra.Command {
	msg := ui.Messages()
	return &cobra.Command{
		Use:   "download [embed|rerank|generate|all]",
		Short: msg.ModelDownloadShort,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "all"
			if len(args) > 0 {
				target = args[0]
			}
			return runModelDownload(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), common.DefaultQMDConfigPath, target)
		},
	}
}

// runModelDownload 下载配置中声明的 qmd 模型文件。
func runModelDownload(ctx context.Context, out, errOut io.Writer, configPath string, target string) error {
	cfg, err := common.LoadQMDConfig(configPath)
	if err != nil {
		return err
	}
	roles, err := modelDownloadRoles(target)
	if err != nil {
		return err
	}
	for _, role := range roles {
		result, err := downloadConfiguredModel(ctx, cfg, role, out, errOut)
		if err != nil {
			return err
		}
		if result.Downloaded {
			fmt.Fprintf(out, ui.Messages().OutputQMDDownloadedFmt, result.Source, result.Destination)
		} else {
			fmt.Fprintf(out, ui.Messages().OutputQMDExistsFmt, result.Destination)
		}
	}
	return nil
}

// modelDownloadRoles 将命令参数映射到需要下载的模型角色。
func modelDownloadRoles(target string) ([]qmd.ModelRole, error) {
	switch strings.TrimSpace(target) {
	case "", "all":
		return []qmd.ModelRole{qmd.ModelRoleEmbed, qmd.ModelRoleRerank, qmd.ModelRoleGenerate}, nil
	case "embed":
		return []qmd.ModelRole{qmd.ModelRoleEmbed}, nil
	case "rerank":
		return []qmd.ModelRole{qmd.ModelRoleRerank}, nil
	case "generate", "query":
		return []qmd.ModelRole{qmd.ModelRoleGenerate}, nil
	default:
		return nil, fmt.Errorf("unknown model role: %s", target)
	}
}

func newModelLibCommand() *cobra.Command {
	msg := ui.Messages()
	cmd := &cobra.Command{
		Use:   "lib",
		Short: msg.ModelLibShort,
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}
	cmd.AddCommand(newModelLibInstallCommand())
	return cmd
}

func newModelLibInstallCommand() *cobra.Command {
	msg := ui.Messages()
	opts := modelLibInstallOptions{
		libPath:   qmd.DefaultLlamaLibDir(),
		processor: "auto",
		version:   "latest",
		osName:    runtime.GOOS,
	}
	cmd := &cobra.Command{
		Use:   "install",
		Short: msg.ModelLibInstallShort,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runModelLibInstall(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.libPath, "lib", qmd.DefaultLlamaLibDir(), msg.FlagLibPath)
	cmd.Flags().StringVarP(&opts.processor, "processor", "p", "auto", msg.FlagProcessor)
	cmd.Flags().StringVar(&opts.version, "version", "latest", msg.FlagVersion)
	cmd.Flags().StringVar(&opts.osName, "os", runtime.GOOS, msg.FlagOS)
	cmd.Flags().BoolVarP(&opts.upgrade, "upgrade", "u", false, msg.FlagUpgrade)
	return cmd
}

// modelLibInstallOptions 保存 llama.cpp 运行时库安装选项。
type modelLibInstallOptions struct {
	// libPath 是 llama.cpp 动态库安装目录。
	libPath string
	// processor 是 yzma 下载的硬件后端，auto 表示按平台选择。
	processor string
	// version 是 llama.cpp release 版本，latest 表示使用最新可用版本。
	version string
	// osName 是 yzma 下载目标操作系统。
	osName string
	// upgrade 表示即使已安装也重新下载。
	upgrade bool
}

// runModelLibInstall 安装 qmd 本地模型所需的 llama.cpp 运行时库。
func runModelLibInstall(ctx context.Context, out, errOut io.Writer, opts modelLibInstallOptions) error {
	libPath := strings.TrimSpace(opts.libPath)
	if libPath == "" {
		libPath = qmd.DefaultLlamaLibDir()
	}
	processor := qmd.ResolveLlamaLibProcessor(opts.processor)
	version := strings.TrimSpace(opts.version)
	if version == "" {
		version = "latest"
	}
	osName := strings.TrimSpace(opts.osName)
	if osName == "" {
		osName = runtime.GOOS
	}
	if !opts.upgrade && qmd.LlamaLibAlreadyInstalled(libPath) {
		fmt.Fprintf(out, ui.Messages().OutputQMDExistsFmt, libPath)
		return nil
	}
	fmt.Fprintf(out, ui.Messages().OutputQMDInstallingLibFmt, processor, version, libPath)
	progress := newLibraryDownloadProgress(errOut)
	err := qmd.InstallLlamaLib(ctx, qmd.LlamaLibInstallOptions{
		LibPath:   libPath,
		Processor: processor,
		Version:   version,
		OS:        osName,
		Progress:  progress,
	})
	progress.finish()
	if err != nil {
		return err
	}
	fmt.Fprintf(out, ui.Messages().OutputQMDInstalledFmt, libPath)
	return nil
}

var _ qmd.LlamaLibProgressTracker = (*libraryDownloadProgress)(nil)

// libraryDownloadProgress 封装 llama.cpp 运行时库下载进度。
type libraryDownloadProgress struct {
	// errOut 是进度条输出目标。
	errOut io.Writer
	// bar 是延迟创建的终端进度条。
	bar *progressbar.ProgressBar
}

// newLibraryDownloadProgress 创建运行时库下载进度状态。
func newLibraryDownloadProgress(errOut io.Writer) *libraryDownloadProgress {
	return &libraryDownloadProgress{errOut: errOut}
}

// TrackProgress 实现 qmd 下载进度接口，把下载流包装为可上报进度的 reader。
func (p *libraryDownloadProgress) TrackProgress(_ string, currentSize, totalSize int64, stream io.ReadCloser) io.ReadCloser {
	if stream == nil {
		return nil
	}
	return &libraryProgressReader{
		reader:      stream,
		progress:    p,
		currentSize: currentSize,
		totalSize:   totalSize,
	}
}

// report 根据已读字节数更新运行时库下载进度条。
func (p *libraryDownloadProgress) report(currentSize, totalSize int64) {
	if !writerIsTerminal(p.errOut) || totalSize <= 0 {
		return
	}
	if p.bar == nil {
		p.bar = progressbar.NewOptions64(totalSize,
			progressbar.OptionSetWriter(p.errOut),
			progressbar.OptionSetDescription("Downloading llama.cpp"),
			progressbar.OptionSetWidth(28),
			progressbar.OptionShowBytes(true),
			progressbar.OptionShowCount(),
			progressbar.OptionClearOnFinish(),
			progressbar.OptionThrottle(100*time.Millisecond),
		)
	}
	_ = p.bar.Set64(currentSize)
}

// finish 收尾运行时库下载进度条。
func (p *libraryDownloadProgress) finish() {
	if p.bar != nil {
		_ = p.bar.Finish()
	}
}

// libraryProgressReader 在读取下载流时同步推进进度条。
type libraryProgressReader struct {
	// reader 是 go-getter 提供的下载流。
	reader io.ReadCloser
	// progress 是共享进度条状态。
	progress *libraryDownloadProgress
	// currentSize 是已经读取的字节数。
	currentSize int64
	// totalSize 是下载响应声明的总字节数。
	totalSize int64
}

// Read 读取下载内容并累计已读字节。
func (r *libraryProgressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		r.currentSize += int64(n)
		r.progress.report(r.currentSize, r.totalSize)
	}
	return n, err
}

// Close 关闭下载流前进行最后一次进度同步。
func (r *libraryProgressReader) Close() error {
	r.progress.report(r.currentSize, r.totalSize)
	return r.reader.Close()
}
