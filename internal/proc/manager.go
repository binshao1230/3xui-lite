package proc

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

// Manager controls an external core process (xray / sing-box).
type Manager struct {
	mu         sync.Mutex
	name       string
	bin        string
	configPath string
	runArgs    []string // template: use "{c}" for config path
	testArgs   []string
	verArgs    []string
	cmd        *exec.Cmd
	startedAt  time.Time
	lastErr    string
	logLines   []string
	maxLog     int
}

type Options struct {
	Name       string
	Bin        string
	ConfigPath string
	RunArgs    []string // e.g. []string{"run", "-c", "{c}"}
	TestArgs   []string // e.g. []string{"run", "-test", "-c", "{c}"} for xray; check -c for sing-box
	VersionArgs []string
}

func New(opts Options) *Manager {
	if len(opts.RunArgs) == 0 {
		opts.RunArgs = []string{"run", "-c", "{c}"}
	}
	if len(opts.TestArgs) == 0 {
		opts.TestArgs = []string{"run", "-test", "-c", "{c}"}
	}
	if len(opts.VersionArgs) == 0 {
		opts.VersionArgs = []string{"version"}
	}
	if opts.Name == "" {
		opts.Name = "core"
	}
	return &Manager{
		name:       opts.Name,
		bin:        opts.Bin,
		configPath: opts.ConfigPath,
		runArgs:    opts.RunArgs,
		testArgs:   opts.TestArgs,
		verArgs:    opts.VersionArgs,
		maxLog:     200,
		logLines:   make([]string, 0, 200),
	}
}

func (m *Manager) Name() string   { return m.name }
func (m *Manager) Bin() string    { return m.bin }
func (m *Manager) Config() string { return m.configPath }

func (m *Manager) Available() bool {
	if m.bin == "" {
		return false
	}
	st, err := os.Stat(m.bin)
	return err == nil && !st.IsDir()
}

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

func (m *Manager) expand(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = strings.ReplaceAll(a, "{c}", m.configPath)
	}
	return out
}

func (m *Manager) Version() string {
	if !m.Available() {
		return "not installed"
	}
	cmd := exec.Command(m.bin, m.verArgs...)
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
	_ = m.stopLocked()
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
	if !m.Available() {
		err := fmt.Errorf("%s binary not found: %s", m.name, m.bin)
		m.lastErr = err.Error()
		return err
	}
	if _, err := os.Stat(m.configPath); err != nil {
		return fmt.Errorf("config not found: %w", err)
	}
	if err := m.testConfigLocked(); err != nil {
		m.lastErr = err.Error()
		return err
	}

	cmd := exec.Command(m.bin, m.expand(m.runArgs)...)
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

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	// Detect immediate crash (bad config runtime / port in use)
	select {
	case err := <-waitCh:
		m.cmd = nil
		msg := fmt.Sprintf("%s exited immediately", m.name)
		if err != nil {
			msg = fmt.Sprintf("%s exited immediately: %v", m.name, err)
		}
		// drain a moment for log pipes
		time.Sleep(150 * time.Millisecond)
		logs := m.Logs()
		if len(logs) > 0 {
			from := len(logs) - 8
			if from < 0 {
				from = 0
			}
			msg += " | " + strings.Join(logs[from:], " ; ")
		}
		m.lastErr = msg
		return fmt.Errorf("%s", msg)
	case <-time.After(400 * time.Millisecond):
		// still running — keep waiting in background
		go func() {
			err := <-waitCh
			m.mu.Lock()
			if m.cmd == cmd {
				m.cmd = nil
				if err != nil && m.lastErr == "" {
					m.lastErr = fmt.Sprintf("%s exited: %v", m.name, err)
				}
			}
			m.mu.Unlock()
		}()
	}
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
	cmd := exec.Command(m.bin, m.expand(m.testArgs)...)
	if dir := filepath.Dir(m.bin); dir != "" && dir != "." {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s config test failed: %s (%v)", m.name, strings.TrimSpace(string(out)), err)
	}
	return nil
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
