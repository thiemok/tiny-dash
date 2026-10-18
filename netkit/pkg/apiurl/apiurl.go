// Package apiurl parses the base URL of an HTTP API, e.g.
// "https://dash.example.net/prefix" or "http://192.168.1.10:8080".
//
// Parsing is meant to run once at startup; the accessors on URL do not
// allocate except where noted, so the result can be used in allocation-free
// request loops.
//
// The parser deliberately avoids net/url: since Go 1.26 url.Parse consults
// the urlstrictcolons GODEBUG, which links internal/godebug and its settings
// table into TinyGo binaries (about 1.8 KB of RAM and 7 KB of flash on the
// RP2350). It accepts the same URLs as url.Parse with Go 1.26's strict-colon
// rule for http/https hosts.
package apiurl

import (
	"errors"
	"net/netip"
	"strconv"
	"strings"
)

const (
	SchemeHTTP  = "http"
	SchemeHTTPS = "https"

	DefaultPortHTTP  = 80
	DefaultPortHTTPS = 443
)

var (
	ErrEmpty             = errors.New("apiurl: empty URL")
	ErrUnsupportedScheme = errors.New("apiurl: scheme must be http or https")
	ErrMissingHost       = errors.New("apiurl: missing host")
	ErrInvalidHost       = errors.New("apiurl: invalid host")
	ErrInvalidPort       = errors.New("apiurl: invalid port")
	ErrInvalidPath       = errors.New("apiurl: invalid path escape")
	ErrInvalidChar       = errors.New("apiurl: control character in URL")
	ErrUserinfo          = errors.New("apiurl: userinfo is not supported")
	ErrQueryOrFragment   = errors.New("apiurl: query and fragment are not supported")
)

// URL is a parsed API base URL.
type URL struct {
	// Scheme is SchemeHTTP or SchemeHTTPS.
	Scheme string
	// Host is the host name or IP literal, without brackets or port.
	Host string
	// Port is the explicit port, or the scheme's default port.
	Port uint16
	// PathPrefix is "" or a path starting with "/" and without trailing slash.
	PathPrefix string
	// Addr is the parsed IP when Host is an IP literal, otherwise invalid.
	Addr netip.Addr

	hostHeader string
}

// Parse parses s, which must be an absolute http or https URL without
// userinfo, query or fragment. Ports default to 80/443 by scheme.
func Parse(s string) (URL, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return URL{}, ErrEmpty
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] == 0x7f {
			return URL{}, ErrInvalidChar
		}
	}
	var u URL
	scheme, rest, _ := strings.Cut(s, ":")
	switch strings.ToLower(scheme) {
	case SchemeHTTP:
		u.Scheme, u.Port = SchemeHTTP, DefaultPortHTTP
	case SchemeHTTPS:
		u.Scheme, u.Port = SchemeHTTPS, DefaultPortHTTPS
	default:
		return URL{}, ErrUnsupportedScheme
	}

	// An empty fragment ("#" at the end) is dropped, as url.Parse does.
	rest, fragment, _ := strings.Cut(rest, "#")
	rest, _, hasQuery := strings.Cut(rest, "?")
	rest, ok := strings.CutPrefix(rest, "//")
	if !ok {
		return URL{}, ErrMissingHost
	}
	authority, path := rest, ""
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		authority, path = rest[:i], rest[i:]
	}
	if strings.IndexByte(authority, '@') >= 0 {
		return URL{}, ErrUserinfo
	}
	if hasQuery || fragment != "" {
		return URL{}, ErrQueryOrFragment
	}

	host, port, err := splitHostPort(authority)
	if err != nil {
		return URL{}, err
	}
	u.Host = strings.ToLower(host)
	if u.Host == "" {
		return URL{}, ErrMissingHost
	}
	if port != "" {
		n, err := strconv.ParseUint(port, 10, 16)
		if err != nil || n == 0 {
			return URL{}, ErrInvalidPort
		}
		u.Port = uint16(n)
	}
	if addr, err := netip.ParseAddr(u.Host); err == nil {
		u.Addr = addr
	}

	if path, err = escapePath(path); err != nil {
		return URL{}, err
	}
	u.PathPrefix = strings.TrimRight(path, "/")

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

