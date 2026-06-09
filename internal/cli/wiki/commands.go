package wiki

import (
	"github.com/spf13/cobra"

	checkcmd "github.com/JieWaZi/wikimesh/internal/cli/wiki/check"
	glossarycmd "github.com/JieWaZi/wikimesh/internal/cli/wiki/glossary"
	initcmd "github.com/JieWaZi/wikimesh/internal/cli/wiki/init"
	querycmd "github.com/JieWaZi/wikimesh/internal/cli/wiki/query"
	readcmd "github.com/JieWaZi/wikimesh/internal/cli/wiki/read"
	repocmd "github.com/JieWaZi/wikimesh/internal/cli/wiki/repo"
	searchcmd "github.com/JieWaZi/wikimesh/internal/cli/wiki/search"
)

// Commands 返回所有直接挂载到根命令的 Wiki 文档库命令。
func Commands() []*cobra.Command {
	return []*cobra.Command{
		initcmd.NewCommand(),
		readcmd.NewCommand(),
		searchcmd.NewCommand(),
		querycmd.NewCommand(),
		glossarycmd.NewCommand(),
		repocmd.NewCommand(),
		checkcmd.NewCommand(),
	}
}
