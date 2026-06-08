package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// SelectOneOptions 描述单选交互所需的数据。
type SelectOneOptions struct {
	// Message 是交互顶部展示的提示语。
	Message string
	// Items 是可选项列表。
	Items []Option
}

// selectOneModel 是 Bubble Tea 的单选状态模型。
type selectOneModel struct {
	message   string
	items     []Option
	cursor    int
	cancelled bool
}

// SelectOne 启动一个单选交互，并返回用户最终选择。
func SelectOne(options SelectOneOptions) (string, bool, error) {
	model := selectOneModel{
		message: options.Message,
		items:   options.Items,
	}
	result, err := tea.NewProgram(model).Run()
	if err != nil {
		return "", false, err
	}
	finalModel := result.(selectOneModel)
	if finalModel.cancelled {
		return "", true, nil
	}
	if len(finalModel.items) == 0 {
		return "", false, nil
	}
	return finalModel.items[finalModel.cursor].Value, false, nil
}

// Init 实现 Bubble Tea 初始化接口。
func (m selectOneModel) Init() tea.Cmd { return nil }

// Update 处理键盘输入并更新单选状态。
func (m selectOneModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			return m, tea.Quit
		case "ctrl+c", "esc":
			m.cancelled = true
			return m, tea.Quit
		case "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		}
	}
	return m, nil
}

// View 渲染当前单选界面。
func (m selectOneModel) View() string {
	copy := Messages()
	var lines []string
	lines = append(lines, fmt.Sprintf("%s  %s", Green+"◆"+Reset, m.message))
	lines = append(lines, Dim+"│"+Reset)
	lines = append(lines, fmt.Sprintf("%s  %s%s%s", Dim+"│"+Reset, Dim, copy.SingleSelectHelp, Reset))
	lines = append(lines, Dim+"│"+Reset)
	for i, item := range m.items {
		prefix := " "
		label := item.Label
		if i == m.cursor {
			prefix = Cyan + "❯" + Reset
			label = "\x1b[4m" + label + "\x1b[24m"
		}
		hint := ""
		if item.Hint != "" {
			hint = fmt.Sprintf(" %s(%s)%s", Dim, item.Hint, Reset)
		}
		lines = append(lines, fmt.Sprintf("%s %s %s%s", Dim+"│"+Reset, prefix, label, hint))
	}
	lines = append(lines, Dim+"└"+Reset)
	return strings.Join(lines, "\n")
}
