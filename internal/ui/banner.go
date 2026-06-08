package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var logoLines = []string{
	"██╗    ██╗██╗██╗  ██╗██╗███╗   ███╗███████╗███████╗██╗  ██╗",
	"██║    ██║██║██║ ██╔╝██║████╗ ████║██╔════╝██╔════╝██║  ██║",
	"██║ █╗ ██║██║█████╔╝ ██║██╔████╔██║█████╗  ███████╗███████║",
	"██║███╗██║██║██╔═██╗ ██║██║╚██╔╝██║██╔══╝  ╚════██║██╔══██║",
	"╚███╔███╔╝██║██║  ██╗██║██║ ╚═╝ ██║███████╗███████║██║  ██║",
	" ╚══╝╚══╝ ╚═╝╚═╝  ╚═╝╚═╝╚═╝     ╚═╝╚══════╝╚══════╝╚═╝  ╚═╝",
}

var grays = []string{
	"\x1b[38;5;250m",
	"\x1b[38;5;248m",
	"\x1b[38;5;245m",
	"\x1b[38;5;243m",
	"\x1b[38;5;240m",
	"\x1b[38;5;238m",
}

// Logo 返回用于帮助信息顶部展示的 ASCII Logo。
func Logo() string {
	var lines []string
	lines = append(lines, "")
	for i, line := range logoLines {
		lines = append(lines, fmt.Sprintf("%s%s%s", grays[i], line, Reset))
	}
	lines = append(lines, "\x1b[38;5;245mWIKIMESH\x1b[0m")
	lines = append(lines, "")
	return strings.Join(lines, "\n") + "\n"
}

// ShowLogo 直接把 Logo 输出到终端。
func ShowLogo() {
	fmt.Print(Logo())
}

// CommandName 返回当前可执行文件名，用于生成帮助中的命令名称。
func CommandName() string {
	if len(os.Args) == 0 {
		return "wikimesh"
	}
	name := filepath.Base(os.Args[0])
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "wikimesh"
	}
	return name
}
