package traefik_cloudflare_client_ip

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

type seen struct{ xff, xri, cf string }

func run(t *testing.T, cfg *Config, remote string, headers map[string]string) seen {
	t.Helper()
	var got seen
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = seen{r.Header.Get("X-Forwarded-For"), r.Header.Get("X-Real-Ip"), r.Header.Get("CF-Connecting-IP")}
	})
	h, err := New(context.Background(), next, cfg, "test")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	req.RemoteAddr = remote
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	h.ServeHTTP(httptest.NewRecorder(), req)
	return got
}

func TestFromCloudflareUsesHeader(t *testing.T) {
	got := run(t, CreateConfig(), "104.21.1.157:443", map[string]string{
		"CF-Connecting-IP": "69.9.136.25",
		"X-Forwarded-For":  "203.0.113.66, 69.9.136.25", // visitor-forged first entry
	})
	if got.xff != "69.9.136.25" || got.xri != "69.9.136.25" {
		t.Fatalf("want real IP in XFF and X-Real-Ip, got %+v", got)
	}
}

func TestFromCloudflareIPv6Peer(t *testing.T) {
	got := run(t, CreateConfig(), "[2606:4700::1]:443", map[string]string{"CF-Connecting-IP": "2001:db8::5"})
	if got.xff != "2001:db8::5" {
		t.Fatalf("got %+v", got)
	}
}

func TestFromCloudflareBadHeaderIsDropped(t *testing.T) {
	got := run(t, CreateConfig(), "104.21.1.157:443", map[string]string{
		"CF-Connecting-IP": "not-an-ip",
		"X-Forwarded-For":  "198.51.100.7",
	})
	if got.cf != "" || got.xff != "198.51.100.7" {
		t.Fatalf("want header dropped and XFF untouched, got %+v", got)
	}
}

func TestDirectToOriginForgedHeaderIsStripped(t *testing.T) {
	got := run(t, CreateConfig(), "69.9.136.25:50000", map[string]string{
		"CF-Connecting-IP": "203.0.113.66",
		"X-Forwarded-For":  "69.9.136.25",
	})
	if got.cf != "" {
		t.Fatalf("forged CF-Connecting-IP must be removed, got %+v", got)
	}
	if got.xff != "69.9.136.25" {
		t.Fatalf("XFF from an untrusted peer is Traefik's business, got %+v", got)
	}
}

func TestCustomCIDRsAndHeader(t *testing.T) {
	cfg := &Config{TrustedCIDRs: []string{"10.0.0.0/8"}, Header: "True-Client-IP"}
	got := run(t, cfg, "10.1.2.3:1234", map[string]string{"True-Client-IP": "192.0.2.10"})
	if got.xff != "192.0.2.10" {
		t.Fatalf("got %+v", got)
	}
	got = run(t, cfg, "104.21.1.157:443", map[string]string{"CF-Connecting-IP": "192.0.2.10"})
	if got.xff != "" {
		t.Fatalf("Cloudflare isn't trusted when the list is replaced, got %+v", got)
	}
}

func TestInvalidCIDRFails(t *testing.T) {
	if _, err := New(context.Background(), http.NotFoundHandler(), &Config{TrustedCIDRs: []string{"nope"}}, "test"); err == nil {
		t.Fatal("want an error for an invalid CIDR")
	}
}
