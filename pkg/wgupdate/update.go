// Package wgupdate keeps the WireGuard userspace tools (wireguard-tools: wg,
// wg-quick) up to date inside the container.
//
// The tools only configure the interface; the WireGuard data path runs in the
// kernel of the host. Replacing the package therefore never drops a tunnel.
// Every update is verified (tools run, interface still up, same peers) and
// rolled back to the previous package version if anything looks wrong.
package wgupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	pkgName           = "wireguard-tools"
	SettingAuto       = "wireguard_auto_update"
	settingLastResult = "wireguard_update_last"
	settingSkip       = "wireguard_update_skip_version"
)

// Store persists small key/value settings (implemented by the panel database).
type Store interface {
	GetSetting(key string) (string, error)
	SetSetting(key, value string) error
}

type Result struct {
	Time       time.Time `json:"time"`
	From       string    `json:"from"`
	To         string    `json:"to"`
	Success    bool      `json:"success"`
	RolledBack bool      `json:"rolled_back"`
	Message    string    `json:"message"`
	Auto       bool      `json:"auto"`
}

type Status struct {
	Installed       bool       `json:"installed"`
	Supported       bool       `json:"supported"`
	Unsupported     string     `json:"unsupported_reason,omitempty"`
	Current         string     `json:"current_version"`
	Latest          string     `json:"latest_version"`
	UpdateAvailable bool       `json:"update_available"`
	AutoUpdate      bool       `json:"auto_update"`
	Checking        bool       `json:"checking"`
	Updating        bool       `json:"updating"`
	CheckedAt       *time.Time `json:"checked_at,omitempty"`
	CheckError      string     `json:"check_error,omitempty"`
	SkippedVersion  string     `json:"skipped_version,omitempty"`
	LastResult      *Result    `json:"last_result,omitempty"`
}

// Updater is safe for concurrent use.
type Updater struct {
	store Store
	iface string
	// exclusive runs fn while no other WireGuard configuration change is running.
	exclusive func(fn func())

	mu         sync.Mutex
	busy       bool
	checking   bool
	updating   bool
	latest     string
	checkedAt  time.Time
	checkError string
}

func New(store Store, iface string, exclusive func(fn func())) *Updater {
	if exclusive == nil {
		exclusive = func(fn func()) { fn() }
	}
	return &Updater{store: store, iface: iface, exclusive: exclusive}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func aptEnv() []string { return append(os.Environ(), "DEBIAN_FRONTEND=noninteractive", "LC_ALL=C") }

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}

// dpkgGreater reports whether Debian version a is newer than b.
func dpkgGreater(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return exec.Command("dpkg", "--compare-versions", a, "gt", b).Run() == nil
}

var toolVersionRe = regexp.MustCompile(`v?(\d+\.\d+\.\d+)`)

// ToolsVersion returns the version printed by `wg --version` (e.g. 1.0.20210914).
func ToolsVersion() string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "wg", "--version").Output()
	if err != nil {
		return ""
	}
	if m := toolVersionRe.FindStringSubmatch(string(out)); m != nil {
		return m[1]
	}
	return ""
}

func installedPackage() string {
	out, err := exec.Command("dpkg-query", "-W", "-f=${Version}", pkgName).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func candidatePackage(ctx context.Context) (string, error) {
	upd := exec.CommandContext(ctx, "apt-get", "update", "-o", "Acquire::Retries=2", "-o", "Acquire::http::Timeout=20")
	upd.Env = aptEnv()
	if out, err := upd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("apt-get update failed: %s", lastLines(string(out), 3))
	}
	pol := exec.CommandContext(ctx, "apt-cache", "policy", pkgName)
	pol.Env = aptEnv()
	out, err := pol.Output()
	if err != nil {
		return "", fmt.Errorf("apt-cache policy failed: %w", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); strings.HasPrefix(line, "Candidate:") {
			c := strings.TrimSpace(strings.TrimPrefix(line, "Candidate:"))
			if c == "" || c == "(none)" {
				break
			}
			return c, nil
		}
	}
	return "", errors.New("no wireguard-tools candidate found in apt")
}

func aptInstall(ctx context.Context, version string, downgrade bool) error {
	ictx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	args := []string{"install", "-y", "--no-install-recommends", "-o", "Dpkg::Options::=--force-confold", "-o", "Acquire::Retries=2"}
	if downgrade {
		args = append(args, "--allow-downgrades")
	}
	args = append(args, pkgName+"="+version)
	cmd := exec.CommandContext(ictx, "apt-get", args...)
	cmd.Env = aptEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("apt-get install %s=%s failed: %s", pkgName, version, lastLines(string(out), 4))
	}
	return nil
}

