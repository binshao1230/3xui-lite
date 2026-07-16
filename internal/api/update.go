package api

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/bin/3xui-lite/internal/update"
	"github.com/bin/3xui-lite/internal/version"
)

func (s *Server) checkUpdate(w http.ResponseWriter, r *http.Request) {
	info, err := update.CheckLatest(version.Repo)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{
		"ok":   true,
		"data": info,
		"os":   runtime.GOOS,
		"arch": runtime.GOARCH,
	})
}

func (s *Server) applyUpdate(w http.ResponseWriter, r *http.Request) {
	info, err := update.CheckLatest(version.Repo)
	if err != nil {
		writeErr(w, 500, "检查更新失败: "+err.Error())
		return
	}
	if !info.HasUpdate {
		writeJSON(w, 200, map[string]any{
			"ok":  true,
			"msg": "已是最新版本 " + info.Current,
			"data": info,
		})
		return
	}
	if !info.CanUpdate || info.AssetURL == "" {
		writeErr(w, 400, info.Reason)
		return
	}

	// stop cores first
	s.Cores.StopAll()

	installDir := installDir()
	msg, err := update.Apply(installDir, info.AssetURL, info.AssetName)
	if err != nil {
		// try restart old cores
		_ = s.applyCore()
		writeErr(w, 500, "更新失败: "+err.Error())
		return
	}

	writeJSON(w, 200, map[string]any{
		"ok":      true,
		"msg":     msg,
		"data":    info,
		"restart": true,
	})

	// give response time to flush, then exit so systemd/helper can swap binary
	go func() {
		time.Sleep(1500 * time.Millisecond)
		// best-effort stop cores again
		s.Cores.StopAll()
		os.Exit(0)
	}()
}

func installDir() string {
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
