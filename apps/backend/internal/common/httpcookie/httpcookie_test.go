package httpcookie

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func req(host, xfh string) *http.Request {
	r := httptest.NewRequest("GET", "/", nil)
	r.Host = host
	if xfh != "" {
		r.Header.Set("X-Forwarded-Host", xfh)
	}
	return r
}

func TestPortSuffix(t *testing.T) {
	tests := []struct {
		name string
		host string
		xfh  string
		want string
	}{
		{"ported host", "127.0.0.1:8443", "", "_8443"},
		{"no port", "example.com", "", ""},
		{"default port 80", "example.com:80", "", ""},
		{"default port 443", "example.com:443", "", ""},
		{"XFH wins over host", "internal:38429", "public.example:8443", "_8443"},
		{"XFH wins on conflicting ports", "public.example:8443", "public.example:9443", "_9443"},
		{"XFH comma separated first", "internal:38429", "public.example:8443, other:9443", "_8443"},
		{"XFH whitespace padded", "internal:38429", "  public.example:8443 , other:9443", "_8443"},
		{"XFH whitespace only", "internal:38429", "   ", ""},
		{"XFH no port", "internal:38429", "public.example", ""},
		{"IPv6 bracket", "[::1]:8080", "", "_8080"},
		{"IPv6 zone", "[fe80::1%25eth0]:8080", "", "_8080"},
		{"service name port", "example.com:http", "", ""},
		{"out of range port", "example.com:99999", "", ""},
		{"zero port", "example.com:0", "", ""},
		{"negative port", "example.com:-1", "", ""},
		{"malformed host", "example.com:abc:def", "", ""},
		{"unbracketed ipv6", "::1", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PortSuffix(req(tt.host, tt.xfh)); got != tt.want {
				t.Fatalf("PortSuffix(%q, xfh=%q) = %q, want %q", tt.host, tt.xfh, got, tt.want)
			}
		})
	}
}

func TestPortSuffixNilRequest(t *testing.T) {
	if got := PortSuffix(nil); got != "" {
		t.Fatalf("PortSuffix(nil) = %q, want empty", got)
	}
}

func TestScopedName(t *testing.T) {
	if got := ScopedName(req("127.0.0.1:8443", ""), "kandev_session"); got != "kandev_session_8443" {
		t.Fatalf("ScopedName ported = %q, want kandev_session_8443", got)
	}
	if got := ScopedName(req("example.com", ""), "kandev_session"); got != "kandev_session" {
		t.Fatalf("ScopedName no-port = %q, want kandev_session", got)
	}
	if got := ScopedName(req("127.0.0.1:8443", ""), ""); got != "" {
		t.Fatalf("ScopedName empty base = %q, want empty", got)
	}
}
