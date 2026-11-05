// Package aptutil coordinates apt usage between the WireGuard and Tailscale
// updaters and keeps the container from growing: apt metadata and downloaded
// packages are removed as soon as an update check or update has finished.
package aptutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

var mu sync.Mutex

// Acquire serialises apt/dpkg use across updaters (dpkg allows one at a time
// anyway, and one updater must not clean the package lists under another).
func Acquire() { mu.Lock() }

// Release removes apt package lists and cached .deb files, then unlocks.
func Release() {
	Cleanup()
	mu.Unlock()
}

// Cleanup frees disk space used by apt. The next check downloads fresh lists.
func Cleanup() {
	// Only inside a container: never touch the apt state of a bare-metal host.
	if _, err := os.Stat("/.dockerenv"); err != nil && os.Getenv("APT_CLEANUP") != "true" {
		return
	}
	_ = exec.Command("apt-get", "clean").Run()
	entries, err := os.ReadDir("/var/lib/apt/lists")
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.Name() == "partial" || e.Name() == "lock" {
			continue
		}
		_ = os.RemoveAll(filepath.Join("/var/lib/apt/lists", e.Name()))
	}
}
