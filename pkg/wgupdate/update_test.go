package wgupdate

import (
	"os"
	"testing"
)

type memStore map[string]string

func (m memStore) GetSetting(k string) (string, error) { return m[k], nil }
func (m memStore) SetSetting(k, v string) error        { m[k] = v; return nil }

func TestAutoEnabledDefaultsOn(t *testing.T) {
	os.Unsetenv("WIREGUARD_AUTO_UPDATE")
	s := memStore{}
	u := New(s, "wg0", nil)
	if !u.AutoEnabled() {
		t.Fatal("auto update must default to on")
	}
	u.SetAuto(false)
	if u.AutoEnabled() {
		t.Fatal("saved switch must win")
	}
	u.SetAuto(true)
	t.Setenv("WIREGUARD_AUTO_UPDATE", "false")
	if !u.AutoEnabled() {
		t.Fatal("saved switch must win over env")
	}
	delete(s, SettingAuto)
	if u.AutoEnabled() {
		t.Fatal("env false must disable when nothing is saved")
	}
}

func TestDpkgGreater(t *testing.T) {
	if _, err := os.Stat("/usr/bin/dpkg"); err != nil {
		t.Skip("dpkg not available")
	}
	if !dpkgGreater("1.0.20210914-1ubuntu2", "1.0.20210914-1ubuntu1") || dpkgGreater("1.0-1", "1.0-1") || dpkgGreater("", "1") {
		t.Fatal("dpkg comparison wrong")
	}
}

func TestExclusiveDefault(t *testing.T) {
	ran := false
	New(nil, "wg0", nil).exclusive(func() { ran = true })
	if !ran {
		t.Fatal("exclusive fallback must run fn")
	}
}
