package dashboard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/thiemok/tiny-dash/api/internal/config"
	"github.com/thiemok/tiny-dash/api/internal/homeassistant"
)

func berlin(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("no tzdata:", err)
	}
	return loc
}

var refreshCfg = config.RefreshConfig{
	Min:   2 * time.Minute,
	Max:   time.Hour,
	Retry: 2 * time.Minute,
	QuietHours: config.QuietHoursConfig{
		Start: "23:00",
		End:   "06:30",
		Min:   time.Hour,
		Max:   4 * time.Hour,
	},
}

func TestDecideRefresh(t *testing.T) {
	loc := berlin(t)
	at := func(h, m int) time.Time { return time.Date(2026, 10, 23, h, m, 0, 0, loc) }
	tests := []struct {
		name       string
		now        time.Time
		cs         changes
		wantAt     time.Time
		wantSource string
		wantQuiet  bool
	}{
		{"earliest change wins", at(14, 2), changes{{at(14, 5), sourceClock}, {at(14, 30), sourceCalendar}}, at(14, 5), sourceClock, false},
		{"raised to min", at(14, 4), changes{{at(14, 5), sourceDepartures}}, at(14, 6), sourceMin, false},
		{"capped at max", at(14, 0), changes{{at(18, 0), sourceCalendar}}, at(15, 0), sourceMax, false},
		{"no changes waits max", at(14, 0), nil, at(15, 0), sourceMax, false},
		{"quiet hours use their bounds", at(23, 10), changes{{at(23, 15), sourceDepartures}}, at(0, 10).AddDate(0, 0, 1), sourceMin, true},
		{"quiet hours cap at their max", at(23, 10), nil, at(3, 10).AddDate(0, 0, 1), sourceMax, true},
		{"quiet hours end on time", at(5, 0), nil, at(6, 30), sourceQuietEnd, true},
		{"min shrinks before quiet end", at(6, 10), changes{{at(6, 15), sourceClock}}, at(6, 30), sourceQuietEnd, true},
		{"after quiet hours", at(6, 30), changes{{at(6, 35), sourceClock}}, at(6, 35), sourceClock, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := decideRefresh(tt.now, refreshCfg, tt.cs)
			if !d.At.Equal(tt.wantAt) || d.Source != tt.wantSource || d.Quiet != tt.wantQuiet {
				t.Fatalf("decideRefresh = %v %s quiet=%v, want %v %s quiet=%v", d.At, d.Source, d.Quiet, tt.wantAt, tt.wantSource, tt.wantQuiet)
			}
		})
	}
}

func TestQuietHours(t *testing.T) {
	loc := berlin(t)
	day := func(d, h, m int) time.Time { return time.Date(2026, 10, d, h, m, 0, 0, loc) }
	tests := []struct {
		name    string
		q       config.QuietHoursConfig
		now     time.Time
		want    bool
		wantEnd time.Time
	}{
		{"disabled", config.QuietHoursConfig{}, day(23, 23, 30), false, time.Time{}},
		{"before window", refreshCfg.QuietHours, day(23, 22, 59), false, time.Time{}},
		{"evening, ends next day", refreshCfg.QuietHours, day(23, 23, 0), true, day(24, 6, 30)},
		{"after midnight, ends same day", refreshCfg.QuietHours, day(24, 2, 0), true, day(24, 6, 30)},
		{"end is exclusive", refreshCfg.QuietHours, day(24, 6, 30), false, time.Time{}},
		{"same-day window", config.QuietHoursConfig{Start: "13:00", End: "15:00"}, day(23, 14, 0), true, day(23, 15, 0)},
		// DST ends on 25 Oct 2026: the night is an hour longer, the end still 06:30.
		{"across DST change", refreshCfg.QuietHours, day(24, 23, 30), true, day(25, 6, 30)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, end := quietHours(tt.now, tt.q)
			if got != tt.want || !end.Equal(tt.wantEnd) {
				t.Fatalf("quietHours = %v %v, want %v %v", got, end, tt.want, tt.wantEnd)
			}
		})
	}
}

