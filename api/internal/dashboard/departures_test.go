package dashboard

import (
	"testing"
	"time"

	"github.com/thiemok/tiny-dash/api/internal/homeassistant"
)

func TestDepartureTime(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("no tzdata:", err)
	}
	now := time.Date(2026, 10, 23, 14, 0, 0, 0, berlin)
	planned := time.Date(2026, 10, 23, 12, 32, 0, 0, time.UTC) // 14:32 in Berlin

	tests := []struct {
		name string
		dep  homeassistant.Departure
		want string
	}{
		{"planned in now's zone", homeassistant.Departure{Planned: planned, Estimated: planned.Add(2 * time.Minute)}, "14:32"},
		{"estimated when unplanned", homeassistant.Departure{Estimated: planned}, "14:32"},
	}
	for _, tt := range tests {
		if got := departureTime(tt.dep, now); got != tt.want {
			t.Errorf("%s: departureTime = %q, want %q", tt.name, got, tt.want)
		}
	}
}
