package main

import (
	"errors"
	"net/netip"
	"time"

	"github.com/soypat/lneto/dns"
	"github.com/soypat/lneto/udp"
	"github.com/thiemok/tiny-dash/netkit/pkg/dnsquery"
)

// dnsBufSize bounds a DNS response and is advertised through EDNS0; an A
// answer for one host is ~100 B.
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
	rxBuf []byte
	txBuf []byte
	buf   []byte
}

func newDNSLookup(host string, link *wifiLink) (*dnsLookup, error) {
	query, err := dnsquery.NewA(host, 2, dnsBufSize)
	if err != nil {
		return nil, err
	}
	d := &dnsLookup{
		link:  link,
		query: query,
		rxBuf: make([]byte, dnsBufSize),
		buf:   make([]byte, dnsBufSize),
	}
	req, err := query.AppendRequest(d.buf[:0], 0)
	if err != nil {
		return nil, err
	}
	// The query is the only datagram ever sent.
	d.txBuf = make([]byte, len(req))
	return d, nil
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
	conn := &d.conn
	err := conn.Configure(udp.ConnConfig{
		RxBuf:       d.rxBuf,
		TxBuf:       d.txBuf,
		RxQueueSize: 1,
		TxQueueSize: 1,
		RWBackoff:   pollBackoff,
	})
	if err != nil {
		return netip.Addr{}, 0, err
	}
	lstack := &d.link.stack
	lport := uint16(lstack.Prand32()>>17) + 1024
	err = lstack.DialUDP(conn, lport, netip.AddrPortFrom(server, dns.ServerPort))
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
		n, err := d.conn.Read(d.buf[:cap(d.buf)])
		if err != nil {
			return netip.Addr{}, err
		}
		addr, _, err := d.query.ParseResponse(d.buf[:n], txid)
		if err != dnsquery.ErrNotResponse {
			return addr, err
		}
	}
}
