package wgmanager

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

var hostLabelRe = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)

// NormalizeEndpointHost validates the host shown in client configs. It accepts a
// DNS hostname, an IPv4 or an IPv6 address - never a port, scheme or path, because
// the port is fixed. An empty value means "use the detected server address".
func NormalizeEndpointHost(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", nil
	}
	v = strings.TrimSuffix(strings.TrimPrefix(v, "["), "]")
	if ip := net.ParseIP(v); ip != nil {
		return ip.String(), nil
	}
	if len(v) > 253 || strings.ContainsAny(v, ":/@ ?#") {
		return "", fmt.Errorf("endpoint must be a hostname or IP address without port or scheme")
	}
	v = strings.TrimSuffix(strings.ToLower(v), ".")
	for _, label := range strings.Split(v, ".") {
		if !hostLabelRe.MatchString(label) {
			return "", fmt.Errorf("endpoint %q is not a valid hostname", v)
		}
	}
	return v, nil
}

// ClientEndpoint returns the Endpoint written into client configs: the configured
// host (if any) with the fixed port of the server endpoint.
func ClientEndpoint(serverEndpoint, host string) string {
	if host == "" {
		return serverEndpoint
	}
	_, port, err := net.SplitHostPort(serverEndpoint)
	if err != nil {
		return serverEndpoint
	}
	return net.JoinHostPort(host, port) // brackets IPv6 automatically
}
