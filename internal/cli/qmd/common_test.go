package qmdcmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JieWaZi/wikimesh/internal/app/qmdapp"
)

func TestWorkspaceQMDConfigPathUsesCurrentWorkspace(t *testing.T) {
	root, gotPath := workspaceQMDConfigPath()
	if root != filepath.Dir(filepath.Dir(qmdapp.DefaultConfigPath)) {
		t.Fatalf("root = %q, want workspace .wikimesh parent", root)
	}
	if gotPath != qmdapp.DefaultConfigPath {
		t.Fatalf("path = %q, want %q", gotPath, qmdapp.DefaultConfigPath)
	}
}

func TestQMDCommandsDoNotExposeProjectFlag(t *testing.T) {
	cmd := NewCommand()
	cmd.SetArgs([]string{"search", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute help: %v", err)
	}
	if strings.Contains(out.String(), "--project") {
		t.Fatalf("qmd help exposes --project:\n%s", out.String())
	}
}
