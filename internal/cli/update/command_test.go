package updatecmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/JieWaZi/wikimesh/internal/app/updateapp"
)

type fakeUpdater struct {
	result updateapp.Result
	err    error
}

func (f fakeUpdater) Update(context.Context) (updateapp.Result, error) {
	return f.result, f.err
}

func TestCommandPrintsUpdatedBinaryPath(t *testing.T) {
	cmd := NewCommandWithService(fakeUpdater{
		result: updateapp.Result{
			Asset: "wikimesh-linux-amd64.tar.gz",
			Path:  "/usr/local/bin/wikimesh",
		},
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute error = %v", err)
	}
	output := out.String()
	for _, want := range []string{"wikimesh-linux-amd64.tar.gz", "/usr/local/bin/wikimesh"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestCommandPrintsDeferredWindowsReplacement(t *testing.T) {
	cmd := NewCommandWithService(fakeUpdater{
		result: updateapp.Result{
			Asset:    "wikimesh-windows-amd64.zip",
			Path:     `C:\Users\admin\AppData\Local\Programs\wikimesh\bin\wikimesh.exe`,
			Deferred: true,
		},
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute error = %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "wikimesh-windows-amd64.zip") {
		t.Fatalf("deferred update output missing asset:\n%s", output)
	}
	if !strings.Contains(output, "重新打开") {
		t.Fatalf("deferred update output missing restart hint:\n%s", output)
	}
}

func TestCommandReturnsUpdaterError(t *testing.T) {
	cmd := NewCommandWithService(fakeUpdater{err: errors.New("download failed")})

	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "download failed") {
		t.Fatalf("Execute error = %v, want updater error", err)
	}
}
