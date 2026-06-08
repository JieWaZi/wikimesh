package qmdcmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JieWaZi/wikimesh/internal/cli/common"
)

func TestWorkspaceQMDConfigPathUsesCurrentWorkspace(t *testing.T) {
	root, gotPath := workspaceQMDConfigPath()
	if root != filepath.Dir(filepath.Dir(common.DefaultQMDConfigPath)) {
		t.Fatalf("root = %q, want workspace .wikimesh parent", root)
	}
	if gotPath != common.DefaultQMDConfigPath {
		t.Fatalf("path = %q, want %q", gotPath, common.DefaultQMDConfigPath)
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
