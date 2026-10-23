package dashboard

import (
	"regexp"
	"testing"
	"testing/fstest"
)

func TestContentHash(t *testing.T) {
	base := templateData{
		Width: 800, Height: 480, Weekday: "FRIDAY", Day: "23 Oct",
		Departures: &departuresView{Departures: []departureRow{{Line: "S8", Time: "14:32"}}},
	}
	hash := func(d templateData, version string) string {
		t.Helper()
		h, err := d.contentHash(version)
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	h0 := hash(base, "v1")

	same := base
	same.NextRefresh = "2026-10-23T14:35:00+02:00"
	same.ETag = "stale"
	if hash(same, "v1") != h0 {
		t.Error("hash changed with the next refresh time, which is not shown")
	}

	moved := base
	moved.Departures = &departuresView{Departures: []departureRow{{Line: "S8", Time: "14:32", Delay: 2}}}
	if hash(moved, "v1") == h0 {
		t.Error("hash unchanged although a delay appeared")
	}
	if hash(base, "v2") == h0 {
		t.Error("hash unchanged although the content version changed")
	}
}

func TestContentVersionFollowsTemplates(t *testing.T) {
	fsys := fstest.MapFS{"templates/dashboard.html": {Data: []byte("<p>a</p>")}}
	v1, err := contentVersion(fsys)
	if err != nil {
		t.Fatal(err)
	}
	fsys["templates/dashboard.html"] = &fstest.MapFile{Data: []byte("<p>b</p>")}
	v2, err := contentVersion(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if v1 == v2 {
		t.Fatal("content version unchanged after a template edit")
	}
}

func TestImageETag(t *testing.T) {
	quoted16 := regexp.MustCompile(`^"[0-9a-f]{16}"$`)
	e4 := imageETag("abc", 4)
	if !quoted16.MatchString(e4) {
		t.Fatalf("imageETag = %q, want a quoted 16-digit hex string", e4)
	}
	if imageETag("abc", 4) != e4 {
		t.Error("imageETag is not deterministic")
	}
	if imageETag("abc", 3) == e4 || imageETag("abd", 4) == e4 {
		t.Error("imageETag ignores the page hash or the color depth")
	}
	if imageETag("", 4) != "" {
		t.Error("imageETag without a page hash is not empty")
	}
}
