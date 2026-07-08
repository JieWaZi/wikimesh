package scripts

import (
	"os"
	"strings"
	"testing"
)

func TestPowerShellInstallerDoesNotSplitSessionPathWithoutNullGuard(t *testing.T) {
	content, err := os.ReadFile("install.ps1")
	if err != nil {
		t.Fatalf("read install.ps1: %v", err)
	}

	script := string(content)
	if strings.Contains(script, "$env:Path.Split(") {
		t.Fatalf("install.ps1 must not call $env:Path.Split directly; empty Path triggers a null-value method error")
	}
}

func TestPowerShellInstallerDoesNotUseRuntimeInformationForArchitecture(t *testing.T) {
	content, err := os.ReadFile("install.ps1")
	if err != nil {
		t.Fatalf("read install.ps1: %v", err)
	}

	script := string(content)
	if strings.Contains(script, "RuntimeInformation]::OSArchitecture") {
		t.Fatalf("install.ps1 must not use RuntimeInformation.OSArchitecture; Windows PowerShell can return null there")
	}
}

func TestInstallScriptsAreAdaptedForWikimesh(t *testing.T) {
	for _, path := range []string{"install.sh", "install.ps1"} {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		script := string(content)
		if strings.Contains(script, "zatools") || strings.Contains(script, "ZATOOLS") {
			t.Fatalf("%s still contains zatools-specific text", path)
		}
		if !strings.Contains(script, "wikimesh") || !strings.Contains(script, "WIKIMESH") {
			t.Fatalf("%s does not contain wikimesh-specific defaults", path)
		}
	}
}

func TestInstallScriptUsesPlainDarwinARM64Asset(t *testing.T) {
	content, err := os.ReadFile("install.sh")
	if err != nil {
		t.Fatalf("read install.sh: %v", err)
	}
	script := string(content)
	legacyAsset := "darwin-arm64-" + "metal"
	if strings.Contains(script, legacyAsset) {
		t.Fatalf("install.sh should not reference legacy darwin arm64 Metal asset")
	}
	if !strings.Contains(script, "darwin-arm64.tar.gz") {
		t.Fatalf("install.sh should resolve darwin arm64 to wikimesh-darwin-arm64.tar.gz")
	}
}
