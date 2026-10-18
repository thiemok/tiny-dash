package apiurl

import (
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// parseNetURL is the url.Parse based implementation Parse replaced; it is the
// reference for TestParseMatchesNetURL.
func parseNetURL(s string) (URL, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return URL{}, ErrEmpty
	}
	pu, err := url.Parse(s)
	if err != nil {
		return URL{}, err
	}
	var u URL
	switch strings.ToLower(pu.Scheme) {
	case SchemeHTTP:
		u.Scheme, u.Port = SchemeHTTP, DefaultPortHTTP
	case SchemeHTTPS:
		u.Scheme, u.Port = SchemeHTTPS, DefaultPortHTTPS
	default:
		return URL{}, ErrUnsupportedScheme
	}
	if pu.Opaque != "" {
		return URL{}, ErrMissingHost
	}
	if pu.User != nil {
		return URL{}, ErrUserinfo
	}
	if pu.RawQuery != "" || pu.ForceQuery || pu.Fragment != "" {
		return URL{}, ErrQueryOrFragment
	}
	u.Host = strings.ToLower(pu.Hostname())
	if u.Host == "" {
		return URL{}, ErrMissingHost
	}
	if p := pu.Port(); p != "" {
		n, err := strconv.ParseUint(p, 10, 16)
		if err != nil || n == 0 {
			return URL{}, ErrInvalidPort
		}
		u.Port = uint16(n)
	} else if strings.HasSuffix(pu.Host, ":") {
		return URL{}, ErrInvalidPort
	}
	if addr, err := netip.ParseAddr(u.Host); err == nil {
		u.Addr = addr
	}
	u.PathPrefix = strings.TrimRight(pu.EscapedPath(), "/")
	hh := u.Host
	if u.Addr.Is6() {
		hh = "[" + hh + "]"
	}
	if !u.IsDefaultPort() {
		hh += ":" + strconv.Itoa(int(u.Port))
	}
	u.hostHeader = hh
	return u, nil
}

func TestParseMatchesNetURL(t *testing.T) {
	inputs := []string{
		"http://192.0.2.10:8080",
		"https://dashboard.example.com",
		"https://Dash.Example.net:8443/tiny/dash/",
		"HTTPS://EXAMPLE.NET/",
		"hTtP://example.net:00080/x",
		"http://example.net:65535",
		"http://[fe80::1]:8080/p",
		"http://[FE80::AB]/",
		"http://[fe80::1%25en0]:80",
		"http://[::ffff:1.2.3.4]",
		"http://example.net/a%20b",
		"http://example.net/a b/c",
		"http://example.net/a%2Fb/",
		"http://example.net/p@x:y;z=1,!$&'()*+[]",
		"http://example.net/ü",
		"http://example.net/a\"b<c>",
		"http://example.net//",
		"http://example.net#",
		"http://example.net/#",
		"http://exa_mple.net~",
		"http://ex!$&'()*+,;=ample",
		"http://xn--bcher-kva.example",
		"http://bücher.example",
		"",
		"ftp://example.net",
		"example.net",
		"http:example.net",
		"http:/example.net",
		"http://",
		"http:///path",
		"http://:8080",
		"http://example.net:",
		"http://example.net:0",
		"http://example.net:65536",
		"http://example.net:abc",
		"http://example.net:80:81",
		"http://a:b:80",
		"http://[::1",
		"http://[::1]x",
		"http://[1.2.3.4]",
		"http://[zz]",
		"http://user@example.net",
		"http://user:pw@example.net",
		"http://example.net?",
		"http://example.net/?a=b",
		"http://example.net/#x",
		"http://example.net/a%zz",
		"http://example.net/a%2",
		"http://exa mple.net",
		"http://exa{mple.net",
		"http://ex%41mple.net",
		"http://example.net/a\x7fb",
		"http://example.net/a\nb",
	}
	for _, in := range inputs {
		got, gotErr := Parse(in)
		want, wantErr := parseNetURL(in)
		if (gotErr == nil) != (wantErr == nil) {
			t.Errorf("Parse(%q) err = %v, net/url err = %v", in, gotErr, wantErr)
			continue
		}
		if gotErr == nil && got != want {
			t.Errorf("Parse(%q) = %+v, net/url = %+v", in, got, want)
		}
	}
}
