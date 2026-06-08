package ui

import (
	"io"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestSelectOneModelUpdateAndView(t *testing.T) {
	model := selectOneModel{
		message: "选择一个",
		items: []Option{
			{Value: "a", Label: "Alpha", Hint: "第一个"},
			{Value: "b", Label: "Beta"},
		},
	}

	if cmd := model.Init(); cmd != nil {
		t.Fatalf("Init() = %#v, want nil", cmd)
	}

	updatedModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updatedModel.(selectOneModel)
	if model.cursor != 1 {
		t.Fatalf("cursor after down = %d, want 1", model.cursor)
	}

	view := model.View()
	if !strings.Contains(view, "选择一个") || !strings.Contains(view, "Beta") {
		t.Fatalf("View() = %q", view)
	}

	cancelledModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = cancelledModel.(selectOneModel)
	if !model.cancelled || cmd == nil {
		t.Fatalf("cancelled=%v cmd=%v, want cancelled quit state", model.cancelled, cmd)
	}
}

func TestMultiSelectModelHelpersUpdateAndView(t *testing.T) {
	model := multiSelectModel{
		message:    "选择多个",
		items:      []Option{{Value: "alpha", Label: "Alpha"}, {Value: "beta", Label: "Beta"}},
		maxVisible: 8,
		required:   true,
		selected:   map[string]bool{},
		lockedSection: &LockedSection{
			Title: "固定",
			Items: []Option{{Value: "base", Label: "Base"}},
		},
	}

	if cmd := model.Init(); cmd != nil {
		t.Fatalf("Init() = %#v, want nil", cmd)
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	model = updated.(multiSelectModel)
	if model.query != "a" {
		t.Fatalf("query = %q, want %q", model.query, "a")
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(multiSelectModel)
	if !model.selected["alpha"] {
		t.Fatalf("selected = %#v, want alpha selected", model.selected)
	}

	if got := model.filtered(); len(got) != 2 {
		t.Fatalf("filtered() len = %d, want 2", len(got))
	}
	if got := model.selectedLabels(); len(got) != 2 || got[0] != "Base" || got[1] != "Alpha" {
		t.Fatalf("selectedLabels() = %#v", got)
	}
	if got := model.lockedValues(); len(got) != 1 || got[0] != "base" {
		t.Fatalf("lockedValues() = %#v", got)
	}

	view := model.View()
	if !strings.Contains(view, "选择多个") || !strings.Contains(view, "固定") {
		t.Fatalf("View() = %q", view)
	}

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(multiSelectModel)
	if !model.submitted || cmd == nil {
		t.Fatalf("submitted=%v cmd=%v, want submitted quit state", model.submitted, cmd)
	}

	submittedView := model.View()
	if !strings.Contains(submittedView, "Base, Alpha") {
		t.Fatalf("submitted View() = %q", submittedView)
	}

	cancelled, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	model = cancelled.(multiSelectModel)
	if !model.cancelled || cmd == nil {
		t.Fatalf("cancelled=%v cmd=%v, want cancelled quit state", model.cancelled, cmd)
	}
	if got := dimBar(); !strings.Contains(got, "│") {
		t.Fatalf("dimBar() = %q", got)
	}
	if got := inverseCursor(); !strings.Contains(got, "\x1b[7m") {
		t.Fatalf("inverseCursor() = %q", got)
	}
	if got := underline("x"); !strings.Contains(got, "x") {
		t.Fatalf("underline() = %q", got)
	}
	if min(1, 2) != 1 || max(1, 2) != 2 {
		t.Fatal("min/max returned unexpected values")
	}
}

func TestStepPrinterAndShowLogo(t *testing.T) {
	output := captureUIStdout(t, func() {
		printer := &StepPrinter{}
		printer.Start("starting")
		printer.Stop("done")
		Step("step")
		Note("title", []string{"line1", "line2"})
		ShowLogo()
	})

	for _, want := range []string{"starting", "done", "step", "title", "line1", "WIKIMESH"} {
		if !strings.Contains(output, want) {
			t.Fatalf("captured output missing %q in %q", want, output)
		}
	}

	if printer := NewStepPrinter(); printer == nil {
		t.Fatal("NewStepPrinter() returned nil")
	}
}

func captureUIStdout(t *testing.T, fn func()) string {
	t.Helper()

	original := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe error = %v", err)
	}

	os.Stdout = writer
	fn()
	_ = writer.Close()
	os.Stdout = original

	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("io.ReadAll error = %v", err)
	}
	_ = reader.Close()
	return string(data)
}
