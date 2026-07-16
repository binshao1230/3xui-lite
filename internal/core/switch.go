package core

import (
	"fmt"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/bin/3xui-lite/internal/models"
	"github.com/bin/3xui-lite/internal/proc"
	"github.com/bin/3xui-lite/internal/singbox"
	"github.com/bin/3xui-lite/internal/xray"
)

const (
	CoreXray    = "xray"
	CoreSingbox = "singbox"
)

// Switch manages dual cores and which one is active.
type Switch struct {
	mu      sync.Mutex
	active  string
	Xray    *proc.Manager
	Singbox *proc.Manager
}

func NewSwitch(xrayMgr, sbMgr *proc.Manager, active string) *Switch {
	if active != CoreSingbox {
		active = CoreXray
	}
	return &Switch{active: active, Xray: xrayMgr, Singbox: sbMgr}
}

func (s *Switch) Active() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active
}

func (s *Switch) SetActive(name string) error {
	if name != CoreXray && name != CoreSingbox {
		return fmt.Errorf("unsupported core: %s", name)
	}
	s.mu.Lock()
	s.active = name
	s.mu.Unlock()
	return nil
}

func (s *Switch) ActiveManager() *proc.Manager {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active == CoreSingbox {
		return s.Singbox
	}
	return s.Xray
}

func (s *Switch) Manager(name string) *proc.Manager {
	if name == CoreSingbox {
		return s.Singbox
	}
	return s.Xray
}

// Apply builds config for active core, stops the other, restarts active.
func (s *Switch) Apply(inbounds []models.Inbound) error {
	s.mu.Lock()
	active := s.active
	s.mu.Unlock()

	var (
		raw []byte
		err error
		mgr *proc.Manager
		other *proc.Manager
	)

	switch active {
	case CoreSingbox:
		mgr = s.Singbox
		other = s.Xray
		cfg, e := singbox.BuildConfig(inbounds)
		if e != nil {
			return e
		}
		raw, err = singbox.MarshalConfig(cfg)
	default:
		mgr = s.Xray
		other = s.Singbox
		cfg, e := xray.BuildConfig(inbounds)
		if e != nil {
			return e
		}
		raw, err = xray.MarshalConfig(cfg)
	}
	if err != nil {
		return err
	}
	if err := mgr.WriteConfig(raw); err != nil {
		return err
	}
	// Stop inactive core so ports are free
	_ = other.Stop()
	_ = mgr.Stop()
	// Kill orphaned processes (previous panel sessions may leave cores running)
	killOrphans()
	time.Sleep(200 * time.Millisecond)
	return mgr.Restart()
}

func killOrphans() {
	if runtime.GOOS == "windows" {
		_ = exec.Command("taskkill", "/IM", "xray.exe", "/F").Run()
		_ = exec.Command("taskkill", "/IM", "sing-box.exe", "/F").Run()
		return
	}
	_ = exec.Command("pkill", "-x", "xray").Run()
	_ = exec.Command("pkill", "-x", "sing-box").Run()
}

// Preview returns config JSON for a given core (or active if empty).
func (s *Switch) Preview(inbounds []models.Inbound, name string) ([]byte, error) {
	if name == "" {
		name = s.Active()
	}
	switch name {
	case CoreSingbox:
		cfg, err := singbox.BuildConfig(inbounds)
		if err != nil {
			return nil, err
		}
		return singbox.MarshalConfig(cfg)
	default:
		cfg, err := xray.BuildConfig(inbounds)
		if err != nil {
			return nil, err
		}
		return xray.MarshalConfig(cfg)
	}
}

func (s *Switch) StopAll() {
	_ = s.Xray.Stop()
	_ = s.Singbox.Stop()
}

func (s *Switch) Status() map[string]any {
	active := s.Active()
	am := s.ActiveManager()
	return map[string]any{
		"activeCore":     active,
		"coreRunning":    am.IsRunning(),
		"coreVersion":    am.Version(),
		"uptime":         am.Uptime(),
		"xrayRunning":    s.Xray.IsRunning(),
		"xrayVersion":    s.Xray.Version(),
		"xrayAvailable":  s.Xray.Available(),
		"singboxRunning": s.Singbox.IsRunning(),
		"singboxVersion": s.Singbox.Version(),
		"singboxAvailable": s.Singbox.Available(),
		// backward-compat fields used by old UI
		"xrayRunningCompat": am.IsRunning(),
	}
}
