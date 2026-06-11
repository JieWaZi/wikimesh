package agentcmd

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/JieWaZi/wikimesh/internal/app/daemonapp"
	"github.com/JieWaZi/wikimesh/pkg/agent"
)

// NewCommand 构造 `wikimesh agent` 调试命令。
func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "运行本地 agent 调试命令",
	}
	cmd.AddCommand(newRunCommand())
	return cmd
}

func newRunCommand() *cobra.Command {
	// run 命令是 daemon 前的最小调试入口，直接调用 pkg/agent Codex runtime。
	var cwd string
	var project string
	var sessionID string
	var model string
	var jsonOutput bool
	var timeout time.Duration

	cmd := &cobra.Command{
		Use:   "run <message>",
		Short: "调用 Codex 执行一轮对话",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resolvedCwd, err := daemonapp.ResolveCwd(cwd, project)
			if err != nil {
				return err
			}
			backend, err := agent.New("codex", agent.Config{})
			if err != nil {
				return err
			}
			sess, err := backend.Execute(cmd.Context(), args[0], agent.ExecOptions{
				Cwd:             resolvedCwd,
				Model:           model,
				ResumeSessionID: sessionID,
				Timeout:         timeout,
			})
			if err != nil {
				return err
			}
			if jsonOutput {
				return streamAgentJSON(cmd.OutOrStdout(), sess)
			}
			return streamAgentText(cmd.OutOrStdout(), sess)
		},
	}
	cmd.Flags().StringVar(&cwd, "cwd", "", "Codex 执行目录")
	cmd.Flags().StringVar(&project, "project", "", "Wikimesh 文档库项目名；未指定 --cwd 时使用项目本地文档库目录")
	cmd.Flags().StringVar(&sessionID, "session", "", "要续聊的 Codex thread/session ID")
	cmd.Flags().StringVar(&model, "model", "", "Codex 模型覆盖")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "以 JSON lines 输出消息和最终结果")
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "单轮执行超时时间，0 表示不设置")
	return cmd
}

func streamAgentText(out io.Writer, sess *agent.Session) error {
	// 文本模式面向人工调试：文本直接输出，工具事件用简短标记分隔。
	for msg := range sess.Messages {
		switch msg.Type {
		case agent.MessageText:
			fmt.Fprint(out, msg.Content)
		case agent.MessageToolUse:
			fmt.Fprintf(out, "\n[tool:%s started]\n", msg.Tool)
		case agent.MessageToolResult:
			if msg.Output != "" {
				fmt.Fprintf(out, "\n[tool:%s result]\n%s\n", msg.Tool, msg.Output)
			}
		case agent.MessageError:
			fmt.Fprintf(out, "\n[error] %s\n", msg.Content)
		}
	}
	result := <-sess.Result
	if result.SessionID != "" {
		fmt.Fprintf(out, "\n\nsession_id: %s\n", result.SessionID)
	}
	if result.Error != "" {
		return fmt.Errorf("%s", result.Error)
	}
	return nil
}

func streamAgentJSON(out io.Writer, sess *agent.Session) error {
	// JSON lines 模式便于脚本检查 session_id、tool 事件和最终状态。
	enc := json.NewEncoder(out)
	for msg := range sess.Messages {
		if err := enc.Encode(map[string]any{"type": "message", "message": msg}); err != nil {
			return err
		}
	}
	result := <-sess.Result
	if err := enc.Encode(map[string]any{"type": "result", "result": result}); err != nil {
		return err
	}
	if result.Error != "" {
		return fmt.Errorf("%s", result.Error)
	}
	return nil
}
