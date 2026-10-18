package main

import (
	"errors"
	"net/netip"
	"time"

	"github.com/soypat/lneto/dns"
	"github.com/soypat/lneto/udp"
	"github.com/thiemok/tiny-dash/netkit/pkg/bufshare"
	"github.com/thiemok/tiny-dash/netkit/pkg/dnsquery"
)

// dnsBufSize bounds a DNS response and is advertised through EDNS0, whose
// smallest valid size it is; an A answer for one host is ~100 B.
const dnsBufSize = 512

var errNoDNSServer = errors.New("no DNS server: DHCP offered none and DNS_SERVER is unset")

// dnsLookup resolves the API host over a reused UDP conn with a query built
// once, so a re-lookup only allocates what lneto's DialUDP does (3 mallocs /
// 20 B: the port registration's two bound methods and the remote address).
// lneto's StackRetrying.DoLookupIP rebuilds the name, the question and its
// UDP port registration on every call: 14 mallocs / 224 B.
type dnsLookup struct {
	link  *wifiLink
	query *dnsquery.Query
	conn  udp.Conn
	cfg   udp.ConnConfig
	buf   []byte
	share *bufshare.Buffer
	user  bufshare.User
}

// newDNSLookup carves the conn's rings and the message buffer out of share,
// which it acquires for every lookup.
func newDNSLookup(host string, link *wifiLink, share *bufshare.Buffer) (*dnsLookup, error) {
	query, err := dnsquery.NewA(host, 2, dnsBufSize)
	if err != nil {
		return nil, err
	}
	borrowed := share.Bytes()
	req, err := query.AppendRequest(borrowed[:0], 0)
	if err != nil {
		return nil, err
	}
	// The query is the only datagram ever sent.
	txLen := len(req)
	if len(borrowed) < 2*dnsBufSize+txLen {
		return nil, errors.New("borrowed DNS buffer too small")
	}
	d := &dnsLookup{
		link:  link,
		query: query,
		cfg: udp.ConnConfig{
			RxBuf:       borrowed[:dnsBufSize:dnsBufSize],
			TxBuf:       borrowed[dnsBufSize : dnsBufSize+txLen : dnsBufSize+txLen],
			RxQueueSize: 1,
			TxQueueSize: 1,
			RWBackoff:   pollBackoff,
		},
		buf:   borrowed[dnsBufSize+txLen : 2*dnsBufSize+txLen : 2*dnsBufSize+txLen],
		share: share,
	}
	d.user = bufshare.User{Name: "dns conn", Idle: d.connIdle}
	// Opening the conn once grows its datagram queues and remote address
	// now rather than in the first lookup. It is never registered with the
	// stack, so the shared buffer stays untouched.
	if err = d.conn.Configure(d.cfg); err != nil {
		return nil, err
	}
	err = d.conn.Open(1024, netip.AddrPortFrom(netip.IPv4Unspecified(), dns.ServerPort))
	d.conn.Abort()
	return d, err
}

// lookup queries the DNS server from the current lease, sending the query up
// to dnsRetries times. It drops the record's TTL, so the resolver caches the
// address for dnsMaxAge as it did when lneto's DoLookupIP, which hides the
// TTL, ran the query.
func (d *dnsLookup) lookup(string) (netip.Addr, time.Duration, error) {
	server := d.link.dnsServer
	if !server.IsValid() {
		return netip.Addr{}, 0, errNoDNSServer
	}
	if err := d.share.Acquire(&d.user); err != nil {
		return netip.Addr{}, 0, err
	}
	conn := &d.conn
	if err := conn.Configure(d.cfg); err != nil {
		return netip.Addr{}, 0, err
	}
	lstack := &d.link.stack
	lport := uint16(lstack.Prand32()>>17) + 1024
	err := lstack.DialUDP(conn, lport, netip.AddrPortFrom(server, dns.ServerPort))
	if err != nil {
		conn.Abort()
		return netip.Addr{}, 0, err
	}
	var addr netip.Addr
	for range dnsRetries {
		// A fresh ID per attempt tells a late answer to an earlier one apart.
		if addr, err = d.exchange(uint16(lstack.Prand32())); err == nil {
			break
		}
	}
	conn.Abort()
	return addr, 0, err
}

// connIdle reports whether the conn is aborted: Abort zeroes the local port,
// so the conn rejects every datagram, and bumps the conn ID, so the stack
// drops its port registration. lookup aborts the conn on every return.
func (d *dnsLookup) connIdle() bool { return d.conn.LocalPort() == 0 }

// exchange sends the query and waits dnsTimeout for its answer, skipping
// datagrams that answer something else.
func (d *dnsLookup) exchange(txid uint16) (netip.Addr, error) {
	req, err := d.query.AppendRequest(d.buf[:0], txid)
	if err != nil {
		return netip.Addr{}, err
	}
	if _, err = d.conn.Write(req); err != nil {
		return netip.Addr{}, err
	}
	d.conn.SetReadDeadline(time.Now().Add(dnsTimeout))
	for {
		n, err := d.conn.Read(d.buf)
		if err != nil {
			return netip.Addr{}, err
		}
		addr, _, err := d.query.ParseResponse(d.buf[:n], txid)
		if err != dnsquery.ErrNotResponse {
			return addr, err
		}
	}
}