// splitHostPort splits an authority into host and port. A present but empty
// port (trailing ':') is an error. Bracketed hosts must be IPv6 literals; an
// escaped zone ("%25") is unescaped.
func splitHostPort(authority string) (host, port string, err error) {
	hostPort := ""
	if strings.HasPrefix(authority, "[") {
		end := strings.IndexByte(authority, ']')
		if end < 0 {
			return "", "", ErrInvalidHost
		}
		host, hostPort = strings.Replace(authority[1:end], "%25", "%", 1), authority[end+1:]
		if hostPort != "" && hostPort[0] != ':' {
			return "", "", ErrInvalidHost
		}
		if addr, err := netip.ParseAddr(host); err != nil || addr.Is4() {
			return "", "", ErrInvalidHost
		}
	} else {
		host = authority
		if i := strings.IndexByte(authority, ':'); i >= 0 {
			host, hostPort = authority[:i], authority[i:]
		}
		for i := 0; i < len(host); i++ {
			if !validHostByte(host[i]) {
				return "", "", ErrInvalidHost
			}
		}
	}
	if hostPort == "" {
		return host, "", nil
	}
	port = hostPort[1:]
	if port == "" {
		return "", "", ErrInvalidPort
	}
	for i := 0; i < len(port); i++ {
		if port[i] < '0' || port[i] > '9' {
			return "", "", ErrInvalidPort
		}
	}
	return host, port, nil
}

func validHostByte(c byte) bool {
	return c >= 0x80 || isUnreserved(c) || strings.IndexByte("!$&'()*+,;=", c) >= 0
}

func isUnreserved(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' ||
		c == '-' || c == '.' || c == '_' || c == '~'
}

// escapePath keeps valid percent-escapes and the characters url.URL.EscapedPath
// leaves alone, and percent-encodes everything else.
func escapePath(p string) (string, error) {
	const hex = "0123456789ABCDEF"
	n := 0
	for i := 0; i < len(p); i++ {
		c := p[i]
		switch {
		case c == '%':
			if i+2 >= len(p) || !isHex(p[i+1]) || !isHex(p[i+2]) {
				return "", ErrInvalidPath
			}
			i += 2
		case !validPathByte(c):
			n++
		}
	}
	if n == 0 {
		return p, nil
	}
	b := make([]byte, 0, len(p)+2*n)
	for i := 0; i < len(p); i++ {
		if c := p[i]; c == '%' || validPathByte(c) {
			b = append(b, c)
		} else {
			b = append(b, '%', hex[c>>4], hex[c&0xf])
		}
	}
	return string(b), nil
}

func validPathByte(c byte) bool {
	return c < 0x80 && (isUnreserved(c) || strings.IndexByte("/!$&'()*+,;=:@[]", c) >= 0)
}

func isHex(c byte) bool {
	return '0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F'
}

// IsIPLiteral reports whether Host is an IP address rather than a name.
func (u URL) IsIPLiteral() bool { return u.Addr.IsValid() }

// IsDefaultPort reports whether Port is the default port for Scheme.
func (u URL) IsDefaultPort() bool {
	switch u.Scheme {
	case SchemeHTTP:
		return u.Port == DefaultPortHTTP
	case SchemeHTTPS:
		return u.Port == DefaultPortHTTPS
	}
	return false
}

// HostHeader returns the value for the HTTP Host header: the host, with
// ":port" appended only when the port is not the scheme default.
func (u URL) HostHeader() string { return u.hostHeader }

// AddrPort returns the dial address when Host is an IP literal.
func (u URL) AddrPort() (netip.AddrPort, bool) {
	if !u.IsIPLiteral() {
		return netip.AddrPort{}, false
	}
	return netip.AddrPortFrom(u.Addr, u.Port), true
}

// RequestTarget joins PathPrefix with path, which must start with "/".
// It allocates; build request targets once and reuse them, or use
// [URL.AppendRequestTarget].
func (u URL) RequestTarget(path string) string { return u.PathPrefix + path }

// AppendRequestTarget appends PathPrefix and path to dst. It does not allocate
// when dst has enough capacity.
func (u URL) AppendRequestTarget(dst []byte, path string) []byte {
	return append(append(dst, u.PathPrefix...), path...)
}

// String returns the normalized URL.
func (u URL) String() string { return u.Scheme + "://" + u.hostHeader + u.PathPrefix }
