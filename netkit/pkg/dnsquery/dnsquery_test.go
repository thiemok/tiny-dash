package dnsquery

import (
	"errors"
	"net/netip"
	"testing"
	"time"

	"github.com/soypat/lneto/dns"
)

const host = "api.example.net"

var addr = netip.MustParseAddr("192.0.2.20")

const (
	flagResponse = dns.HeaderFlags(1 << 15)
	flagsOK      = flagResponse | 1<<8 | 1<<7
)

func mustName(t testing.TB, s string) dns.Name {
	t.Helper()
	var n dns.Name
	if err := n.Parse(s); err != nil {
		t.Fatal(err)
	}
	return n
}

// response encodes a reply to a query for host carrying answers.
func response(t testing.TB, txid uint16, flags dns.HeaderFlags, answers ...dns.Resource) []byte {
	t.Helper()
	var msg dns.Message
	msg.AddQuestions([]dns.Question{{Name: mustName(t, host), Type: dns.TypeA, Class: dns.ClassINET}})
	msg.Answers = answers
	b, err := msg.AppendTo(nil, txid, flags)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func aRecord(t testing.TB, name string, a netip.Addr, ttl uint32) dns.Resource {
	return dns.NewResource(mustName(t, name), dns.TypeA, dns.ClassINET, ttl, a.AsSlice())
}

func newQuery(t testing.TB) *Query {
	t.Helper()
	q, err := NewA(host, 4, 512)
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func TestRequest(t *testing.T) {
	q := newQuery(t)
	b, err := q.AppendRequest(nil, 0x1234)
	if err != nil {
		t.Fatal(err)
	}
	var msg dns.Message
	msg.LimitResourceDecoding(1, 0, 0, 1)
	if _, _, err := msg.Decode(b); err != nil {
		t.Fatal(err)
	}
	frm, _ := dns.NewFrame(b)
	if frm.TxID() != 0x1234 || frm.Flags().IsResponse() {
		t.Errorf("txid %#x, flags %v", frm.TxID(), frm.Flags())
	}
	if len(msg.Questions) != 1 {
		t.Fatalf("%d questions", len(msg.Questions))
	}
	qn := msg.Questions[0]
	if !qn.Name.EqualString(host) || qn.Type != dns.TypeA || qn.Class != dns.ClassINET {
		t.Errorf("question %v %v %v", qn.Name.String(), qn.Type, qn.Class)
	}
	if len(msg.Additionals) != 1 || msg.Additionals[0].Header().Type != dns.TypeOPT {
		t.Errorf("want one EDNS0 OPT record, got %d additionals", len(msg.Additionals))
	}
}

func TestRequestWithoutEDNS(t *testing.T) {
	q, err := NewA(host, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	b, err := q.AppendRequest(nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	if frm, _ := dns.NewFrame(b); frm.ARCount() != 0 {
		t.Errorf("ARCount = %d, want 0", frm.ARCount())
	}
}

func TestParseResponse(t *testing.T) {
	other := netip.MustParseAddr("10.0.0.1")
	tests := []struct {
		name    string
		msg     func(t *testing.T) []byte
		want    netip.Addr
		wantTTL time.Duration
		wantErr error
	}{
		{
			name: "A record",
			msg: func(t *testing.T) []byte {
				return response(t, 7, flagsOK, aRecord(t, host, addr, 3600))
			},
			want: addr, wantTTL: time.Hour,
		},
		{
			name: "skips records for other names",
			msg: func(t *testing.T) []byte {
				return response(t, 7, flagsOK, aRecord(t, "other.example.net", other, 60), aRecord(t, host, addr, 0))
			},
			want: addr,
		},
		{
			name: "CNAME chain has no A record for the host",
			msg: func(t *testing.T) []byte {
				target := mustName(t, "target.example.net")
				var cname []byte
				cname, _ = target.AppendTo(cname)
				return response(t, 7, flagsOK,
					dns.NewResource(mustName(t, host), dns.TypeCNAME, dns.ClassINET, 60, cname),
					aRecord(t, "target.example.net", addr, 60))
			},
			wantErr: ErrNoAddress,
		},
		{
			name:    "no answers",
			msg:     func(t *testing.T) []byte { return response(t, 7, flagsOK) },
			wantErr: ErrNoAddress,
		},
		{
			name: "wrong transaction ID",
			msg: func(t *testing.T) []byte {
				return response(t, 8, flagsOK, aRecord(t, host, addr, 60))
			},
			wantErr: ErrNotResponse,
		},
		{
			name: "query instead of response",
			msg: func(t *testing.T) []byte {
				return response(t, 7, flagsOK&^flagResponse, aRecord(t, host, addr, 60))
			},
			wantErr: ErrNotResponse,
		},
		{
			name:    "NXDOMAIN",
			msg:     func(t *testing.T) []byte { return response(t, 7, flagsOK|dns.HeaderFlags(dns.RCodeNameError)) },
			wantErr: dns.RCodeNameError,
		},
		{
			name: "more answers than decoded",
			msg: func(t *testing.T) []byte {
				var ans []dns.Resource
				for range 6 {
					ans = append(ans, aRecord(t, "other.example.net", other, 60))
				}
				return response(t, 7, flagsOK, append(ans, aRecord(t, host, addr, 60))...)
			},
			wantErr: ErrNoAddress,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ttl, err := newQuery(t).ParseResponse(tt.msg(t), 7)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if got != tt.want || ttl != tt.wantTTL {
				t.Errorf("got %v ttl %v, want %v ttl %v", got, ttl, tt.want, tt.wantTTL)
			}
		})
	}
}

func TestParseTruncated(t *testing.T) {
	msg := response(t, 7, flagsOK, aRecord(t, host, addr, 60))
	q := newQuery(t)
	for n := range len(msg) {
		if a, _, err := q.ParseResponse(msg[:n], 7); err == nil {
			t.Fatalf("ParseResponse(msg[:%d]) = %v, want an error", n, a)
		}
	}
}

func TestRepeatedLookupsDoNotAllocate(t *testing.T) {
	q := newQuery(t)
	resp := response(t, 7, flagsOK, aRecord(t, "other.example.net", addr, 60), aRecord(t, host, addr, 60))
	buf := make([]byte, 0, 512)
	if _, _, err := q.ParseResponse(resp, 7); err != nil {
		t.Fatal(err)
	}
	allocs := testing.AllocsPerRun(100, func() {
		if _, err := q.AppendRequest(buf[:0], 7); err != nil {
			t.Fatal(err)
		}
		if _, _, err := q.ParseResponse(resp, 7); err != nil {
			t.Fatal(err)
		}
		q.ParseResponse(resp, 8)
	})
	if allocs != 0 {
		t.Errorf("lookup allocated %v times per run", allocs)
	}
}

func TestFirstResponseDoesNotAllocate(t *testing.T) {
	const runs = 10
	resp := response(t, 7, flagsOK, aRecord(t, host, addr, 60), aRecord(t, host, addr, 60))
	// AllocsPerRun calls f once more as warm-up; each call gets a fresh Query.
	queries := make([]*Query, runs+1)
	for i := range queries {
		queries[i] = newQuery(t)
	}
	i := 0
	allocs := testing.AllocsPerRun(runs, func() {
		if _, _, err := queries[i].ParseResponse(resp, 7); err != nil {
			t.Fatal(err)
		}
		i++
	})
	if allocs != 0 {
		t.Errorf("first response allocated %v times per Query", allocs)
	}
}
