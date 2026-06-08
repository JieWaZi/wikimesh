$ErrorActionPreference = "Stop"

$Owner = if ($env:WIKIMESH_OWNER) { $env:WIKIMESH_OWNER } else { "JieWaZi" }
$Repo = if ($env:WIKIMESH_REPO) { $env:WIKIMESH_REPO } else { "wikimesh" }
$BinaryName = if ($env:WIKIMESH_BINARY) { $env:WIKIMESH_BINARY } else { "wikimesh" }
$Version = if ($env:VERSION) { $env:VERSION } else { "" }
$InstallDir = if ($env:WIKIMESH_INSTALL_DIR) {
    $env:WIKIMESH_INSTALL_DIR
} else {
    Join-Path $env:LOCALAPPDATA "Programs\wikimesh\bin"
}

function Resolve-BaseUrl {
    if ($Version) {
        return "https://github.com/$Owner/$Repo/releases/download/$Version"
    }

    return "https://github.com/$Owner/$Repo/releases/latest/download"
}

function Resolve-Arch {
    $arch = if ($env:PROCESSOR_ARCHITEW6432) {
        $env:PROCESSOR_ARCHITEW6432
    } else {
        $env:PROCESSOR_ARCHITECTURE
    }

    if ([string]::IsNullOrEmpty($arch)) {
        throw "Unable to determine Windows architecture"
    }

    $arch = $arch.ToUpperInvariant()
    switch ($arch) {
        "AMD64" { return "amd64" }
        default { throw "Unsupported architecture: $arch" }
    }
}

function Split-PathEntries {
    param(
        [string]$PathValue
    )

    if ([string]::IsNullOrEmpty($PathValue)) {
        return @()
    }

    return $PathValue.Split(';', [System.StringSplitOptions]::RemoveEmptyEntries)
}

function Resolve-Checksum {
    param(
        [string]$ChecksumsPath,
        [string]$Asset
    )

    foreach ($line in Get-Content $ChecksumsPath) {
        $parts = $line -split '\s+'
        if ($parts.Length -lt 2) {
            continue
        }

        $candidate = $parts[1] -replace '^\./', ''
        if ($candidate -eq $Asset) {
            return $parts[0].ToLowerInvariant()
        }
    }

    throw "Unable to find checksum for $Asset"
}

$ResolvedArch = Resolve-Arch
$BaseUrl = Resolve-BaseUrl
$Asset = "${BinaryName}-windows-${ResolvedArch}.zip"
$ArchiveBinaryName = "${BinaryName}-windows-${ResolvedArch}.exe"
$TempDir = Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid().ToString())
$ChecksumsPath = Join-Path $TempDir "checksums.txt"

New-Item -ItemType Directory -Force -Path $TempDir | Out-Null

try {
    Invoke-WebRequest -Uri "$BaseUrl/checksums.txt" -OutFile $ChecksumsPath
    $ArchivePath = Join-Path $TempDir $Asset

    Write-Host "Downloading $Asset"
    Invoke-WebRequest -Uri "$BaseUrl/$Asset" -OutFile $ArchivePath

    $expectedHash = Resolve-Checksum -ChecksumsPath $ChecksumsPath -Asset $Asset
    $actualHash = (Get-FileHash -Path $ArchivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actualHash -ne $expectedHash) {
        throw "Checksum verification failed for $Asset"
    }

    Expand-Archive -Path $ArchivePath -DestinationPath $TempDir -Force

    $BinaryPath = Join-Path $TempDir $ArchiveBinaryName
    if (-not (Test-Path $BinaryPath)) {
        throw "Archive did not contain $ArchiveBinaryName"
    }

    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    Copy-Item -Path $BinaryPath -Destination (Join-Path $InstallDir "$BinaryName.exe") -Force

    $UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
    $NormalizedInstallDir = $InstallDir.TrimEnd('\')
    $PathEntries = Split-PathEntries -PathValue $UserPath

    $HasUserPath = $false
    foreach ($entry in $PathEntries) {
        if ($entry.TrimEnd('\') -ieq $NormalizedInstallDir) {
            $HasUserPath = $true
            break
        }
    }

    if (-not $HasUserPath) {
        $NewUserPath = if ($UserPath) { "$UserPath;$InstallDir" } else { $InstallDir }
        [Environment]::SetEnvironmentVariable("Path", $NewUserPath, "User")
    }

    $SessionEntries = Split-PathEntries -PathValue $env:Path
    $HasSessionPath = $false
    foreach ($entry in $SessionEntries) {
        if ($entry.TrimEnd('\') -ieq $NormalizedInstallDir) {
            $HasSessionPath = $true
            break
        }
    }

    if (-not $HasSessionPath) {
        $env:Path = if ($env:Path) { "$InstallDir;$env:Path" } else { $InstallDir }
    }

    Write-Host "Installed $BinaryName to $InstallDir"
    Write-Host "Run: $BinaryName --help"
}
finally {
    if (Test-Path $TempDir) {
        Remove-Item -Recurse -Force $TempDir
    }
}
