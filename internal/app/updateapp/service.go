package updateapp

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	defaultOwner = "JieWaZi"
	defaultRepo  = "wikimesh"
	binaryBase   = "wikimesh"
)

// ServiceOptions 配置自更新服务，测试可用这些字段替换外部依赖。
type ServiceOptions struct {
	BaseURL        string
	ExecutablePath string
	GOOS           string
	GOARCH         string
	HTTPClient     *http.Client
	RunDetached    func(name string, args ...string) error
}

// Result 描述一次自更新结果。
type Result struct {
	Asset    string
	Path     string
	Deferred bool
}

// Service 从 GitHub Release 下载并替换当前正在运行的 wikimesh 二进制。
type Service struct {
	baseURL        string
	executablePath string
	goos           string
	goarch         string
	httpClient     *http.Client
	runDetached    func(name string, args ...string) error
}

// NewService 构造自更新服务。
func NewService(opts ServiceOptions) *Service {
	baseURL := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	if baseURL == "" {
		owner := strings.TrimSpace(os.Getenv("WIKIMESH_OWNER"))
		if owner == "" {
			owner = defaultOwner
		}
		repo := strings.TrimSpace(os.Getenv("WIKIMESH_REPO"))
		if repo == "" {
			repo = defaultRepo
		}
		version := strings.TrimSpace(os.Getenv("VERSION"))
		if version != "" {
			baseURL = fmt.Sprintf("https://github.com/%s/%s/releases/download/%s", owner, repo, version)
		} else {
			baseURL = fmt.Sprintf("https://github.com/%s/%s/releases/latest/download", owner, repo)
		}
	}

	goos := opts.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	goarch := opts.GOARCH
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	client := opts.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	runDetached := opts.RunDetached
	if runDetached == nil {
		runDetached = startDetached
	}

	return &Service{
		baseURL:        baseURL,
		executablePath: opts.ExecutablePath,
		goos:           goos,
		goarch:         goarch,
		httpClient:     client,
		runDetached:    runDetached,
	}
}

// Update 下载最新匹配产物，校验 checksum，并替换当前二进制。
func (s *Service) Update(ctx context.Context) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	executablePath := s.executablePath
	if strings.TrimSpace(executablePath) == "" {
		path, err := os.Executable()
		if err != nil {
			return Result{}, fmt.Errorf("解析当前可执行文件: %w", err)
		}
		executablePath = path
	}
	executablePath, err := filepath.Abs(executablePath)
	if err != nil {
		return Result{}, fmt.Errorf("解析可执行文件路径: %w", err)
	}

	checksums, err := s.downloadText(ctx, s.baseURL+"/checksums.txt")
	if err != nil {
		return Result{}, err
	}
	asset, expectedHash, err := selectAsset(checksums, s.goos, s.goarch)
	if err != nil {
		return Result{}, err
	}

	tempDir, err := os.MkdirTemp("", "wikimesh-update-*")
	if err != nil {
		return Result{}, fmt.Errorf("创建临时目录: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(tempDir)
		}
	}()

	archivePath := filepath.Join(tempDir, asset)
	if err := s.downloadFile(ctx, s.baseURL+"/"+asset, archivePath); err != nil {
		return Result{}, err
	}
	if err := verifySHA256(archivePath, expectedHash); err != nil {
		return Result{}, err
	}

	extractedPath, err := extractBinary(archivePath, tempDir, binaryNameForAsset(asset))
	if err != nil {
		return Result{}, err
	}
	if s.goos == "windows" {
		if err := s.scheduleWindowsReplace(tempDir, extractedPath, executablePath); err != nil {
			return Result{}, err
		}
		cleanup = false
		return Result{Asset: asset, Path: executablePath, Deferred: true}, nil
	}

	if err := replaceExecutable(extractedPath, executablePath); err != nil {
		return Result{}, err
	}
	return Result{Asset: asset, Path: executablePath}, nil
}

func (s *Service) downloadText(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("创建请求: %w", err)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("下载 %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("下载 %s: 非预期状态 %s", url, resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取 %s: %w", url, err)
	}
	return string(data), nil
}

func (s *Service) downloadFile(ctx context.Context, url, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("创建请求: %w", err)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("下载 %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("下载 %s: 非预期状态 %s", url, resp.Status)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("创建 %s: %w", path, err)
	}
	defer file.Close()
	if _, err := io.Copy(file, resp.Body); err != nil {
		return fmt.Errorf("写入 %s: %w", path, err)
	}
	return nil
}

func selectAsset(checksums, goos, goarch string) (string, string, error) {
	want, err := assetName(goos, goarch)
	if err != nil {
		return "", "", err
	}
	var selectedHash string
	count := 0
	for _, line := range strings.Split(checksums, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		candidate := strings.TrimPrefix(fields[1], "./")
		if candidate == want {
			count++
			selectedHash = strings.ToLower(fields[0])
		}
	}
	switch count {
	case 0:
		return "", "", fmt.Errorf("checksums.txt 中没有匹配当前平台的 release 产物")
	case 1:
		return want, selectedHash, nil
	default:
		return "", "", fmt.Errorf("checksums.txt 中有多个匹配当前平台的 release 产物")
	}
}

