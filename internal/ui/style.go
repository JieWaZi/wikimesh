package ui

// ANSI 样式常量统一收口在这里，避免散落魔法字符串。
const (
	Reset  = "\x1b[0m"
	Bold   = "\x1b[1m"
	Dim    = "\x1b[38;5;102m"
	Text   = "\x1b[38;5;145m"
	Cyan   = "\x1b[36m"
	Yellow = "\x1b[33m"
	Green  = "\x1b[32m"
	Red    = "\x1b[31m"
)
