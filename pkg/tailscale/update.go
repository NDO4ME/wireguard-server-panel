package tailscale

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ndo4me/wireguard-server-panel/pkg/aptutil"
)

// Settings keys used by the updater (stored through Store).
const (
	SettingAutoUpdate = "tailscale_auto_update"
	settingLastResult = "tailscale_update_last"
	settingSkip       = "tailscale_update_skip_version"
)

const (
	tailscaleSourceList = "/etc/apt/sources.list.d/tailscale.list"
	latestVersionURL    = "https://pkgs.tailscale.com/stable/?mode=json"
)

// Store persists small key/value settings (implemented by the panel database).
type Store interface {
	GetSetting(key string) (string, error)
	SetSetting(key, value string) error
}

// UpdateResult describes the outcome of the last update attempt.
type UpdateResult struct {
	Time       time.Time `json:"time"`
	From       string    `json:"from"`
	To         string    `json:"to"`
	Success    bool      `json:"success"`
	RolledBack bool      `json:"rolled_back"`
	Message    string    `json:"message"`
	Auto       bool      `json:"auto"`
}

// UpdateStatus is returned to the admin panel.
type UpdateStatus struct {
	Installed       bool          `json:"installed"`
	Supported       bool          `json:"supported"`
	Unsupported     string        `json:"unsupported_reason,omitempty"`
	Current         string        `json:"current_version"`
	Latest          string        `json:"latest_version"`
	UpdateAvailable bool          `json:"update_available"`
	AutoUpdate      bool          `json:"auto_update"`
	Checking        bool          `json:"checking"`
	Updating        bool          `json:"updating"`
	CheckedAt       *time.Time    `json:"checked_at,omitempty"`
	CheckError      string        `json:"check_error,omitempty"`
	SkippedVersion  string        `json:"skipped_version,omitempty"`
	LastResult      *UpdateResult `json:"last_result,omitempty"`
}

type updaterState struct {
	mu         sync.Mutex
	store      Store
	busy       bool // a check or an update is running
	checking   bool
	updating   bool
	latest     string
	candidate  string // exact apt version string of the latest package
	checkedAt  time.Time
	checkError string
	afterApply func()
}

// ConfigureUpdater wires the settings store and an optional callback that is
// invoked after a successful update (e.g. to re-apply routing rules).
func (m *Manager) ConfigureUpdater(store Store, afterUpdate func()) {
	m.updater.mu.Lock()
	m.updater.store = store
	m.updater.afterApply = afterUpdate
	m.updater.mu.Unlock()
}

// ---------------------------------------------------------------------------
// Version helpers
// ---------------------------------------------------------------------------

var semverRe = regexp.MustCompile(`\d+\.\d+\.\d+`)

// normalizeVersion extracts "major.minor.patch" from strings such as
// "1.80.3", "1.80.3-t2a3b4c5d6-g7e8f9a0b1" or "1:1.80.3-1".
func normalizeVersion(s string) string {
	return semverRe.FindString(s)
}

// compareVersions returns 1 if a > b, -1 if a < b and 0 if equal/unparsable.
func compareVersions(a, b string) int {
	pa, pb := parseTriple(a), parseTriple(b)
	for i := 0; i < 3; i++ {
		if pa[i] > pb[i] {
			return 1
		}
		if pa[i] < pb[i] {
			return -1
		}
	}
	return 0
}

func parseTriple(s string) [3]int {
	var out [3]int
	parts := strings.Split(normalizeVersion(s), ".")
	for i := 0; i < 3 && i < len(parts); i++ {
		out[i], _ = strconv.Atoi(parts[i])
	}
	return out
}

