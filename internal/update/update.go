package update

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/bin/3xui-lite/internal/version"
)

const userAgent = "3xui-lite-updater/" + version.Version

// Info describes latest release vs current.
type Info struct {
	Current     string `json:"current"`
	Latest      string `json:"latest"`
	HasUpdate   bool   `json:"hasUpdate"`
	Name        string `json:"name"`
	Body        string `json:"body"`
	AssetName   string `json:"assetName"`
	AssetURL    string `json:"assetURL"`
	HTMLURL     string `json:"htmlUrl"`
	PublishedAt string `json:"publishedAt"`
	CanUpdate   bool   `json:"canUpdate"`
	Reason      string `json:"reason,omitempty"`
}

type ghRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Body        string `json:"body"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
	Assets      []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// CheckLatest queries GitHub Releases API.
func CheckLatest(repo string) (*Info, error) {
	if repo == "" {
		repo = version.Repo
	}
	url := "https://api.github.com/repos/" + repo + "/releases/latest"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{Timeout: 25 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 GitHub 失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("GitHub API %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	latest := strings.TrimPrefix(rel.TagName, "v")
	current := version.Version
	assetName, assetURL := pickAsset(rel)
	info := &Info{
		Current:     current,
		Latest:      latest,
		HasUpdate:   versionLess(current, latest),
		Name:        rel.Name,
		Body:        rel.Body,
		AssetName:   assetName,
		AssetURL:    assetURL,
		HTMLURL:     rel.HTMLURL,
		PublishedAt: rel.PublishedAt,
		CanUpdate:   true,
	}
	if runtime.GOOS != "linux" {
		info.CanUpdate = false
		info.Reason = "在线更新目前仅支持 Linux 服务器（systemd 部署）"
		return info, nil
	}
	if assetURL == "" {
		info.CanUpdate = false
		info.Reason = "最新 Release 中没有适合本系统的安装包 (" + runtime.GOOS + "/" + runtime.GOARCH + ")"
	}
	return info, nil
}

func pickAsset(rel ghRelease) (name, url string) {
	arch := runtime.GOARCH
	// map go arch to our package name
	pkg := "3xui-lite-linux-amd64.tar.gz"
	if runtime.GOOS == "linux" && arch == "arm64" {
		pkg = "3xui-lite-linux-arm64.tar.gz"
	}
	if runtime.GOOS == "windows" {
		// no windows package yet — try linux name won't work; look for any tar.gz amd64
		pkg = "3xui-lite-linux-amd64.tar.gz"
	}
	for _, a := range rel.Assets {
		if a.Name == pkg {
			return a.Name, a.BrowserDownloadURL
		}
	}
	// fallback: first matching tar.gz
	for _, a := range rel.Assets {
		n := strings.ToLower(a.Name)
		if strings.HasSuffix(n, ".tar.gz") && strings.Contains(n, "linux") && strings.Contains(n, arch) {
			return a.Name, a.BrowserDownloadURL
		}
	}
	for _, a := range rel.Assets {
		if strings.HasSuffix(strings.ToLower(a.Name), ".tar.gz") {
			return a.Name, a.BrowserDownloadURL
		}
	}
	return "", ""
}

// Apply downloads assetURL and replaces panel + cores under installDir.
// installDir is typically directory of the running executable.
func Apply(installDir, assetURL, assetName string) (string, error) {
	if assetURL == "" {
		return "", fmt.Errorf("无下载地址")
	}
	if installDir == "" {
		return "", fmt.Errorf("安装目录为空")
	}
	tmpDir, err := os.MkdirTemp("", "3xui-update-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	tarPath := filepath.Join(tmpDir, assetName)
	if tarPath == filepath.Join(tmpDir, "") {
		tarPath = filepath.Join(tmpDir, "update.tar.gz")
	}
	if err := downloadFile(assetURL, tarPath); err != nil {
		return "", err
	}
	extractDir := filepath.Join(tmpDir, "extract")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return "", err
	}
	if err := extractTarGz(tarPath, extractDir); err != nil {
		return "", fmt.Errorf("解压失败: %w", err)
	}
	srcRoot, err := findPackageRoot(extractDir)
	if err != nil {
		return "", err
	}

	// stop cores before replacing
	_ = exec.Command("pkill", "-x", "xray").Run()
	_ = exec.Command("pkill", "-x", "sing-box").Run()
	if runtime.GOOS == "windows" {
		_ = exec.Command("taskkill", "/IM", "xray.exe", "/F").Run()
		_ = exec.Command("taskkill", "/IM", "sing-box.exe", "/F").Run()
	}

	// replace cores
	binDir := filepath.Join(installDir, "bin")
	_ = os.MkdirAll(binDir, 0o755)
	for _, name := range []string{"xray", "sing-box", "geoip.dat", "geosite.dat"} {
		src := filepath.Join(srcRoot, "bin", name)
		if runtime.GOOS == "windows" && (name == "xray" || name == "sing-box") {
			// package is linux-only currently
			continue
		}
		if st, err := os.Stat(src); err == nil && !st.IsDir() {
			dst := filepath.Join(binDir, name)
			if err := copyFile(src, dst); err != nil {
				return "", fmt.Errorf("更新 %s 失败: %w", name, err)
			}
			if name == "xray" || name == "sing-box" {
				_ = os.Chmod(dst, 0o755)
			}
		}
	}

	// replace panel binary
	panelName := "3xui-lite"
	if runtime.GOOS == "windows" {
		panelName = "3xui-lite.exe"
	}
	srcPanel := filepath.Join(srcRoot, "3xui-lite")
	if _, err := os.Stat(srcPanel); err != nil {
		return "", fmt.Errorf("安装包中未找到面板二进制 3xui-lite")
	}
	dstPanel := filepath.Join(installDir, panelName)
	// On Linux, write to .new then swap via helper script after restart
	newPath := dstPanel + ".new"
	if err := copyFile(srcPanel, newPath); err != nil {
		return "", fmt.Errorf("写入新面板失败: %w", err)
	}
	_ = os.Chmod(newPath, 0o755)

	// also update install scripts if present
	for _, sh := range []string{"install.sh", "start.sh", "stop.sh", "uninstall.sh"} {
		src := filepath.Join(srcRoot, sh)
		if st, err := os.Stat(src); err == nil && !st.IsDir() {
			_ = copyFile(src, filepath.Join(installDir, sh))
			_ = os.Chmod(filepath.Join(installDir, sh), 0o755)
		}
	}

	msg := "文件已下载并准备就绪，即将重启服务完成替换"
	if err := scheduleRestart(installDir, dstPanel, newPath); err != nil {
		// fallback: try rename now (may fail if binary locked on Windows)
		_ = os.Rename(newPath, dstPanel)
		return msg + "（自动重启失败: " + err.Error() + "，请手动 systemctl restart 3xui-lite）", nil
	}
	return msg, nil
}

func scheduleRestart(installDir, dstPanel, newPath string) error {
	if runtime.GOOS == "windows" {
		// best-effort: rename after short delay won't work well on windows for locked exe
		// leave .new and ask user — still try start a bat
		bat := filepath.Join(installDir, "apply-update.bat")
		content := fmt.Sprintf("@echo off\r\ntimeout /t 2 /nobreak >nul\r\nmove /Y \"%s\" \"%s\"\r\n", newPath, dstPanel)
		_ = os.WriteFile(bat, []byte(content), 0o755)
		cmd := exec.Command("cmd", "/C", "start", "", bat)
		return cmd.Start()
	}

	script := filepath.Join(installDir, "apply-update.sh")
	body := fmt.Sprintf(`#!/bin/bash
