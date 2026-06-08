package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Option 描述可供用户选择的一项内容。
type Option struct {
	// Value 是提交后的实际值。
	Value string
	// Label 是展示给用户的标签。
	Label string
	// Hint 是展示在标签后的提示信息。
	Hint string
}

// LockedSection 描述永远会被包含在结果中的固定选项区域。
type LockedSection struct {
	// Title 是固定区域标题。
	Title string
	// Items 是固定包含的选项。
	Items []Option
}

// SearchMultiselectOptions 描述带搜索能力的多选交互。
type SearchMultiselectOptions struct {
	// Message 是交互顶部展示的提示语。
	Message string
	// Items 是可供筛选和选择的动态选项。
	Items []Option
	// MaxVisible 控制列表区域最多展示多少项。
	MaxVisible int
	// InitialSelected 指定初始选中的值集合。
	InitialSelected []string
	// Required 表示提交前至少需要选择一项。
	Required bool
	// LockedSection 表示固定包含且不可取消的选项区域。
	LockedSection *LockedSection
}

// multiSelectModel 是 Bubble Tea 的多选状态模型。
type multiSelectModel struct {
	message       string
	items         []Option
	maxVisible    int
	required      bool
	lockedSection *LockedSection
	query         string
	cursor        int
	selected      map[string]bool
	cancelled     bool
	submitted     bool
}

// SearchMultiselect 启动一个支持搜索的多选交互。
func SearchMultiselect(options SearchMultiselectOptions) ([]string, bool, error) {
	model := multiSelectModel{
		message:       options.Message,
		items:         options.Items,
		maxVisible:    options.MaxVisible,
		required:      options.Required,
		lockedSection: options.LockedSection,
		selected:      map[string]bool{},
	}
	if model.maxVisible == 0 {
		model.maxVisible = 8
	}
	for _, value := range options.InitialSelected {
		model.selected[value] = true
	}

	result, err := tea.NewProgram(model).Run()
	if err != nil {
		return nil, false, err
	}
	finalModel := result.(multiSelectModel)
	if finalModel.cancelled {
		return nil, true, nil
	}

	locked := []string{}
	if finalModel.lockedSection != nil {
		for _, item := range finalModel.lockedSection.Items {
			locked = append(locked, item.Value)
		}
	}
	selected := make([]string, 0, len(finalModel.items))
	for _, item := range finalModel.items {
		if finalModel.selected[item.Value] {
			selected = append(selected, item.Value)
		}
	}
	return append(locked, selected...), false, nil
}

// Init 实现 Bubble Tea 初始化接口。
func (m multiSelectModel) Init() tea.Cmd { return nil }

// Update 处理搜索、多选和确认逻辑。
func (m multiSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		filtered := m.filtered()
		switch msg.String() {
		case "enter":
			if m.required && len(m.selected) == 0 && len(m.lockedValues()) == 0 {
				return m, nil
			}
			m.submitted = true
			return m, tea.Quit
		case "ctrl+c", "esc":
			m.cancelled = true
			return m, tea.Quit
		case "up":
			if len(filtered) > 0 && m.cursor > 0 {
				m.cursor--
			}
		case "down":
			if len(filtered) > 0 && m.cursor < len(filtered)-1 {
				m.cursor++
			}
		case "backspace":
			if len(m.query) > 0 {
				m.query = m.query[:len(m.query)-1]
				m.cursor = 0
			}
		case " ":
			if len(filtered) > 0 {
				item := filtered[m.cursor]
				if m.selected[item.Value] {
					delete(m.selected, item.Value)
				} else {
					m.selected[item.Value] = true
				}
			}
		default:
			if len(msg.Runes) > 0 && msg.Alt == false && msg.Type == tea.KeyRunes {
				m.query += string(msg.Runes)
				m.cursor = 0
			}
		}
	}
	// 输入变化后需要重新校正光标，避免落在无效位置。
	if filtered := m.filtered(); len(filtered) == 0 {
		m.cursor = 0
	} else if m.cursor >= len(filtered) {
		m.cursor = len(filtered) - 1
	}
	return m, nil
}