// CurrentVersion returns the installed client version (e.g. "1.104.4").
func (m *Manager) CurrentVersion() (string, error) {
	if !m.IsInstalled() {
		return "", errors.New("tailscale is not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "tailscale", "version").Output()
	if err != nil {
		return "", fmt.Errorf("tailscale version: %w", err)
	}
	line := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
	v := normalizeVersion(line)
	if v == "" {
		return "", fmt.Errorf("cannot parse tailscale version from %q", line)
	}
	return v, nil
}

// daemonVersion returns the version reported by the running tailscaled.
func daemonVersion() string {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "tailscale", "status", "--json", "--peers=false").Output()
	if err != nil {
		// "status" exits non-zero in some states but still prints JSON.
		if len(out) == 0 {
			return ""
		}
	}
	var s struct {
		Version string `json:"Version"`
	}
	if json.Unmarshal(out, &s) != nil {
		return ""
	}
	return normalizeVersion(s.Version)
}

// ---------------------------------------------------------------------------
// Capability checks
// ---------------------------------------------------------------------------

// updateSupport tells whether this deployment can update Tailscale in place.
func (m *Manager) updateSupport() (bool, string) {
	if !m.IsInstalled() {
		return false, "Tailscale is not installed"
	}
	if os.Geteuid() != 0 {
		return false, "panel is not running as root"
	}
	if _, err := exec.LookPath("apt-get"); err != nil {
		return false, "apt-get is not available (non-Debian/Ubuntu system)"
	}
	if _, err := os.Stat(tailscaleSourceList); err != nil {
		return false, "Tailscale apt repository is not configured"
	}
	if !m.OwnsDaemon() {
		if _, err := exec.LookPath("systemctl"); err != nil {
			return false, "tailscaled is managed externally and cannot be restarted automatically"
		}
	}
	return true, ""
}

// ---------------------------------------------------------------------------
// Latest version lookup
// ---------------------------------------------------------------------------

// aptEnv returns a non-interactive environment for apt-get.
func aptEnv() []string {
	return append(os.Environ(), "DEBIAN_FRONTEND=noninteractive", "LC_ALL=C")
}

// latestFromApt refreshes only the Tailscale repository and returns the
// install candidate (exact apt version string).
func latestFromApt(ctx context.Context) (string, error) {
	upd := exec.CommandContext(ctx, "apt-get", "update",
		"-o", "Dir::Etc::sourcelist="+tailscaleSourceList,
		"-o", "Dir::Etc::sourceparts=-",
		"-o", "APT::Get::List-Cleanup=0",
		"-o", "Acquire::Retries=2",
		"-o", "Acquire::http::Timeout=20",
	)
	upd.Env = aptEnv()
	if out, err := upd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("apt-get update failed: %s", lastLines(string(out), 3))
	}

	pol := exec.CommandContext(ctx, "apt-cache", "policy", "tailscale")
	pol.Env = aptEnv()
	out, err := pol.Output()
	if err != nil {
		return "", fmt.Errorf("apt-cache policy failed: %w", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Candidate:") {
			cand := strings.TrimSpace(strings.TrimPrefix(line, "Candidate:"))
			if cand == "" || cand == "(none)" {
				break
			}
			return cand, nil
		}
	}
	return "", errors.New("no tailscale candidate found in apt")
}

