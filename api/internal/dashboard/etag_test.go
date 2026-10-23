package dashboard

import (
	"fmt"
	"net/http"
	"net/http/httptest"
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

	unshown := base
	unshown.Departures = &departuresView{Departures: []departureRow{{Line: "S8", Time: "14:32", Alerts: true}}}
	if hash(unshown, "v1") != h0 {
		t.Error("hash changed with departure alerts, which are not shown")
	}

	weather := func(temp float64) templateData {
		d := base
		d.Weather = &weatherView{Temperature: temp, Icon: "wi-sunny", HourlyForecast: []hourlyForecastView{{TimeStr: "15", Temperature: temp}}}
		return d
	}
	if hash(weather(12.3), "v1") != hash(weather(12.4), "v1") {
		t.Error("hash changed with a temperature that is shown as the same whole degree")
	}
	if hash(weather(12.4), "v1") == hash(weather(12.6), "v1") {
		t.Error("hash unchanged although the shown temperature changed")
	}
	d := weather(12.6)
	_ = hash(d, "v1")
	if d.Weather.Temperature != 12.6 || d.Weather.HourlyForecast[0].Temperature != 12.6 {
		t.Error("contentHash rounded the page's own temperatures")
	}
}

func TestShownTempMatchesTemplate(t *testing.T) {
	for _, temp := range []float64{-0.5, -0.4, 0.5, 1.5, 2.5, 12.49, 12.5, 12.51, -3.5} {
		if got, want := fmt.Sprintf("%.0f", shownTemp(temp)), fmt.Sprintf("%.0f", temp); got != want {
			t.Errorf("shownTemp(%v) prints %s, the template prints %s", temp, got, want)
		}
		if shownTemp(temp) != shownTemp(shownTemp(temp)) {
			t.Errorf("shownTemp(%v) is not a whole number", temp)
		}
	}
}

func TestWriteNotModified(t *testing.T) {
	meta := map[string]string{etagMeta: "abc"}
	etag := imageETag("abc", 4)
	tests := []struct {
		name        string
		meta        map[string]string
		ifNoneMatch string
		want        bool
		wantETag    string
	}{
		{"match", meta, etag, true, etag},
		{"other tag", meta, imageETag("abd", 4), false, etag},
		{"placeholder", meta, `"----------------"`, false, etag},
		{"weak tag", meta, "W/" + etag, false, etag},
		{"no If-None-Match", meta, "", false, etag},
		{"page without hash", map[string]string{}, `""`, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/dashboard/image", nil)
			if tt.ifNoneMatch != "" {
				r.Header.Set("If-None-Match", tt.ifNoneMatch)
			}
			w := httptest.NewRecorder()
			w.Header().Set(refreshHintHeader, "120")
			if got := writeNotModified(w, r, tt.meta, 4); got != tt.want {
				t.Fatalf("writeNotModified = %v, want %v", got, tt.want)
			}
			res := w.Result()
			if got := res.Header.Get("ETag"); got != tt.wantETag {
				t.Errorf("ETag = %q, want %q", got, tt.wantETag)
			}
			if tt.want {
				if res.StatusCode != http.StatusNotModified || w.Body.Len() != 0 {
					t.Errorf("got %d with %d body bytes, want 304 without a body", res.StatusCode, w.Body.Len())
				}
				if res.Header.Get(refreshHintHeader) != "120" {
					t.Error("304 lost the X-Refresh-After set before it")
				}
			} else if res.StatusCode != http.StatusOK {
				t.Error("wrote a status although the image is to be sent")
			}
		})
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
