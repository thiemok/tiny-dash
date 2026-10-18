package apiurl

import (
	"errors"
	"net/netip"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		in         string
		scheme     string
		host       string
		port       uint16
		prefix     string
		hostHeader string
		ip         bool
		str        string
	}{
		{"http://192.0.2.10:8080", "http", "192.0.2.10", 8080, "", "192.0.2.10:8080", true, "http://192.0.2.10:8080"},
		{"http://192.0.2.20", "http", "192.0.2.20", 80, "", "192.0.2.20", true, "http://192.0.2.20"},
		{"http://192.0.2.20:80/", "http", "192.0.2.20", 80, "", "192.0.2.20", true, "http://192.0.2.20"},
		{"https://dashboard.example.com", "https", "dashboard.example.com", 443, "", "dashboard.example.com", false, "https://dashboard.example.com"},
		{"https://Dash.Example.net:8443/tiny/dash/", "https", "dash.example.net", 8443, "/tiny/dash", "dash.example.net:8443", false, "https://dash.example.net:8443/tiny/dash"},
		{"HTTP://example.net:443", "http", "example.net", 443, "", "example.net:443", false, "http://example.net:443"},
		{"https://example.net:80", "https", "example.net", 80, "", "example.net:80", false, "https://example.net:80"},
		{"  http://10.0.0.1/api-prefix  ", "http", "10.0.0.1", 80, "/api-prefix", "10.0.0.1", true, "http://10.0.0.1/api-prefix"},
		{"http://[fe80::1]:8080/p", "http", "fe80::1", 8080, "/p", "[fe80::1]:8080", true, "http://[fe80::1]:8080/p"},
		{"http://[::1]", "http", "::1", 80, "", "[::1]", true, "http://[::1]"},
		{"http://example.net/a%20b", "http", "example.net", 80, "/a%20b", "example.net", false, "http://example.net/a%20b"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			u, err := Parse(tt.in)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if u.Scheme != tt.scheme || u.Host != tt.host || u.Port != tt.port || u.PathPrefix != tt.prefix {
				t.Errorf("got scheme=%q host=%q port=%d prefix=%q", u.Scheme, u.Host, u.Port, u.PathPrefix)
			}
			if got := u.HostHeader(); got != tt.hostHeader {
				t.Errorf("HostHeader = %q, want %q", got, tt.hostHeader)
			}
			if u.IsIPLiteral() != tt.ip {
				t.Errorf("IsIPLiteral = %v, want %v", u.IsIPLiteral(), tt.ip)
			}
			if got := u.String(); got != tt.str {
				t.Errorf("String = %q, want %q", got, tt.str)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		in  string
		err error
	}{
		{"", ErrEmpty},
		{"   ", ErrEmpty},
		{"ftp://example.net", ErrUnsupportedScheme},
		{"example.net", ErrUnsupportedScheme},
		{"192.168.1.1:8080", ErrUnsupportedScheme},
		{"http://", ErrMissingHost},
		{"http:example.net", ErrMissingHost},
		{"http://:8080", ErrMissingHost},
		{"http://example.net:0", ErrInvalidPort},
		{"http://example.net:", ErrInvalidPort},
		{"http://user:pw@example.net", ErrUserinfo},
		{"http://example.net/?a=b", ErrQueryOrFragment},
		{"http://example.net/?", ErrQueryOrFragment},
		{"http://example.net/#x", ErrQueryOrFragment},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			_, err := Parse(tt.in)
			if !errors.Is(err, tt.err) {
				t.Fatalf("Parse(%q) err = %v, want %v", tt.in, err, tt.err)
			}
		})
	}
	for _, in := range []string{"http://example.net:65536", "http://example.net:abc", "http://[::1"} {
		if _, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) succeeded, want error", in)
		}
	}
}

func TestAddrPort(t *testing.T) {
	u, err := Parse("http://192.0.2.10:8080/x")
	if err != nil {
		t.Fatal(err)
	}
	ap, ok := u.AddrPort()
	if !ok || ap != netip.MustParseAddrPort("192.0.2.10:8080") {
		t.Errorf("AddrPort = %v, %v", ap, ok)
	}
	if got := u.RequestTarget("/api/dashboard/image?w=1"); got != "/x/api/dashboard/image?w=1" {
		t.Errorf("RequestTarget = %q", got)
	}

	u, err = Parse("https://example.net")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := u.AddrPort(); ok {
		t.Error("AddrPort ok for host name")
	}
	if !u.IsDefaultPort() {
		t.Error("IsDefaultPort false for https:443")
	}
}

func TestAccessorsDoNotAllocate(t *testing.T) {
	u, err := Parse("http://192.0.2.10:8080/x")
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 0, 64)
	allocs := testing.AllocsPerRun(100, func() {
		_ = u.HostHeader()
		_, _ = u.AddrPort()
		_ = u.IsDefaultPort()
		buf = u.AppendRequestTarget(buf[:0], "/api/dashboard/image")
	})
	if allocs != 0 {
		t.Errorf("accessors allocated %v times per run, want 0", allocs)
	}
	if got := string(buf); got != "/x/api/dashboard/image" {
		t.Errorf("AppendRequestTarget = %q", got)
	}
}
