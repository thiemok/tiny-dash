// Package dnsquery builds DNS A queries for one host name and reads the
// address from their responses, using lneto's dns package for the wire
// format. It does no I/O, so any UDP transport can carry the messages.
//
// The query is encoded once and the response is decoded into a message that
// keeps its buffers, so after the first response a lookup does not allocate.
package dnsquery

import (
	"errors"
	"net/netip"
	"time"

	"github.com/soypat/lneto/dns"
)

var (
	ErrNotResponse = errors.New("dnsquery: not a response to this query")
	ErrNoAddress   = errors.New("dnsquery: no IPv4 address in DNS answer")
)

// Query is an A record query for one host name. It is not safe for
// concurrent use.
type Query struct {
	host string
	name dns.Name
	req  dns.Message
	resp dns.Message
}

// NewA returns a Query for host's A record. Only the first maxAnswers answer
// records of a response are decoded. A non-zero udpSize advertises that
// receive buffer size through EDNS0.
func NewA(host string, maxAnswers int, udpSize uint16) (*Query, error) {
	var name dns.Name
	if err := name.Parse(host); err != nil {
		return nil, err
	}
	q := &Query{host: host, name: name}
	q.req.AddQuestions([]dns.Question{{Name: name, Type: dns.TypeA, Class: dns.ClassINET}})
	if udpSize > 0 {
		// An OPT record carries the UDP payload size in its class, RFC 6891 6.1.2.
		opt := dns.NewResource(dns.MustNewName("."), dns.TypeOPT, dns.Class(udpSize), 0, nil)
		q.req.AddAdditionals([]dns.Resource{opt})
	}
	q.resp.LimitResourceDecoding(1, uint16(maxAnswers), 0, 0)
	q.growResponse()
	return q, nil
}

// growResponse sizes the decode buffers for maxAnswers A records about the
// host, so that the first response does not allocate either. Decode keeps
// the buffers when it resets the records.
func (q *Query) growResponse() {
	var a [4]byte
	r := &q.resp
	r.Questions = r.Questions[:1]
	r.Questions[0].Name.CopyFrom(q.name)
	r.Answers = r.Answers[:cap(r.Answers)]
	for i := range r.Answers {
		r.Answers[i].SetA(q.name, dns.ClassINET, 0, a[:])
	}
	r.Reset()
}

// Host returns the host name the Query asks for.
func (q *Query) Host() string { return q.host }

// AppendRequest appends the query message with transaction ID txid to dst.
func (q *Query) AppendRequest(dst []byte, txid uint16) ([]byte, error) {
	return q.req.AppendTo(dst, txid, dns.NewClientHeaderFlags(dns.OpCodeQuery, true))
}

// ParseResponse returns the first A record for the host in msg and its TTL.
// It returns ErrNotResponse for a message that does not answer the query
// with transaction ID txid, such as a late answer to an earlier attempt, so
// that the caller can keep waiting for the right one.
func (q *Query) ParseResponse(msg []byte, txid uint16) (netip.Addr, time.Duration, error) {
	frm, err := dns.NewFrame(msg)
	if err != nil {
		return netip.Addr{}, 0, err
	}
	flags := frm.Flags()
	if frm.TxID() != txid || !flags.IsResponse() {
		return netip.Addr{}, 0, ErrNotResponse
	}
	if rcode := flags.ResponseCode(); rcode != 0 {
		return netip.Addr{}, 0, rcode
	}
	// Records beyond the limits set in NewA are skipped, not an error.
	if _, incompleteButOK, err := q.resp.Decode(msg); err != nil && !incompleteButOK {
		return netip.Addr{}, 0, err
	}
	// Names are compared in wire format: under TinyGo, dns.Name.EqualString
	// allocates a string per label it compares.
	for i := range q.resp.Answers {
		ans := &q.resp.Answers[i]
		hdr := ans.Header()
		data := ans.RawData()
		if hdr.Type == dns.TypeA && hdr.Class == dns.ClassINET && len(data) == 4 && dns.NamesEqual(hdr.Name, q.name) {
			return netip.AddrFrom4([4]byte(data)), time.Duration(hdr.TTL) * time.Second, nil
		}
	}
	return netip.Addr{}, 0, ErrNoAddress
}
