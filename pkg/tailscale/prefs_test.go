package tailscale

import (
	"reflect"
	"testing"
)

func TestValidateRoutes(t *testing.T) {
	got, err := ValidateRoutes([]string{" 192.168.1.5/24 ", "192.168.1.0/24", "", "fd00::1/64"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"192.168.1.0/24", "fd00::/64"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for _, bad := range []string{"0.0.0.0/0", "::/0", "10.0.0.1", "nope", "10.0.0.0/33"} {
		if _, err := ValidateRoutes([]string{bad}); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

func TestBuildSetArgs(t *testing.T) {
	got := buildSetArgs(Prefs{AdvertiseRoutes: []string{"10.0.0.0/8", "172.16.0.0/12"}, ShieldsUp: true})
	want := []string{"set", "--advertise-exit-node=false", "--advertise-routes=10.0.0.0/8,172.16.0.0/12", "--shields-up=true", "--accept-routes=false"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}
