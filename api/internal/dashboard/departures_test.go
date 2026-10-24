package dashboard

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/thiemok/tiny-dash/api/internal/homeassistant"
)

func TestDepartureTime(t *testing.T) {
	cest := time.FixedZone("CEST", 2*60*60)
	now := time.Date(2026, 10, 23, 14, 0, 0, 0, cest)
	planned := time.Date(2026, 10, 23, 12, 32, 0, 0, time.UTC) // 14:32 CEST

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

func TestDeparturesPartial(t *testing.T) {
	tmpl, err := loadTemplates(os.DirFS("../.."))
	if err != nil {
		t.Fatal(err)
	}
	data := templateData{Departures: &departuresView{Departures: []departureRow{
		{Line: "U76", Direction: "Krefeld", Minutes: 12, Time: "14:32", Delay: 2},
		{Line: "S8", Direction: "Hagen", Minutes: 20, Time: "14:40", Delay: 3, Cancelled: true},
	}}}
	var out strings.Builder
	if err := tmpl.ExecuteTemplate(&out, "departures", data); err != nil {
		t.Fatal(err)
	}
	cells := regexp.MustCompile(`class="dep-(eta|delay)">([^<]*)<`).FindAllStringSubmatch(out.String(), -1)
	var got []string
	for _, c := range cells {
		got = append(got, c[1]+"="+c[2])
	}
	want := []string{"eta=14:32", "delay=+2", "eta=cancelled", "delay="}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("departure cells = %v, want %v", got, want)
	}
}
