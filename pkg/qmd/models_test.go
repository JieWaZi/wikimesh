package qmd_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	qmd "github.com/JieWaZi/wikimesh/pkg/qmd"
)

func TestDownloadModelReportsByteProgress(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "remote.gguf")
	payload := []byte("fake gguf payload for progress")
	if err := os.WriteFile(source, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(dir, ".wikimesh", "models", "remote.gguf")

	var events []qmd.ModelDownloadProgress
	result, err := qmd.DownloadModelWithOptions(context.Background(), qmd.ModelRoleEmbed, "file://"+source, destination, qmd.ModelDownloadOptions{
		Progress: func(info qmd.ModelDownloadProgress) {
			events = append(events, info)
		},
	})
	if err != nil {
		t.Fatalf("DownloadModelWithOptions: %v", err)
	}
	if !result.Downloaded {
		t.Fatalf("Downloaded = false, want true")
	}
	if len(events) == 0 {
		t.Fatalf("progress events = 0, want byte progress")
	}
	last := events[len(events)-1]
	if last.Current != int64(len(payload)) || last.Total != int64(len(payload)) {
		t.Fatalf("last progress = %#v, want current and total %d", last, len(payload))
	}
}
