package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestVersionCommandPrintsBuildInfo(t *testing.T) {
	oldVersion, oldBuildTime := Version, BuildTime
	Version = "v9.9.9"
	BuildTime = "2026-07-08T00:00:00Z"
	t.Cleanup(func() {
		Version = oldVersion
		BuildTime = oldBuildTime
	})

	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(version) error = %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"version: v9.9.9",
		"build_time: 2026-07-08T00:00:00Z",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("version output missing %q in:\n%s", want, got)
		}
	}
}

func TestMakefileInjectsVersionAndBuildTime(t *testing.T) {
	data, err := os.ReadFile("../../Makefile")
	if err != nil {
		t.Fatalf("ReadFile(Makefile) error = %v", err)
	}
	makefile := string(data)

	for _, want := range []string{
		"VERSION ?=",
		"BUILD_TIME ?=",
		"-ldflags",
		"github.com/JieWaZi/wikimesh/internal/cli.Version=$(VERSION)",
		"github.com/JieWaZi/wikimesh/internal/cli.BuildTime=$(BUILD_TIME)",
	} {
		if !strings.Contains(makefile, want) {
			t.Fatalf("Makefile missing %q", want)
		}
	}
}
