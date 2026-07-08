package updateapp

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestUpdateDownloadsTarballVerifiesAndReplacesExecutable(t *testing.T) {
	exePath := filepath.Join(t.TempDir(), "wikimesh")
	if runtime.GOOS == "windows" {
		exePath += ".exe"
	}
	if err := os.WriteFile(exePath, []byte("old"), 0o755); err != nil {
		t.Fatalf("write executable: %v", err)
	}

	archive := makeTarGz(t, "wikimesh-linux-amd64", []byte("new"))
	sum := sha256.Sum256(archive)
	asset := "wikimesh-linux-amd64.tar.gz"
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path)
		switch r.URL.Path {
		case "/checksums.txt":
			fmt.Fprintf(w, "%x  ./%s\n", sum, asset)
		case "/" + asset:
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	service := NewService(ServiceOptions{
		BaseURL:        server.URL,
		ExecutablePath: exePath,
		GOOS:           "linux",
		GOARCH:         "amd64",
	})
	result, err := service.Update(context.Background())
	if err != nil {
		t.Fatalf("Update error = %v", err)
	}

	data, err := os.ReadFile(exePath)
	if err != nil {
		t.Fatalf("read updated executable: %v", err)
	}
	if string(data) != "new" {
		t.Fatalf("updated executable content = %q, want new", string(data))
	}
	if result.Asset != asset {
		t.Fatalf("result Asset = %q, want %q", result.Asset, asset)
	}
	if result.Path != exePath {
		t.Fatalf("result Path = %q, want %q", result.Path, exePath)
	}
	if strings.Join(requests, ",") != "/checksums.txt,/"+asset {
		t.Fatalf("requests = %#v", requests)
	}
}

func TestUpdateDownloadsZipVerifiesAndDefersWindowsReplacement(t *testing.T) {
	root := t.TempDir()
	exePath := filepath.Join(root, "wikimesh.exe")
	if err := os.WriteFile(exePath, []byte("old"), 0o755); err != nil {
		t.Fatalf("write executable: %v", err)
	}

	archive := makeZip(t, "wikimesh-windows-amd64.exe", []byte("new"))
	sum := sha256.Sum256(archive)
	asset := "wikimesh-windows-amd64.zip"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/checksums.txt":
			fmt.Fprintf(w, "%x  %s\n", sum, asset)
		case "/" + asset:
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	var commands [][]string
	service := NewService(ServiceOptions{
		BaseURL:        server.URL,
		ExecutablePath: exePath,
		GOOS:           "windows",
		GOARCH:         "amd64",
		RunDetached: func(name string, args ...string) error {
			commands = append(commands, append([]string{name}, args...))
			return nil
		},
	})
	result, err := service.Update(context.Background())
	if err != nil {
		t.Fatalf("Update error = %v", err)
	}

	data, err := os.ReadFile(exePath)
	if err != nil {
		t.Fatalf("read executable: %v", err)
	}
	if string(data) != "old" {
		t.Fatalf("windows update should defer replacement while process is running, got %q", string(data))
	}
	if result.Asset != asset || !result.Deferred {
		t.Fatalf("result = %#v, want asset %q with Deferred=true", result, asset)
	}
	if len(commands) != 1 || commands[0][0] != "cmd" {
		t.Fatalf("detached commands = %#v", commands)
	}
}

func TestUpdateRejectsChecksumMismatch(t *testing.T) {
	exePath := filepath.Join(t.TempDir(), "wikimesh")
	if err := os.WriteFile(exePath, []byte("old"), 0o755); err != nil {
		t.Fatalf("write executable: %v", err)
	}

	archive := makeTarGz(t, "wikimesh-linux-amd64", []byte("new"))
	asset := "wikimesh-linux-amd64.tar.gz"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/checksums.txt":
			fmt.Fprintf(w, "%064x  ./%s\n", 0, asset)
		case "/" + asset:
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	service := NewService(ServiceOptions{
		BaseURL:        server.URL,
		ExecutablePath: exePath,
		GOOS:           "linux",
		GOARCH:         "amd64",
	})
	if _, err := service.Update(context.Background()); err == nil || !strings.Contains(err.Error(), "checksum verification failed") {
		t.Fatalf("Update error = %v, want checksum failure", err)
	}

	data, err := os.ReadFile(exePath)
	if err != nil {
		t.Fatalf("read executable: %v", err)
	}
	if string(data) != "old" {
		t.Fatalf("executable changed after checksum failure: %q", string(data))
	}
}

func TestSelectAssetMatchesWikimeshReleaseNames(t *testing.T) {
	checksums := strings.Join([]string{
		"aaa  ./wikimesh-linux-amd64.tar.gz",
		"bbb  ./wikimesh-darwin-arm64.tar.gz",
		"ccc  ./wikimesh-windows-amd64.zip",
	}, "\n")

	tests := []struct {
		name   string
		goos   string
		goarch string
		want   string
	}{
		{name: "linux", goos: "linux", goarch: "amd64", want: "wikimesh-linux-amd64.tar.gz"},
		{name: "darwin arm64", goos: "darwin", goarch: "arm64", want: "wikimesh-darwin-arm64.tar.gz"},
		{name: "windows uses zip", goos: "windows", goarch: "amd64", want: "wikimesh-windows-amd64.zip"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := selectAsset(checksums, tt.goos, tt.goarch)
			if err != nil {
				t.Fatalf("selectAsset error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("selectAsset = %q, want %q", got, tt.want)
			}
		})
	}
}

func makeTarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	header := &tar.Header{Name: name, Mode: 0o755, Size: int64(len(content))}
	if err := tw.WriteHeader(header); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("write tar content: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}

func makeZip(t *testing.T, name string, content []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	writer, err := zw.Create(name)
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	if _, err := writer.Write(content); err != nil {
		t.Fatalf("write zip entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}
