package main

import (
	"errors"
	"net/netip"
	"time"

	"github.com/soypat/lneto/dns"
	"github.com/soypat/lneto/udp"
	"github.com/soypat/lneto/x/xnet"
	"github.com/thiemok/tiny-dash/netkit/pkg/bufshare"
	"github.com/thiemok/tiny-dash/netkit/pkg/dnsquery"
)

// dnsBufSize bounds a DNS response and is advertised through EDNS0, whose
// smallest valid size it is; an A answer for one host is ~100 B.
const dnsBufSize = 512

var errNoDNSServer = errors.New("no DNS server: DHCP offered none and DNS_SERVER is unset")

// udpConn is the one UDP conn that DNS lookups and NTP syncs take turns on.
// They all borrow the fetch conn's idle receive buffer, so no two of them can
// be open at once anyway, and sharing the conn saves a udp.Conn and its
// datagram queues per user. Each user configures it with its own rings out of
// the borrowed buffer before every dial.
type udpConn struct {
	conn  udp.Conn
	share *bufshare.Buffer
	user  bufshare.User
}

// udpQueueSize is every user's datagram queue length: the conn keeps the
// queues it grew first, so a shorter one would not take effect anyway.
const udpQueueSize = 2

func newUDPConn(share *bufshare.Buffer) (*udpConn, error) {
	u := &udpConn{share: share}
	u.user = bufshare.User{Name: "udp conn", Idle: u.idle}
	// Opening the conn once grows its datagram queues and remote address
	// now rather than in the first lookup. It is never registered with the
	// stack, so the shared buffer stays untouched.
	borrowed := share.Bytes()
	err := u.conn.Configure(udp.ConnConfig{
		RxBuf:       borrowed[:dnsBufSize],
		TxBuf:       borrowed[dnsBufSize : 2*dnsBufSize],
		RxQueueSize: udpQueueSize,
		TxQueueSize: udpQueueSize,
		RWBackoff:   pollBackoff,
	})
	if err != nil {
		return nil, err
	}
	err = u.conn.Open(1024, netip.AddrPortFrom(netip.IPv4Unspecified(), dns.ServerPort))
	u.conn.Abort()
	return u, err
}

// acquire takes the shared buffer over and configures the conn with cfg.
func (u *udpConn) acquire(cfg udp.ConnConfig) error {
	if err := u.share.Acquire(&u.user); err != nil {
		return err
	}
	return u.conn.Configure(cfg)
}

// dial opens the conn from a random local port; the caller aborts it once
// done, and dial aborts it itself on failure.
func (u *udpConn) dial(lstack *xnet.StackAsync, raddr netip.AddrPort) error {
	lport := uint16(lstack.Prand32()>>17) + 1024
	err := lstack.DialUDP(&u.conn, lport, raddr)
	if err != nil {
		u.conn.Abort()
	}
	return err
}

// idle reports whether the conn is aborted: Abort zeroes the local port, so
// the conn rejects every datagram, and bumps the conn ID, so the stack drops
// its port registration. Every user aborts the conn once done.
func (u *udpConn) idle() bool { return u.conn.LocalPort() == 0 }

// dnsLookup resolves a host over the shared UDP conn with a query built once,
// so a re-lookup only allocates what lneto's DialUDP does (3 mallocs / 20 B:
// the port registration's two bound methods and the remote address).
// lneto's StackRetrying.DoLookupIP rebuilds the name, the question and its
// UDP port registration on every call: 14 mallocs / 224 B.
type dnsLookup struct {
	link  *wifiLink
	query *dnsquery.Query
	udp   *udpConn
	cfg   udp.ConnConfig
	buf   []byte
}

// newDNSLookup carves the conn's rings and the message buffer out of the
// buffer u borrows.
func newDNSLookup(host string, link *wifiLink, u *udpConn) (*dnsLookup, error) {
	query, err := dnsquery.NewA(host, 2, dnsBufSize)
	if err != nil {
		return nil, err
	}
	borrowed := u.share.Bytes()
	req, err := query.AppendRequest(borrowed[:0], 0)
	if err != nil {
		return nil, err
	}
	// The query is the only datagram ever sent.
	txLen := len(req)
	if len(borrowed) < 2*dnsBufSize+txLen {
		return nil, errors.New("borrowed DNS buffer too small")
	}
	return &dnsLookup{
		link:  link,
		query: query,
		udp:   u,
		cfg: udp.ConnConfig{
			RxBuf:       borrowed[:dnsBufSize:dnsBufSize],
			TxBuf:       borrowed[dnsBufSize : dnsBufSize+txLen : dnsBufSize+txLen],
			RxQueueSize: udpQueueSize,
			TxQueueSize: udpQueueSize,
			RWBackoff:   pollBackoff,
		},
		buf: borrowed[dnsBufSize+txLen : 2*dnsBufSize+txLen : 2*dnsBufSize+txLen],
	}, nil
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
	if err := d.udp.acquire(d.cfg); err != nil {
		return netip.Addr{}, 0, err
	}
	lstack := &d.link.stack
	if err := d.udp.dial(lstack, netip.AddrPortFrom(server, dns.ServerPort)); err != nil {
		return netip.Addr{}, 0, err
	}
	var addr netip.Addr
	var err error
	for range dnsRetries {
		// A fresh ID per attempt tells a late answer to an earlier one apart.
		if addr, err = d.exchange(uint16(lstack.Prand32())); err == nil {
			break
		}
	}
	d.udp.conn.Abort()
	return addr, 0, err
}

// exchange sends the query and waits dnsTimeout for its answer, skipping
// datagrams that answer something else.
func (d *dnsLookup) exchange(txid uint16) (netip.Addr, error) {
	req, err := d.query.AppendRequest(d.buf[:0], txid)
	if err != nil {
		return netip.Addr{}, err
	}
	if _, err = d.udp.conn.Write(req); err != nil {
		return netip.Addr{}, err
	}
	d.udp.conn.SetReadDeadline(time.Now().Add(dnsTimeout))
	for {
		n, err := d.udp.conn.Read(d.buf)
		if err != nil {
			return netip.Addr{}, err
		}
		addr, _, err := d.query.ParseResponse(d.buf[:n], txid)
		if err != dnsquery.ErrNotResponse {
			return addr, err
		}
	}
}
