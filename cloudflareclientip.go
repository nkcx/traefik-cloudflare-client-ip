// Package traefik_cloudflare_client_ip is a Traefik middleware plugin that gives backends the
// visitor's real IP when Traefik sits behind Cloudflare, without letting visitors forge it.
//
// Cloudflare appends the visitor's address to any X-Forwarded-For the visitor sent, so many
// backends (which read the first entry) can be fooled. Cloudflare also sets CF-Connecting-IP,
// which it overwrites, but anyone who reaches the origin directly can send that header too.
//
// This middleware looks at the connection's own address (not any header):
//   - from Cloudflare's ranges, with a valid CF-Connecting-IP: X-Forwarded-For and X-Real-Ip
//     are replaced with that address;
//   - from anywhere else: CF-Connecting-IP is removed, and X-Forwarded-For / X-Real-Ip are
//     left to Traefik (whose entrypoint replaces them for sources it doesn't trust).
//
// Use it on the entrypoint, and don't list Cloudflare in the entrypoint's
// forwardedHeaders.trustedIPs: this middleware is then the one place that trusts Cloudflare.
package traefik_cloudflare_client_ip

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
)

// DefaultHeader is the header Cloudflare puts the visitor's address in.
const DefaultHeader = "CF-Connecting-IP"

// DefaultTrustedCIDRs are Cloudflare's published ranges (https://www.cloudflare.com/ips/),
// as of 2026-11-07. Override with trustedCIDRs if they change before a new release.
var DefaultTrustedCIDRs = []string{
	"173.245.48.0/20", "103.21.244.0/22", "103.22.200.0/22", "103.31.4.0/22",
	"141.101.64.0/18", "108.162.192.0/18", "190.93.240.0/20", "188.114.96.0/20",
	"197.234.240.0/22", "198.41.128.0/17", "162.158.0.0/15", "104.16.0.0/13",
	"104.24.0.0/14", "172.64.0.0/13", "131.0.72.0/22",
	"2400:cb00::/32", "2606:4700::/32", "2803:f800::/32", "2405:b500::/32",
	"2405:8100::/32", "2a06:98c0::/29", "2c0f:f248::/32",
}

// Config is the plugin configuration.
type Config struct {
	// TrustedCIDRs replaces the built-in Cloudflare ranges when set.
	TrustedCIDRs []string `json:"trustedCIDRs,omitempty"`
	// Header is the header holding the visitor's address (default CF-Connecting-IP).
	Header string `json:"header,omitempty"`
}

// CreateConfig returns the default configuration.
func CreateConfig() *Config {
	return &Config{}
}

type clientIP struct {
	next   http.Handler
	name   string
	header string
	nets   []*net.IPNet
}

// New creates the middleware.
func New(_ context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	cidrs := config.TrustedCIDRs
	if len(cidrs) == 0 {
		cidrs = DefaultTrustedCIDRs
	}
	header := strings.TrimSpace(config.Header)
	if header == "" {
		header = DefaultHeader
	}
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(strings.TrimSpace(c))
		if err != nil {
			return nil, fmt.Errorf("%s: invalid trusted CIDR %q: %w", name, c, err)
		}
		nets = append(nets, n)
	}
	return &clientIP{next: next, name: name, header: header, nets: nets}, nil
}

func (c *clientIP) trusted(ip net.IP) bool {
	for _, n := range c.nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func peerIP(remoteAddr string) net.IP {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	return net.ParseIP(strings.Trim(host, "[]"))
}

func (c *clientIP) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	peer := peerIP(req.RemoteAddr)
	if peer != nil && c.trusted(peer) {
		if visitor := net.ParseIP(strings.TrimSpace(req.Header.Get(c.header))); visitor != nil {
			v := visitor.String()
			req.Header.Set("X-Forwarded-For", v)
			req.Header.Set("X-Real-Ip", v)
		} else {
			// From Cloudflare but without a usable header: don't pass on whatever was there.
			req.Header.Del(c.header)
		}
	} else {
		req.Header.Del(c.header)
	}
	c.next.ServeHTTP(rw, req)
}