// View 渲染当前多选界面。
func (m multiSelectModel) View() string {
	copy := Messages()
	var lines []string

	stateIcon := Green + "◆" + Reset
	if m.cancelled {
		stateIcon = Red + "■" + Reset
	}
	if m.submitted {
		stateIcon = Green + "◇" + Reset
	}

	lines = append(lines, fmt.Sprintf("%s  %s", stateIcon, m.message))

	if m.cancelled {
		lines = append(lines, fmt.Sprintf("%s  %s%s%s", dimBar(), Dim, copy.Cancelled, Reset))
		return strings.Join(lines, "\n")
	}

	if m.submitted {
		lines = append(lines, fmt.Sprintf("%s  %s%s%s", dimBar(), Dim, strings.Join(m.selectedLabels(), ", "), Reset))
		return strings.Join(lines, "\n")
	}

	filtered := m.filtered()
	lines = append(lines, dimBar())
	if m.lockedSection != nil && len(m.lockedSection.Items) > 0 {
		lines = append(lines, fmt.Sprintf("%s  %s%s%s %s── %s%s", dimBar(), Bold, m.lockedSection.Title, Reset, Dim, copy.AlwaysIncludedSuffix, Reset))
		for _, item := range m.lockedSection.Items {
			lines = append(lines, fmt.Sprintf("%s    %s•%s %s%s%s", dimBar(), Green, Reset, Bold, item.Label, Reset))
		}
		lines = append(lines, dimBar())
	}

	lines = append(lines, fmt.Sprintf("%s  %s%s:%s %s%s%s", dimBar(), Dim, copy.SearchLabel, Reset, m.query, inverseCursor(), Reset))
	lines = append(lines, fmt.Sprintf("%s  %s%s%s", dimBar(), Dim, copy.MultiSelectHelp, Reset))
	lines = append(lines, dimBar())

	if len(filtered) == 0 {
		lines = append(lines, fmt.Sprintf("%s  %s%s%s", dimBar(), Dim, copy.NoMatchesFound, Reset))
	} else {
		start := max(0, min(m.cursor-m.maxVisible/2, len(filtered)-m.maxVisible))
		end := min(len(filtered), start+m.maxVisible)
		for i, item := range filtered[start:end] {
			actual := start + i
			prefix := " "
			if actual == m.cursor {
				prefix = Cyan + "❯" + Reset
			}

			radio := Dim + "○" + Reset
			if m.selected[item.Value] {
				radio = Green + "●" + Reset
			}

			label := item.Label
			if actual == m.cursor {
				label = underline(label)
			}
			hint := ""
			if item.Hint != "" {
				hint = fmt.Sprintf(" %s(%s)%s", Dim, item.Hint, Reset)
			}
			lines = append(lines, fmt.Sprintf("%s %s %s %s%s", dimBar(), prefix, radio, label, hint))
		}
	}

	lines = append(lines, dimBar())
	selected := m.selectedLabels()
	if len(selected) == 0 {
		lines = append(lines, fmt.Sprintf("%s  %s%s: %s%s", dimBar(), Dim, copy.SelectedLabel, copy.SelectedNone, Reset))
	} else {
		summary := strings.Join(selected, ", ")
		if len(selected) > 3 {
			summary = fmt.Sprintf(copy.MoreSelectedFmt, strings.Join(selected[:3], ", "), len(selected)-3)
		}
		lines = append(lines, fmt.Sprintf("%s  %s%s:%s %s", dimBar(), Green, copy.SelectedLabel, Reset, summary))
	}
	lines = append(lines, Dim+"└"+Reset)

	return strings.Join(lines, "\n")
}

// filtered 根据当前查询词返回可见选项。
func (m multiSelectModel) filtered() []Option {
	if m.query == "" {
		return m.items
	}
	query := strings.ToLower(m.query)
	filtered := make([]Option, 0, len(m.items))
	for _, item := range m.items {
		if strings.Contains(strings.ToLower(item.Label), query) || strings.Contains(strings.ToLower(item.Value), query) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// selectedLabels 返回当前已选项的展示标签。
func (m multiSelectModel) selectedLabels() []string {
	var selected []string
	if m.lockedSection != nil {
		for _, item := range m.lockedSection.Items {
			selected = append(selected, item.Label)
		}
	}
	for _, item := range m.items {
		if m.selected[item.Value] {
			selected = append(selected, item.Label)
		}
	}
	return selected
}

// lockedValues 返回固定包含区域的值列表。
func (m multiSelectModel) lockedValues() []string {
	if m.lockedSection == nil {
		return nil
	}
	values := make([]string, 0, len(m.lockedSection.Items))
	for _, item := range m.lockedSection.Items {
		values = append(values, item.Value)
	}
	return values
}

// dimBar 返回统一的左侧竖线样式。
func dimBar() string {
	return Dim + "│" + Reset
}

// inverseCursor 渲染查询框中的反白光标。
func inverseCursor() string {
	return "\x1b[7m \x1b[27m"
}

// underline 给当前高亮文本加下划线。
func underline(value string) string {
	return "\x1b[4m" + value + "\x1b[24m"
}

// min 返回两个整数中的较小值。
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// max 返回两个整数中的较大值。
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
