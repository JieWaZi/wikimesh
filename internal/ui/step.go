package ui

import (
	"fmt"
	"os"
)

// StepPrinter 用于在终端中输出带状态的步骤提示。
type StepPrinter struct {
	// active 表示当前是否存在尚未结束的步骤。
	active bool
	// isTTY 表示当前输出是否连接到真实终端。
	isTTY bool
}

// NewStepPrinter 创建步骤输出器，并探测当前是否处于 TTY。
func NewStepPrinter() *StepPrinter {
	info, err := os.Stdout.Stat()
	isTTY := err == nil && (info.Mode()&os.ModeCharDevice) != 0
	return &StepPrinter{isTTY: isTTY}
}

// Start 输出一个开始中的步骤提示。
func (p *StepPrinter) Start(message string) {
	if p.active && p.isTTY {
		fmt.Print("\r\x1b[2K")
	}
	fmt.Printf("%s  %s\n", Green+"◆"+Reset, message)
	p.active = true
}

// Stop 输出一个完成状态的步骤提示。
func (p *StepPrinter) Stop(message string) {
	if p.active && p.isTTY {
		fmt.Print("\r\x1b[1A\x1b[2K")
	}
	fmt.Printf("%s  %s\n", Green+"◇"+Reset, message)
	p.active = false
}

// Step 输出一个普通步骤提示。
func Step(message string) {
	fmt.Printf("%s  %s\n", Green+"◆"+Reset, message)
}

// Note 输出一个标题加多行内容的说明块。
func Note(title string, lines []string) {
	fmt.Printf("%s  %s\n", Green+"◆"+Reset, Bold+title+Reset)
	for _, line := range lines {
		fmt.Printf("%s\n", line)
	}
}
