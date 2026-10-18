// Package apiurl parses the base URL of an HTTP API, e.g.
// "https://dash.example.net/prefix" or "http://192.168.1.10:8080".
//
// Parsing is meant to run once at startup; the accessors on URL do not
// allocate except where noted, so the result can be used in allocation-free
// request loops.
package apiurl

import (
	"errors"
	"net/netip"
	"net/url"
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
	ErrInvalidPort       = errors.New("apiurl: invalid port")
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
	pu, err := url.Parse(s)
	if err != nil {
		if !strings.Contains(s, "://") {
			return URL{}, ErrUnsupportedScheme
		}
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
// It allocates; build request targets once and reuse them.
func (u URL) RequestTarget(path string) string { return u.PathPrefix + path }

// String returns the normalized URL.
func (u URL) String() string { return u.Scheme + "://" + u.hostHeader + u.PathPrefix }