// latestFromWeb asks the official stable-track endpoint (fallback / sanity check).
func latestFromWeb(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestVersionURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("pkgs.tailscale.com returned %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	var r struct {
		TarballsVersion string `json:"TarballsVersion"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return "", err
	}
	v := normalizeVersion(r.TarballsVersion)
	if v == "" {
		return "", errors.New("latest version missing in response")
	}
	return v, nil
}

// ---------------------------------------------------------------------------
// Public status / check / update API
// ---------------------------------------------------------------------------

func (m *Manager) setting(key string) string {
	m.updater.mu.Lock()
	st := m.updater.store
	m.updater.mu.Unlock()
	if st == nil {
		return ""
	}
	v, _ := st.GetSetting(key)
	return v
}

func (m *Manager) putSetting(key, value string) {
	m.updater.mu.Lock()
	st := m.updater.store
	m.updater.mu.Unlock()
	if st != nil {
		if err := st.SetSetting(key, value); err != nil {
			log.Printf("tailscale: cannot persist %s: %v", key, err)
		}
	}
}

// AutoUpdateEnabled returns the persisted auto-update switch.
// When never set, the TAILSCALE_AUTO_UPDATE env var decides (default: on).
func (m *Manager) AutoUpdateEnabled() bool {
	switch m.setting(SettingAutoUpdate) {
	case "true":
		return true
	case "false":
		return false
	}
	return strings.ToLower(os.Getenv("TAILSCALE_AUTO_UPDATE")) != "false"
}

// SetAutoUpdate persists the auto-update switch.
func (m *Manager) SetAutoUpdate(enabled bool) {
	m.putSetting(SettingAutoUpdate, strconv.FormatBool(enabled))
}

// UpdateStatus returns the current update state without any network access.
func (m *Manager) UpdateStatus() UpdateStatus {
	supported, reason := m.updateSupport()
	st := UpdateStatus{
		Installed:   m.IsInstalled(),
		Supported:   supported,
		Unsupported: reason,
		AutoUpdate:  m.AutoUpdateEnabled(),
	}
	if v, err := m.CurrentVersion(); err == nil {
		st.Current = v
	}

	m.updater.mu.Lock()
	st.Checking = m.updater.checking
	st.Updating = m.updater.updating
	st.Latest = m.updater.latest
	st.CheckError = m.updater.checkError
	if !m.updater.checkedAt.IsZero() {
		t := m.updater.checkedAt
		st.CheckedAt = &t
	}
	m.updater.mu.Unlock()

	st.UpdateAvailable = st.Current != "" && st.Latest != "" && compareVersions(st.Latest, st.Current) > 0
	st.SkippedVersion = m.setting(settingSkip)
	if raw := m.setting(settingLastResult); raw != "" {
		var r UpdateResult
		if json.Unmarshal([]byte(raw), &r) == nil {
			st.LastResult = &r
		}
	}
	return st
}

// CheckForUpdate looks up the latest stable version. Only one check/update
// may run at a time; a concurrent call returns an error.
func (m *Manager) CheckForUpdate(ctx context.Context) error {
	if !m.beginBusy(false) {
		return errors.New("another check or update is already running")
	}
	defer m.endBusy()
	return m.check(ctx)
}

func (m *Manager) beginBusy(updating bool) bool {
	m.updater.mu.Lock()
	defer m.updater.mu.Unlock()
	if m.updater.busy {
		return false
	}
	m.updater.busy = true
	m.updater.checking = !updating
	m.updater.updating = updating
	m.updater.mu.Unlock()
	aptutil.Acquire() // serialise apt use with the WireGuard updater
	m.updater.mu.Lock()
	return true
}

func (m *Manager) endBusy() {
	aptutil.Release() // frees apt lists and cached packages
	m.updater.mu.Lock()
	m.updater.busy = false
	m.updater.checking = false
	m.updater.updating = false
	m.updater.mu.Unlock()
}

// check performs the lookup; the caller holds the busy flag.
func (m *Manager) check(parent context.Context) error {
	ctx, cancel := context.WithTimeout(parent, 3*time.Minute)
	defer cancel()

	var (
		candidate string
		latest    string
		err       error
	)
	if ok, _ := m.updateSupport(); ok {
		candidate, err = latestFromApt(ctx)
		if err == nil {
			latest = normalizeVersion(candidate)
		}
	}
	if latest == "" {
		// Not apt-capable (or apt failed): still report the latest version.
		var werr error
		latest, werr = latestFromWeb(ctx)
		if werr != nil {
			if err == nil {
				err = werr
			} else {
				err = fmt.Errorf("%v; web lookup: %v", err, werr)
			}
		}
	}

	m.updater.mu.Lock()
	m.updater.checkedAt = time.Now()
	if latest != "" {
		m.updater.latest = latest
		m.updater.candidate = candidate
		m.updater.checkError = ""
		err = nil
	} else if err != nil {
		m.updater.checkError = err.Error()
	}
	m.updater.mu.Unlock()
	return err
}

// ApplyUpdate upgrades Tailscale to the latest stable version and verifies
// it; on any failure it rolls back to the previous version. It blocks.
func (m *Manager) ApplyUpdate(ctx context.Context, auto bool) (*UpdateResult, error) {
	if ok, reason := m.updateSupport(); !ok {
		return nil, errors.New(reason)
	}
	if !m.beginBusy(true) {
		return nil, errors.New("another check or update is already running")
	}
	defer m.endBusy()

	res := &UpdateResult{Time: time.Now(), Auto: auto}
	// Background runs only record real install attempts, so periodic no-op
	// checks never overwrite the last meaningful result shown in the panel.
	persist := !auto
	defer func() {
		if !persist {
			return
		}
		if b, err := json.Marshal(res); err == nil {
			m.putSetting(settingLastResult, string(b))
		}
	}()

	current, err := m.CurrentVersion()
	if err != nil {
		res.Message = err.Error()
		return res, err
	}
	res.From = current

	// Always re-check right before applying.
	if err := m.check(ctx); err != nil {
		res.Message = "version check failed: " + err.Error()
		return res, err
	}
	m.updater.mu.Lock()
	latest, candidate := m.updater.latest, m.updater.candidate
	m.updater.mu.Unlock()
	res.To = latest

	if candidate == "" || compareVersions(latest, current) <= 0 {
		res.Success = true
		res.To = current
		res.Message = "Already up to date"
		return res, nil
	}

	if auto && m.setting(settingSkip) == latest {
		res.Message = fmt.Sprintf("Version %s previously failed and is skipped for automatic updates", latest)
		return res, errors.New(res.Message)
	}

	previousPkg, _ := installedPackageVersion()
	wasConnected := false
	if s, err := m.GetStatus(); err == nil && s != nil {
		wasConnected = s.Connected
	}

	log.Printf("tailscale: updating %s -> %s", current, latest)
	persist = true

	fail := func(msg string) (*UpdateResult, error) {
		log.Printf("tailscale: update to %s failed: %s", latest, msg)
		res.Message = msg
		if previousPkg != "" {
			if rbErr := m.rollback(ctx, previousPkg, current, wasConnected); rbErr != nil {
				res.Message += "; ROLLBACK FAILED: " + rbErr.Error()
			} else {
				res.RolledBack = true
				res.Message += "; rolled back to " + current
			}
		} else {
			res.Message += "; previous package version unknown, rollback skipped"
		}
		m.putSetting(settingSkip, latest)
		return res, errors.New(res.Message)
	}

	if err := aptInstall(ctx, candidate, false); err != nil {
		return fail(err.Error())
	}
	if v, err := m.CurrentVersion(); err != nil || compareVersions(v, latest) != 0 {
		return fail(fmt.Sprintf("installed binary reports %q, expected %s", v, latest))
	}
	if err := m.restartDaemon(); err != nil {
		return fail("daemon restart failed: " + err.Error())
	}
	if err := m.verify(latest, wasConnected); err != nil {
		return fail("post-update verification failed: " + err.Error())
	}

	m.putSetting(settingSkip, "")
	res.Success = true
	res.Message = fmt.Sprintf("Updated from %s to %s", current, latest)
	log.Printf("tailscale: %s", res.Message)

	m.updater.mu.Lock()
	cb := m.updater.afterApply
	m.updater.mu.Unlock()
	if cb != nil {
		cb()
	}
	return res, nil
}

// verify waits until the restarted daemon runs the expected version and, if
// Tailscale was connected before the update, is connected again.
func (m *Manager) verify(expected string, wasConnected bool) error {
	if !waitDaemonReady(45 * time.Second) {
		return errors.New("tailscaled did not come back")
	}
	deadline := time.Now().Add(60 * time.Second)
	for {
		dv := daemonVersion()
		if dv != "" && compareVersions(dv, expected) == 0 {
			if !wasConnected {
				return nil
			}
			if s, err := m.GetStatus(); err == nil && s != nil && s.Connected {
				return nil
			}
		}
		if time.Now().After(deadline) {
			if dv != "" && compareVersions(dv, expected) != 0 {
				return fmt.Errorf("daemon runs %s, expected %s", dv, expected)
			}
			if wasConnected {
				return errors.New("tailscale did not reconnect")
			}
			return errors.New("daemon version unavailable")
		}
		time.Sleep(2 * time.Second)
	}
}

// rollback reinstalls the previous package version and restarts the daemon.
func (m *Manager) rollback(ctx context.Context, pkgVersion, expected string, wasConnected bool) error {
	// The caller's context may already be cancelled; rollback must still run.
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 6*time.Minute)
	defer cancel()
	if err := aptInstall(rctx, pkgVersion, true); err != nil {
		return err
	}
	if err := m.restartDaemon(); err != nil {
		return err
	}
	return m.verify(expected, wasConnected)
}

func installedPackageVersion() (string, error) {
	out, err := exec.Command("dpkg-query", "-W", "-f=${Version}", "tailscale").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func aptInstall(ctx context.Context, pkgVersion string, allowDowngrade bool) error {
	ictx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	args := []string{
		"install", "-y", "--no-install-recommends",
		"-o", "Dpkg::Options::=--force-confold",
		"-o", "Acquire::Retries=2",
	}
	if allowDowngrade {
		args = append(args, "--allow-downgrades")
	}
	args = append(args, "tailscale="+pkgVersion)
	cmd := exec.CommandContext(ictx, "apt-get", args...)
	cmd.Env = aptEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("apt-get install tailscale=%s failed: %s", pkgVersion, lastLines(string(out), 4))
	}
	return nil
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}

// ---------------------------------------------------------------------------
// Background auto-updater
// ---------------------------------------------------------------------------

// RunAutoUpdate periodically checks for a new stable release and, when the
// auto-update switch is on, installs it. It blocks until ctx is cancelled.
func (m *Manager) RunAutoUpdate(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 6 * time.Hour
	}
	// Give tailscaled and the panel time to settle after a start.
	sleepCtx(ctx, 3*time.Minute)
	for ctx.Err() == nil {
		m.autoTick(ctx)
		sleepCtx(ctx, interval)
	}
}

func (m *Manager) autoTick(ctx context.Context) {
	if ok, _ := m.updateSupport(); !ok {
		return
	}
	if !m.AutoUpdateEnabled() {
		// Keep the "update available" hint fresh without changing anything.
		if m.beginBusy(false) {
			_ = m.check(ctx)
			m.endBusy()
		}
		return
	}
	res, err := m.ApplyUpdate(ctx, true)
	if err != nil {
		log.Printf("tailscale: auto-update: %v", err)
		return
	}
	if res != nil && res.Message != "Already up to date" {
		log.Printf("tailscale: auto-update: %s", res.Message)
	}
}
