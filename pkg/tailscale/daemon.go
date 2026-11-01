package tailscale

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

const (
	defaultSocket   = "/var/run/tailscale/tailscaled.sock"
	defaultStateDir = "/data/tailscale"
)

// daemonState holds the supervised tailscaled process.
type daemonState struct {
	mu     sync.Mutex
	proc   *exec.Cmd
	exited chan struct{} // closed when proc exits
	paused bool          // supervisor must not (re)start tailscaled while true
	owned  bool          // true when this process supervises tailscaled
}

func socketPath() string {
	if v := os.Getenv("TS_SOCKET"); v != "" {
		return v
	}
	return defaultSocket
}

func stateDir() string {
	if v := os.Getenv("TS_STATE_DIR"); v != "" {
		return v
	}
	return defaultStateDir
}

// socketAlive reports whether something is already listening on the tailscaled socket.
func socketAlive() bool {
	conn, err := net.DialTimeout("unix", socketPath(), 2*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// OwnsDaemon reports whether this process supervises tailscaled
// (true in the Docker image, false when an external service manager does).
func (m *Manager) OwnsDaemon() bool {
	m.daemon.mu.Lock()
	defer m.daemon.mu.Unlock()
	return m.daemon.owned
}

// RunDaemon starts tailscaled and keeps it running until ctx is cancelled.
// If a tailscaled is already reachable on the socket (systemd, host service,
// ...) it is left alone and RunDaemon returns immediately.
// It blocks, so run it in a goroutine and wait for it on shutdown.
func (m *Manager) RunDaemon(ctx context.Context) {
	if _, err := exec.LookPath("tailscaled"); err != nil {
		log.Println("tailscale: tailscaled not installed, daemon supervision disabled")
		return
	}
	if socketAlive() {
		log.Println("tailscale: external tailscaled detected, not supervising it")
		return
	}

	st := stateDir()
	if err := os.MkdirAll(st, 0o700); err != nil {
		log.Printf("tailscale: cannot create state dir %s: %v", st, err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(socketPath()), 0o755); err != nil {
		log.Printf("tailscale: cannot create socket dir: %v", err)
		return
	}

	m.daemon.mu.Lock()
	m.daemon.owned = true
	m.daemon.mu.Unlock()

	backoff := time.Second
	for ctx.Err() == nil {
		m.daemon.mu.Lock()
		if m.daemon.paused {
			m.daemon.mu.Unlock()
			sleepCtx(ctx, 300*time.Millisecond)
			continue
		}

		// Resolve the binary on every start so an upgraded tailscaled is picked up.
		bin, err := exec.LookPath("tailscaled")
		if err != nil {
			m.daemon.mu.Unlock()
			log.Printf("tailscale: tailscaled binary not found: %v", err)
			sleepCtx(ctx, 5*time.Second)
			continue
		}
		cmd := exec.Command(bin,
			"--state="+filepath.Join(st, "tailscaled.state"),
			"--socket="+socketPath(),
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := cmd.Start(); err != nil {
			m.daemon.mu.Unlock()
			log.Printf("tailscale: failed to start tailscaled: %v", err)
			sleepCtx(ctx, backoff)
			backoff = nextBackoff(backoff)
			continue
		}
		exited := make(chan struct{})
		m.daemon.proc = cmd
		m.daemon.exited = exited
		m.daemon.mu.Unlock()

		started := time.Now()
		log.Printf("tailscale: tailscaled started (pid %d)", cmd.Process.Pid)

		waitCh := make(chan error, 1)
		go func() { waitCh <- cmd.Wait() }()

		select {
		case werr := <-waitCh:
			m.daemon.mu.Lock()
			m.daemon.proc = nil
			close(exited)
			wasPaused := m.daemon.paused
			m.daemon.mu.Unlock()
			if ctx.Err() != nil {
				return
			}
			if wasPaused {
				continue // intentional stop (update); restart when resumed
			}
			log.Printf("tailscale: tailscaled exited unexpectedly: %v", werr)
			if time.Since(started) > 30*time.Second {
				backoff = time.Second
			}
			sleepCtx(ctx, backoff)
			backoff = nextBackoff(backoff)
		case <-ctx.Done():
			terminate(cmd, waitCh)
			m.daemon.mu.Lock()
			m.daemon.proc = nil
			close(exited)
			m.daemon.mu.Unlock()
			return
		}
	}
}

// pauseDaemon stops tailscaled and prevents the supervisor from restarting it
// until resumeDaemon is called. No-op when the daemon is not owned.
func (m *Manager) pauseDaemon() error {
	m.daemon.mu.Lock()
	if !m.daemon.owned {
		m.daemon.mu.Unlock()
		return fmt.Errorf("tailscaled is not supervised by this process")
	}
	m.daemon.paused = true
	proc := m.daemon.proc
	exited := m.daemon.exited
	m.daemon.mu.Unlock()

	if proc == nil || proc.Process == nil {
		return nil
	}
	_ = syscall.Kill(-proc.Process.Pid, syscall.SIGTERM)
	select {
	case <-exited:
		return nil
	case <-time.After(20 * time.Second):
		_ = syscall.Kill(-proc.Process.Pid, syscall.SIGKILL)
		select {
		case <-exited:
			return nil
		case <-time.After(5 * time.Second):
			return fmt.Errorf("tailscaled did not stop")
		}
	}
}

func (m *Manager) resumeDaemon() {
	m.daemon.mu.Lock()
	m.daemon.paused = false
	m.daemon.mu.Unlock()
}

// restartDaemon restarts tailscaled so a newly installed binary is used.
func (m *Manager) restartDaemon() error {
	if m.OwnsDaemon() {
		err := m.pauseDaemon()
		m.resumeDaemon()
		if err != nil {
			return err
		}
		return nil
	}
	if _, err := exec.LookPath("systemctl"); err == nil {
		if out, err := exec.Command("systemctl", "restart", "tailscaled").CombinedOutput(); err != nil {
			return fmt.Errorf("systemctl restart tailscaled: %s", string(out))
		}
		return nil
	}
	return fmt.Errorf("cannot restart external tailscaled; restart the service or container manually")
}

// waitDaemonReady waits until the tailscaled socket answers.
func waitDaemonReady(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if socketAlive() {
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

func terminate(cmd *exec.Cmd, waitCh <-chan error) {
	if cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	select {
	case <-waitCh:
	case <-time.After(15 * time.Second):
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-waitCh
	}
}

func nextBackoff(d time.Duration) time.Duration {
	d *= 2
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d
}

func sleepCtx(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