// snapshotFiles archives the regular files of the installed package. The
// Ubuntu archive drops superseded versions from its index, so an apt based
// rollback can be impossible; the archive is the fallback that always works.
func snapshotFiles(dir string) (string, error) {
	out, err := exec.Command("dpkg", "-L", pkgName).Output()
	if err != nil {
		return "", fmt.Errorf("dpkg -L failed: %w", err)
	}
	var rel []string
	for _, f := range strings.Split(string(out), "\n") {
		f = strings.TrimSpace(f)
		if !strings.HasPrefix(f, "/") || f == "/" {
			continue
		}
		if fi, err := os.Lstat(f); err == nil && !fi.IsDir() {
			rel = append(rel, strings.TrimPrefix(f, "/"))
		}
	}
	if len(rel) == 0 {
		return "", errors.New("no package files found")
	}
	list := filepath.Join(dir, "files.txt")
	if err := os.WriteFile(list, []byte(strings.Join(rel, "\n")+"\n"), 0o600); err != nil {
		return "", err
	}
	archive := filepath.Join(dir, "snapshot.tar")
	if b, err := exec.Command("tar", "-cf", archive, "-C", "/", "--files-from", list).CombinedOutput(); err != nil {
		return "", fmt.Errorf("tar failed: %s", lastLines(string(b), 2))
	}
	return archive, nil
}

func restoreSnapshot(archive string) error {
	if b, err := exec.Command("tar", "-xpf", archive, "-C", "/").CombinedOutput(); err != nil {
		return fmt.Errorf("tar restore failed: %s", lastLines(string(b), 2))
	}
	return nil
}

// peerCount returns the number of peers on the interface, or -1 if it is down.
func (u *Updater) peerCount() int {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "wg", "show", u.iface, "peers").Output()
	if err != nil {
		return -1
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return 0
	}
	return len(strings.Split(s, "\n"))
}

func (u *Updater) get(key string) string {
	if u.store == nil {
		return ""
	}
	v, _ := u.store.GetSetting(key)
	return v
}

func (u *Updater) put(key, val string) {
	if u.store != nil {
		if err := u.store.SetSetting(key, val); err != nil {
			log.Printf("wireguard: could not save %s: %v", key, err)
		}
	}
}

// Support reports whether in-place updates can work here.
func (u *Updater) Support() (bool, string) {
	if _, err := exec.LookPath("wg"); err != nil {
		return false, "WireGuard tools are not installed"
	}
	if os.Geteuid() != 0 {
		return false, "panel is not running as root"
	}
	if _, err := exec.LookPath("apt-get"); err != nil {
		return false, "apt-get is not available (non-Debian/Ubuntu system)"
	}
	if installedPackage() == "" {
		return false, "wireguard-tools was not installed from a package"
	}
	return true, ""
}

// AutoEnabled: the saved switch wins; otherwise WIREGUARD_AUTO_UPDATE (default on).
func (u *Updater) AutoEnabled() bool {
	switch u.get(SettingAuto) {
	case "true":
		return true
	case "false":
		return false
	}
	return strings.ToLower(os.Getenv("WIREGUARD_AUTO_UPDATE")) != "false"
}

func (u *Updater) SetAuto(enabled bool) { u.put(SettingAuto, strconv.FormatBool(enabled)) }

func (u *Updater) Status() Status {
	ok, reason := u.Support()
	u.mu.Lock()
	defer u.mu.Unlock()
	cur := ToolsVersion()
	st := Status{
		Installed:      cur != "",
		Supported:      ok,
		Unsupported:    reason,
		Current:        cur,
		Latest:         u.latest,
		AutoUpdate:     u.AutoEnabled(),
		Checking:       u.checking,
		Updating:       u.updating,
		CheckError:     u.checkError,
		SkippedVersion: u.get(settingSkip),
	}
	if !u.checkedAt.IsZero() {
		t := u.checkedAt
		st.CheckedAt = &t
	}
	st.UpdateAvailable = dpkgGreater(u.latest, installedPackage())
	if raw := u.get(settingLastResult); raw != "" {
		var r Result
		if json.Unmarshal([]byte(raw), &r) == nil {
			st.LastResult = &r
		}
	}
	return st
}

func (u *Updater) begin(updating bool) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.busy {
		return false
	}
	u.busy, u.checking, u.updating = true, !updating, updating
	return true
}