func TestNextClockStep(t *testing.T) {
	loc := berlin(t)
	at := func(h, m, s int) time.Time { return time.Date(2026, 10, 23, h, m, s, 0, loc) }
	tests := []struct {
		now  time.Time
		step time.Duration
		want time.Time
	}{
		{at(14, 2, 30), 5 * time.Minute, at(14, 5, 0)},
		{at(14, 5, 0), 5 * time.Minute, at(14, 10, 0)},
		{at(23, 58, 0), 5 * time.Minute, at(0, 0, 0).AddDate(0, 0, 1)},
		{at(2, 40, 0), time.Hour, at(3, 0, 0)},
	}
	for _, tt := range tests {
		if got := nextClockStep(tt.now, tt.step); !got.Equal(tt.want) {
			t.Errorf("nextClockStep(%v, %v) = %v, want %v", tt.now, tt.step, got, tt.want)
		}
	}
}

func TestHourCellsFillFollowsStep(t *testing.T) {
	now := time.Date(2026, 10, 23, 14, 44, 0, 0, time.UTC)
	tests := []struct {
		step time.Duration
		want int
	}{
		{5 * time.Minute, 66}, // 40 of 60 minutes
		{time.Hour, 0},
	}
	for _, tt := range tests {
		if got := hourCells(now, tt.step)[14].Fill; got != tt.want {
			t.Errorf("hourCells(step %v) fill = %d, want %d", tt.step, got, tt.want)
		}
	}
}

func TestDepartureRowsReportChanges(t *testing.T) {
	now := time.Date(2026, 10, 23, 14, 0, 0, 0, time.UTC)
	deps := []homeassistant.Departure{
		{Line: "S8", Planned: now.Add(3 * time.Minute)},
		{Line: "709", Planned: now.Add(10 * time.Minute), Estimated: now.Add(12 * time.Minute)},
	}
	var cs changes
	view := departureRows(deps, now, &cs)
	if !view.Departures[0].Soon || view.Departures[1].Soon {
		t.Fatalf("Soon = %v, %v; want true, false", view.Departures[0].Soon, view.Departures[1].Soon)
	}
	// S8 already turned red, so only its departure counts; 709 turns red at
	// 14:07 (estimated 14:12) and leaves at 14:12.
	want := []time.Time{now.Add(3 * time.Minute), now.Add(7 * time.Minute), now.Add(12 * time.Minute)}
	if len(cs) != len(want) {
		t.Fatalf("changes = %v, want %v", cs, want)
	}
	for i := range want {
		if !cs[i].At.Equal(want[i]) || cs[i].Source != sourceDepartures {
			t.Fatalf("changes = %v, want %v from departures", cs, want)
		}
	}
}

func TestSetRefreshHint(t *testing.T) {
	now := time.Date(2026, 10, 23, 14, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		meta map[string]string
		want string
	}{
		{"rounds up", map[string]string{nextRefreshMeta: now.Add(299*time.Second + time.Millisecond).Format(time.RFC3339Nano)}, "300"},
		{"past is zero", map[string]string{nextRefreshMeta: now.Add(-time.Minute).Format(time.RFC3339)}, "0"},
		{"missing sets nothing", nil, ""},
		{"malformed sets nothing", map[string]string{nextRefreshMeta: "soon"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			setRefreshHint(context.Background(), w, tt.meta, now)
			if got := w.Header().Get(refreshHintHeader); got != tt.want {
				t.Fatalf("%s = %q, want %q", refreshHintHeader, got, tt.want)
			}
		})
	}
}

func TestDeparturesRetryOnlyAfterFailedRead(t *testing.T) {
	tests := []struct {
		name      string
		state     http.HandlerFunc
		wantRetry bool
	}{
		{"sensor unreachable", func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "down", http.StatusBadGateway)
		}, true},
		{"nothing upcoming", func(w http.ResponseWriter, _ *http.Request) {
			w.Write([]byte(`{"entity_id":"sensor.dep","state":"","attributes":{"times":[]}}`))
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.state)
			defer srv.Close()
			h := &DashboardHandler{
				haClient: homeassistant.NewClient(srv.URL, "token"),
				config:   &config.Config{Departures: config.DeparturesConfig{EntityIDs: []string{"sensor.dep"}}, Refresh: refreshCfg},
			}
			now := time.Now()
			var cs changes
			if view := h.fetchDepartures(context.Background(), now, &cs); view != nil {
				t.Fatalf("view = %+v, want nil", view)
			}
			retry := change{now.Add(refreshCfg.Retry), sourceDepartures}
			if got := len(cs) == 1 && cs[0] == retry; got != tt.wantRetry {
				t.Fatalf("changes = %v, want retry %v", cs, tt.wantRetry)
			}
		})
	}
}
