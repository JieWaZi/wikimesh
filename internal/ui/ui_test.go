package ui

import (
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestMessagesAndHelpLocalization(t *testing.T) {
	if got := Messages().RootShort; got == "" {
		t.Fatal("Messages().RootShort returned empty string")
	}

	cmd := &cobra.Command{Use: "demo"}
	child := &cobra.Command{Use: "child"}
	cmd.AddCommand(child)
	ApplyLocalizedHelp(cmd)
	if flag := cmd.Flags().Lookup("help"); flag == nil || flag.Usage != "显示帮助" {
		t.Fatalf("help flag usage = %#v", flag)
	}
	if child.HelpFunc() == nil {
		t.Fatal("localized help was not applied recursively")
	}
}

func TestLogoAndCommandName(t *testing.T) {
	logo := Logo()
	if !strings.Contains(logo, "WIKIMESH") {
		t.Fatalf("Logo() = %q", logo)
	}

	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })

	os.Args = []string{"/usr/local/bin/wikimesh"}
	if got := CommandName(); got != "wikimesh" {
		t.Fatalf("CommandName = %q, want %q", got, "wikimesh")
	}

	os.Args = nil
	if got := CommandName(); got != "wikimesh" {
		t.Fatalf("CommandName(nil args) = %q, want %q", got, "wikimesh")
	}
}
