package xray

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Manager controls the xray-core process and config file.
type Manager struct {
	mu         sync.Mutex
	bin        string
	configPath string
	cmd        *exec.Cmd
	startedAt  time.Time
	lastErr    string
	logLines   []string
	maxLog     int
}

func NewManager(bin, configPath string) *Manager {
	return &Manager{
		bin:        bin,
		configPath: configPath,
		maxLog:     200,
		logLines:   make([]string, 0, 200),
	}
}

func (m *Manager) Bin() string { return m.bin }

func (m *Manager) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cmd != nil && m.cmd.Process != nil && m.cmd.ProcessState == nil
}

func (m *Manager) Uptime() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cmd == nil || m.cmd.Process == nil || m.cmd.ProcessState != nil {
		return 0
	}
	return int64(time.Since(m.startedAt).Seconds())
}

func (m *Manager) LastError() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastErr
}

func (m *Manager) Logs() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.logLines))
	copy(out, m.logLines)
	return out
}

func (m *Manager) Version() string {
	cmd := exec.Command(m.bin, "version")
	if dir := filepath.Dir(m.bin); dir != "" && dir != "." {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "unknown"
	}
	lines := strings.Split(string(out), "\n")
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0])
	}
	return strings.TrimSpace(string(out))
}

func (m *Manager) WriteConfig(data []byte) error {
	if err := os.MkdirAll(filepath.Dir(m.configPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(m.configPath, data, 0o644)
}

func (m *Manager) Restart() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.stopLocked(); err != nil {
		// ignore stop errors if not running
		_ = err
	}
	return m.startLocked()
}

func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cmd != nil && m.cmd.Process != nil && m.cmd.ProcessState == nil {
		return nil
	}
	return m.startLocked()
}

func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopLocked()
}

func (m *Manager) startLocked() error {
	if _, err := os.Stat(m.configPath); err != nil {
		return fmt.Errorf("config not found: %w", err)
	}
	// Validate first
	if err := m.testConfigLocked(); err != nil {
		m.lastErr = err.Error()
		return err
	}

	cmd := exec.Command(m.bin, "run", "-c", m.configPath)
	// Run from xray binary dir so geoip.dat / geosite.dat / wintun.dll resolve.
	if dir := filepath.Dir(m.bin); dir != "" && dir != "." {
		cmd.Dir = dir
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		m.lastErr = err.Error()
		return err
	}
	m.cmd = cmd
	m.startedAt = time.Now()
	m.lastErr = ""

	go m.consume(stdout)
	go m.consume(stderr)
	go func() {
		_ = cmd.Wait()
		m.mu.Lock()
		if m.cmd == cmd {
			m.cmd = nil
		}
		m.mu.Unlock()
	}()
	return nil
}

func (m *Manager) stopLocked() error {
	if m.cmd == nil || m.cmd.Process == nil {
		m.cmd = nil
		return nil
	}
	var err error
	if runtime.GOOS == "windows" {
		err = m.cmd.Process.Kill()
	} else {
		err = m.cmd.Process.Signal(os.Interrupt)
		done := make(chan struct{})
		go func() {
			_, _ = m.cmd.Process.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			err = m.cmd.Process.Kill()
		}
	}
	m.cmd = nil
	return err
}

func (m *Manager) testConfigLocked() error {
	cmd := exec.Command(m.bin, "run", "-test", "-c", m.configPath)
	if dir := filepath.Dir(m.bin); dir != "" && dir != "." {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("xray config test failed: %s (%v)", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (m *Manager) TestConfig() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.testConfigLocked()
}

func (m *Manager) consume(r interface{ Read([]byte) (int, error) }) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		m.mu.Lock()
		m.logLines = append(m.logLines, line)
		if len(m.logLines) > m.maxLog {
			m.logLines = m.logLines[len(m.logLines)-m.maxLog:]
		}
		m.mu.Unlock()
	}
}