set -e
sleep 2
if [ -f "%s" ]; then
  mv -f "%s" "%s"
  chmod +x "%s"
fi
if command -v systemctl >/dev/null 2>&1; then
  systemctl restart 3xui-lite 2>/dev/null || systemctl restart 3xui-lite.service 2>/dev/null || true
fi
# fallback if service not active
if ! systemctl is-active --quiet 3xui-lite 2>/dev/null; then
  pkill -f "%s/3xui-lite" 2>/dev/null || true
  sleep 1
  cd "%s"
  export XUI_LISTEN="${XUI_LISTEN:-0.0.0.0:18080}"
  export XUI_DATA="${XUI_DATA:-%s/data}"
  export XRAY_BIN="${XRAY_BIN:-%s/bin/xray}"
  export SINGBOX_BIN="${SINGBOX_BIN:-%s/bin/sing-box}"
  nohup ./3xui-lite >> data/panel.log 2>&1 &
fi
rm -f "%s"
`, newPath, newPath, dstPanel, dstPanel, installDir, installDir, installDir, installDir, installDir, script)
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		return err
	}
	cmd := exec.Command("/bin/bash", script)
	cmd.Dir = installDir
	// detach
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func downloadFile(url, dest string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("下载失败 HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func extractTarGz(tarPath, dest string) error {
	f, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// zip-slip guard
		target := filepath.Join(dest, hdr.Name)
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(dest)+string(os.PathSeparator)) &&
			filepath.Clean(target) != filepath.Clean(dest) {
			return fmt.Errorf("非法路径: %s", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode)|0o644)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
			// ensure exec for known bins
			base := filepath.Base(target)
			if base == "3xui-lite" || base == "xray" || base == "sing-box" || strings.HasSuffix(base, ".sh") {
				_ = os.Chmod(target, 0o755)
			}
		}
	}
	return nil
}

func findPackageRoot(extractDir string) (string, error) {
	// prefer */3xui-lite file
	var found string
	_ = filepath.Walk(extractDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if info.Name() == "3xui-lite" {
			found = filepath.Dir(path)
			return io.EOF
		}
		return nil
	})
	if found != "" {
		return found, nil
	}
	// maybe extractDir itself
	if _, err := os.Stat(filepath.Join(extractDir, "3xui-lite")); err == nil {
		return extractDir, nil
	}
	return "", fmt.Errorf("解压后未找到 3xui-lite")
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	// write to temp then rename
	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	cerr := out.Close()
	if err != nil {
		return err
	}
	if cerr != nil {
		return cerr
	}
	return os.Rename(tmp, dst)
}

// versionLess reports whether a < b (simple dotted numeric compare).
func versionLess(a, b string) bool {
	a = strings.TrimPrefix(strings.TrimSpace(a), "v")
	b = strings.TrimPrefix(strings.TrimSpace(b), "v")
	// strip suffix like -lite
	if i := strings.Index(a, "-"); i >= 0 {
		a = a[:i]
	}
	if i := strings.Index(b, "-"); i >= 0 {
		b = b[:i]
	}
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		var ai, bi int
		if i < len(as) {
			fmt.Sscanf(as[i], "%d", &ai)
		}
		if i < len(bs) {
			fmt.Sscanf(bs[i], "%d", &bi)
		}
		if ai < bi {
			return true
		}
		if ai > bi {
			return false
		}
	}
	return false
}
