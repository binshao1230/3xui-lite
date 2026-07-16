package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
)

// App holds runtime configuration for the panel.
type App struct {
	Listen        string
	DataDir       string
	DBPath        string
	XrayBin       string
	XrayConfig    string
	SingboxBin    string
	SingboxConfig string
	Secret        string
}

func Default() *App {
	base := appBaseDir()
	dataDir := env("XUI_DATA", filepath.Join(base, "data"))
	return &App{
		Listen:        env("XUI_LISTEN", "127.0.0.1:18080"),
		DataDir:       dataDir,
		DBPath:        filepath.Join(dataDir, "xui.db"),
		XrayBin:       resolveBin(base, "XRAY_BIN", "xray", "xray.exe"),
		XrayConfig:    filepath.Join(dataDir, "config.json"),
		SingboxBin:    resolveBin(base, "SINGBOX_BIN", "sing-box", "sing-box.exe"),
		SingboxConfig: filepath.Join(dataDir, "singbox.json"),
		Secret:        env("XUI_SECRET", "change-me-please"),
	}
}

func appBaseDir() string {
	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		return filepath.Dir(exe)
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}

func resolveBin(base, envKey, unixName, winName string) string {
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	name := unixName
	if runtime.GOOS == "windows" {
		name = winName
	}
	candidates := []string{
		filepath.Join(base, "bin", name),
		filepath.Join(base, name),
		filepath.Join(".", "bin", name),
		name,
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			abs, err := filepath.Abs(c)
			if err == nil {
				return abs
			}
			return c
		}
	}
	// Prefer bin path even if missing (so UI shows expected location)
	return filepath.Join(base, "bin", name)
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func EnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
