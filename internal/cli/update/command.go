package updatecmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/JieWaZi/wikimesh/internal/app/updateapp"
	"github.com/JieWaZi/wikimesh/internal/ui"
)

type updater interface {
	Update(context.Context) (updateapp.Result, error)
}

// NewCommand 构造 `wikimesh update` 命令。
func NewCommand() *cobra.Command {
	return NewCommandWithService(updateapp.NewService(updateapp.ServiceOptions{}))
}

// NewCommandWithService 使用注入的服务构造命令，便于测试 CLI 输出。
func NewCommandWithService(service updater) *cobra.Command {
	msg := ui.Messages()
	return &cobra.Command{
		Use:   "update",
		Short: msg.WikiUpdateShort,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := service.Update(cmd.Context())
			if err != nil {
				return err
			}
			if result.Deferred {
				fmt.Fprintf(cmd.OutOrStdout(), msg.OutputWikiUpdateDeferredFmt, ui.Green, ui.Reset, result.Asset, result.Path)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), msg.OutputWikiUpdateDoneFmt, ui.Green, ui.Reset, result.Asset, result.Path)
			return nil
		},
	}
}
