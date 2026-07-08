package updatecmd

import (
	"os"
	"strings"
	"testing"
)

func TestBuildWorkflowPublishesChecksumsArtifact(t *testing.T) {
	data, err := os.ReadFile("../../../.github/workflows/build.yml")
	if err != nil {
		t.Fatalf("ReadFile(build.yml) error = %v", err)
	}
	workflow := string(data)

	for _, want := range []string{
		"shasum -a 256 wikimesh-${{ matrix.asset }}.tar.gz",
		"Get-FileHash -Algorithm SHA256 $file",
		"name: checksums.txt",
		"path: .wikimesh/dist/checksums.txt",
		"needs: [package, package-windows]",
	} {
		if !strings.Contains(workflow, want) {
			t.Fatalf("build workflow missing %q", want)
		}
	}
}

func TestBuildWorkflowPublishesReleaseArchivesAndChecksumsOnTags(t *testing.T) {
	data, err := os.ReadFile("../../../.github/workflows/build.yml")
	if err != nil {
		t.Fatalf("ReadFile(build.yml) error = %v", err)
	}
	workflow := string(data)

	for _, want := range []string{
		`tags:`,
		`- "v*"`,
		"softprops/action-gh-release@v2",
		"startsWith(github.ref, 'refs/tags/')",
		".wikimesh/release/wikimesh-linux-amd64.tar.gz",
		".wikimesh/release/wikimesh-linux-arm64.tar.gz",
		".wikimesh/release/wikimesh-darwin-amd64.tar.gz",
		".wikimesh/release/wikimesh-darwin-arm64.tar.gz",
		".wikimesh/release/wikimesh-windows-amd64.zip",
		".wikimesh/dist/checksums.txt",
	} {
		if !strings.Contains(workflow, want) {
			t.Fatalf("release workflow missing %q", want)
		}
	}
	legacyAsset := "darwin-arm64-" + "metal"
	if strings.Contains(workflow, legacyAsset) {
		t.Fatalf("release workflow should not publish legacy darwin arm64 Metal asset")
	}
}

func TestBuildWorkflowDoesNotInstallLlamaRuntimeDuringPackaging(t *testing.T) {
	data, err := os.ReadFile("../../../.github/workflows/build.yml")
	if err != nil {
		t.Fatalf("ReadFile(build.yml) error = %v", err)
	}
	workflow := string(data)

	for _, forbidden := range []string{
		"Install llama.cpp libraries",
		"make install-llama",
		"model lib install",
	} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("build workflow should not install llama.cpp runtime during packaging, found %q", forbidden)
		}
	}
}
