package wgmanager

import (
	"strings"
	"testing"
	"text/template"
)

func TestClientConfigDNSOptional(t *testing.T) {
	tpl := template.Must(template.New("c").Parse(clientConfigTemplate))
	for _, tc := range []struct {
		dns  string
		want bool
	}{{"", false}, {"1.1.1.1", true}} {
		var b strings.Builder
		if err := tpl.Execute(&b, ClientConfig{PrivateKey: "k", Address: "10.8.0.2", DNS: tc.dns, ServerPublicKey: "p", ServerEndpoint: "h:1", AllowedIPs: "0.0.0.0/0"}); err != nil {
			t.Fatal(err)
		}
		out := b.String()
		if got := strings.Contains(out, "DNS ="); got != tc.want {
			t.Fatalf("dns=%q: DNS line present=%v\n%s", tc.dns, got, out)
		}
		if !strings.Contains(out, "Address = 10.8.0.2/32\n\n[Peer]") && !tc.want {
			t.Fatalf("unexpected layout:\n%s", out)
		}
	}
}