func assetName(goos, goarch string) (string, error) {
	switch {
	case goos == "linux" && (goarch == "amd64" || goarch == "arm64"):
		return fmt.Sprintf("%s-linux-%s.tar.gz", binaryBase, goarch), nil
	case goos == "darwin" && goarch == "amd64":
		return binaryBase + "-darwin-amd64.tar.gz", nil
	case goos == "darwin" && goarch == "arm64":
		return binaryBase + "-darwin-arm64.tar.gz", nil
	case goos == "windows" && goarch == "amd64":
		return binaryBase + "-windows-amd64.zip", nil
	default:
		return "", fmt.Errorf("当前平台暂无 release 产物: %s/%s", goos, goarch)
	}
}

func verifySHA256(path, expectedHash string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("打开 %s: %w", path, err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("计算 %s checksum: %w", path, err)
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if actual != strings.ToLower(expectedHash) {
		return fmt.Errorf("checksum verification failed for %s", filepath.Base(path))
	}
	return nil
}

func extractBinary(archivePath, targetDir, name string) (string, error) {
	switch {
	case strings.HasSuffix(archivePath, ".tar.gz"):
		return extractTarGzBinary(archivePath, targetDir, name)
	case strings.HasSuffix(archivePath, ".zip"):
		return extractZipBinary(archivePath, targetDir, name)
	default:
		return "", fmt.Errorf("不支持的 release 归档格式: %s", filepath.Base(archivePath))
	}
}

func extractTarGzBinary(archivePath, targetDir, name string) (string, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("打开归档: %w", err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return "", fmt.Errorf("读取 gzip 归档: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("读取 tar 归档: %w", err)
		}
		if header.Typeflag != tar.TypeReg || filepath.Base(header.Name) != name {
			continue
		}
		return writeExtractedBinary(targetDir, name, tr)
	}
	return "", fmt.Errorf("归档中没有找到 %s", name)
}

func extractZipBinary(archivePath, targetDir, name string) (string, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", fmt.Errorf("读取 zip 归档: %w", err)
	}
	defer reader.Close()
	for _, file := range reader.File {
		if file.FileInfo().IsDir() || filepath.Base(file.Name) != name {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return "", fmt.Errorf("打开 zip 文件项: %w", err)
		}
		path, copyErr := writeExtractedBinary(targetDir, name, rc)
		closeErr := rc.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if closeErr != nil {
			return "", fmt.Errorf("关闭 zip 文件项: %w", closeErr)
		}
		return path, nil
	}
	return "", fmt.Errorf("归档中没有找到 %s", name)
}

func writeExtractedBinary(targetDir, name string, reader io.Reader) (string, error) {
	outPath := filepath.Join(targetDir, name)
	out, err := os.OpenFile(outPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return "", fmt.Errorf("创建解压二进制: %w", err)
	}
	_, copyErr := io.Copy(out, reader)
	closeErr := out.Close()
	if copyErr != nil {
		return "", fmt.Errorf("解压二进制: %w", copyErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("关闭解压二进制: %w", closeErr)
	}
	return outPath, nil
}

func replaceExecutable(sourcePath, executablePath string) error {
	targetDir := filepath.Dir(executablePath)
	tempTarget, err := os.CreateTemp(targetDir, ".wikimesh-update-*")
	if err != nil {
		return fmt.Errorf("创建替换文件: %w", err)
	}
	tempTargetPath := tempTarget.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tempTargetPath)
		}
	}()

	source, err := os.Open(sourcePath)
	if err != nil {
		_ = tempTarget.Close()
		return fmt.Errorf("打开解压二进制: %w", err)
	}
	if _, err := io.Copy(tempTarget, source); err != nil {
		_ = source.Close()
		_ = tempTarget.Close()
		return fmt.Errorf("写入替换文件: %w", err)
	}
	if err := source.Close(); err != nil {
		_ = tempTarget.Close()
		return fmt.Errorf("关闭解压二进制: %w", err)
	}
	if err := tempTarget.Chmod(0o755); err != nil {
		_ = tempTarget.Close()
		return fmt.Errorf("设置替换文件权限: %w", err)
	}
	if err := tempTarget.Close(); err != nil {
		return fmt.Errorf("关闭替换文件: %w", err)
	}
	if err := os.Rename(tempTargetPath, executablePath); err != nil {
		return fmt.Errorf("替换 %s: %w", executablePath, err)
	}
	cleanup = false
	return nil
}

func (s *Service) scheduleWindowsReplace(tempDir, sourcePath, executablePath string) error {
	scriptPath := filepath.Join(tempDir, "replace-wikimesh.ps1")
	script := fmt.Sprintf(`$ErrorActionPreference = "SilentlyContinue"
$Source = %s
$Target = %s
$TempDir = %s
for ($i = 0; $i -lt 30; $i++) {
    Start-Sleep -Milliseconds 500
    Move-Item -Force -Path $Source -Destination $Target
    if ($?) {
        Remove-Item -Recurse -Force $TempDir
        exit 0
    }
}
exit 1
`, psQuote(sourcePath), psQuote(executablePath), psQuote(tempDir))
	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		return fmt.Errorf("写入替换脚本: %w", err)
	}
	return s.runDetached("cmd", "/C", "start", "/B", "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", scriptPath)
}

func binaryNameForAsset(asset string) string {
	name := strings.TrimSuffix(asset, ".tar.gz")
	name = strings.TrimSuffix(name, ".zip")
	if strings.Contains(name, "windows") {
		return name + ".exe"
	}
	return name
}

func psQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func startDetached(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	return cmd.Start()
}
