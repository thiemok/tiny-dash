package apiurl

import (
	"strings"
	"testing"
)

// FuzzParseMatchesNetURL checks that every URL Parse accepts is accepted by
// url.Parse with the same result, and that the parts that end up in the
// request line and Host header cannot break out of them. Parse may reject
// URLs that url.Parse accepts.
func FuzzParseMatchesNetURL(f *testing.F) {
	for _, in := range netURLCorpus {
		f.Add(in)
	}
	for _, in := range stricterThanNetURL {
		f.Add(in)
	}
	f.Fuzz(func(t *testing.T, in string) {
		got, err := Parse(in)
		if err != nil {
			return
		}
		want, wantErr := parseNetURL(in)
		if wantErr != nil {
			t.Fatalf("Parse(%q) = %+v, net/url rejects it: %v", in, got, wantErr)
		}
		if got != want {
			t.Fatalf("Parse(%q) = %+v, net/url = %+v", in, got, want)
		}
		if i := strings.IndexFunc(got.HostHeader(), unsafeInHeader); i >= 0 {
			t.Fatalf("Parse(%q).HostHeader() = %q has unsafe byte at %d", in, got.HostHeader(), i)
		}
		if i := strings.IndexFunc(got.PathPrefix, unsafeInHeader); i >= 0 {
			t.Fatalf("Parse(%q).PathPrefix = %q has unsafe byte at %d", in, got.PathPrefix, i)
		}
		if strings.ContainsAny(got.PathPrefix, "?#") || got.PathPrefix != "" && got.PathPrefix[0] != '/' {
			t.Fatalf("Parse(%q).PathPrefix = %q is not a bare path", in, got.PathPrefix)
		}
		if got.Port == 0 {
			t.Fatalf("Parse(%q).Port = 0", in)
		}
	})
}

func unsafeInHeader(r rune) bool { return r <= ' ' || r == 0x7f }