func (u *Updater) end() {
	u.mu.Lock()
	u.busy, u.checking, u.updating = false, false, false
	u.mu.Unlock()
}

func (u *Updater) check(parent context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(parent, 3*time.Minute)
	defer cancel()
	cand, err := candidatePackage(ctx)
	u.mu.Lock()
	defer u.mu.Unlock()
	u.checkedAt = time.Now()
	if err != nil {
		u.checkError = err.Error()
		return "", err
	}
	u.checkError, u.latest = "", cand
	return cand, nil
}

// Check looks up the newest package version without changing anything.
func (u *Updater) Check(ctx context.Context) error {
	if !u.begin(false) {
		return errors.New("another check or update is already running")
	}
	defer u.end()
	_, err := u.check(ctx)
	return err
}

// Apply installs the newest package version, verifies it and rolls back on failure.
func (u *Updater) Apply(ctx context.Context, auto bool) (*Result, error) {
	if ok, reason := u.Support(); !ok {
		return nil, errors.New(reason)
	}
	if !u.begin(true) {
		return nil, errors.New("another check or update is already running")
	}
	defer u.end()

	res := &Result{Time: time.Now(), Auto: auto, From: ToolsVersion()}
	persist := !auto // background no-op runs never overwrite the last meaningful result
	defer func() {
		if persist {
			if b, err := json.Marshal(res); err == nil {
				u.put(settingLastResult, string(b))
			}
		}
	}()

	cand, err := u.check(ctx)
	if err != nil {
		res.Message = "version check failed: " + err.Error()
		return res, err
	}
	prev := installedPackage()
	if !dpkgGreater(cand, prev) {
		res.Success, res.To, res.Message = true, res.From, "Already up to date"
		return res, nil
	}
	if auto && u.get(settingSkip) == cand {
		res.Message = fmt.Sprintf("Version %s previously failed and is skipped for automatic updates", cand)
		return res, errors.New(res.Message)
	}
	persist = true
	res.To = cand

	var applyErr error
	snapDir, derr := os.MkdirTemp("", "wgupdate-")
	snapshot := ""
	if derr == nil {
		defer os.RemoveAll(snapDir)
		var serr error
		if snapshot, serr = snapshotFiles(snapDir); serr != nil {
			log.Printf("wireguard: no file snapshot for rollback: %v", serr)
		}
	}
	u.exclusive(func() {
		peersBefore := u.peerCount()
		if applyErr = aptInstall(ctx, cand, false); applyErr == nil {
			if v := ToolsVersion(); v == "" {
				applyErr = errors.New("wg does not run after the update")
			} else if peersBefore >= 0 && u.peerCount() != peersBefore {
				applyErr = fmt.Errorf("interface %s lost peers or went down after the update", u.iface)
			} else {
				res.To = v
			}
		}
		if applyErr != nil {
			res.Message = "update failed: " + applyErr.Error()
			if prev != "" {
				if rerr := aptInstall(ctx, prev, true); rerr == nil {
					res.RolledBack = true
					res.Message += "; previous version restored"
				} else if snapshot != "" {
					if serr := restoreSnapshot(snapshot); serr == nil && ToolsVersion() != "" {
						res.RolledBack = true
						res.Message += "; previous files restored from snapshot"
					} else {
						res.Message += "; rollback failed: " + rerr.Error()
					}
				} else {
					res.Message += "; rollback failed: " + rerr.Error()
				}
			}
		}
	})
	if applyErr != nil {
		u.put(settingSkip, cand)
		return res, applyErr
	}
	res.Success = true
	res.Message = fmt.Sprintf("Updated %s to %s; the interface stayed up", res.From, res.To)
	log.Printf("wireguard: %s", res.Message)
	return res, nil
}

// Run checks periodically and installs updates while auto-update is on.
func (u *Updater) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 6 * time.Hour
	}
	sleep := func(d time.Duration) {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-ctx.Done():
		case <-t.C:
		}
	}
	sleep(5 * time.Minute)
	for ctx.Err() == nil {
		if ok, _ := u.Support(); ok {
			if !u.AutoEnabled() {
				if u.begin(false) {
					_, _ = u.check(ctx)
					u.end()
				}
			} else if res, err := u.Apply(ctx, true); err != nil {
				log.Printf("wireguard: auto-update: %v", err)
			} else if res != nil && res.Message != "Already up to date" {
				log.Printf("wireguard: auto-update: %s", res.Message)
			}
		}
		sleep(interval)
	}
}
