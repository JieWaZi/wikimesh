package daemoncmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/JieWaZi/wikimesh/internal/app/daemonapp"
)

// NewCommand 构造 `wikimesh daemon` 命令。
func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "运行本地 Codex agent daemon",
	}
	cmd.AddCommand(newStartCommand())
	cmd.AddCommand(newStatusCommand())
	cmd.AddCommand(newStopCommand())
	return cmd
}

func newStartCommand() *cobra.Command {
	// addr/dbPath/codexPath 都通过 flag 注入，方便本地调试和 web 启动器覆盖。
	var addr string
	var dbPath string
	var codexPath string
	cmd := &cobra.Command{
		Use:   "start",
		Short: "启动本地 Codex agent daemon",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			server, err := daemonapp.NewServer(daemonapp.Options{
				Addr:            addr,
				DBPath:          dbPath,
				CodexExecutable: codexPath,
			})
			if err != nil {
				return err
			}
			defer server.Close()
			fmt.Fprintf(cmd.OutOrStdout(), "wikimesh daemon listening on %s\n", defaultAddr(addr))
			return server.Run(cmd.Context())
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:19515", "daemon 监听地址")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite 状态数据库路径，默认 ~/.wikimesh/daemon/state.db")
	cmd.Flags().StringVar(&codexPath, "codex", "", "codex 可执行文件路径")
	return cmd
}

func newStatusCommand() *cobra.Command {
	var addr string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "查看本地 Codex agent daemon 状态",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return proxyDaemonRequest(cmd.Context(), cmd.OutOrStdout(), http.MethodGet, "http://"+defaultAddr(addr)+"/health")
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:19515", "daemon 地址")
	return cmd
}

func newStopCommand() *cobra.Command {
	var addr string
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "停止本地 Codex agent daemon",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return proxyDaemonRequest(cmd.Context(), cmd.OutOrStdout(), http.MethodPost, "http://"+defaultAddr(addr)+"/shutdown")
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:19515", "daemon 地址")
	return cmd
}

func proxyDaemonRequest(ctx context.Context, out io.Writer, method, url string) error {
	// status/stop 只是薄客户端，按 Multica 的本地 /health 和 /shutdown 模式调用 daemon。
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, copyErr := io.Copy(out, resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("%s returned %s", url, resp.Status)
	}
	return copyErr
}

func defaultAddr(addr string) string {
	// 多个子命令共享默认地址，避免调用方传空值时拼出非法 URL。
	if addr == "" {
		return "127.0.0.1:19515"
	}
	return addr
}
