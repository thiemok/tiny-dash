package dashboard

import (
	"fmt"
	"time"
)

// mockWeather returns a weather view that exercises every condition icon
// the CSS knows about, so the visuals can be reviewed in one screenshot.
func mockWeather(now time.Time) *weatherView {
	// Show the current condition rotating through states based on the hour
	// so different runs see different "now" icons.
	allConditions := []string{
		"sunny", "partlycloudy", "cloudy", "rainy", "pouring",
		"snowy", "fog", "lightning", "windy", "clear-night",
	}
	currentIdx := now.Hour() % len(allConditions)
	currentCondition := allConditions[currentIdx]

	view := &weatherView{
		Temperature: 8,
		Condition:   currentCondition,
		Icon:        conditionClass[currentCondition],
		TempHigh:    14,
		TempLow:     3,
		SunriseStr:  "06:12",
		SunsetStr:   "19:48",
	}

	// Forecast: pack distinct conditions into each slot.
	forecastConditions := []string{
		"sunny", "partlycloudy", "rainy", "lightning", "snowy",
	}
	temps := []float64{12, 11, 9, 7, 4}
	startHour := now.Hour() + 1
	for i, cond := range forecastConditions {
		view.HourlyForecast = append(view.HourlyForecast, hourlyForecastView{
			TimeStr:     fmt.Sprintf("%02d", (startHour+i*2)%24),
			Temperature: temps[i],
			Icon:        conditionClass[cond],
		})
	}
	return view
}

// mockCalendar returns a calendar view with a mix of all-day and timed
// events spanning the full day, including one currently ongoing.
func mockCalendar(now time.Time) *calendarView {
	colors := calendarPalette
	view := &calendarView{
		AllDay: []eventView{
			{TimeStr: "All day", Summary: "Fronleichnam (regionaler Feiertag)", AllDay: true, Source: "Feiertage", Color: colors[2]},
			{TimeStr: "All day", Summary: "Sarahs Geburtstag", AllDay: true, Source: "Birthdays", Color: colors[1]},
		},
	}
	start := time.Date(now.Year(), now.Month(), now.Day(), 8, 0, 0, 0, now.Location())
	events := []struct {
		offsetMin int
		duration  time.Duration
		summary   string
		source    int
	}{
		{0, 60 * time.Minute, "Standup", 0},
		{90, 30 * time.Minute, "1:1 with Marc", 0},
		{180, 60 * time.Minute, "Design review", 0},
		{240, 30 * time.Minute, "Coffee with Lisa", 3},
		{360, 90 * time.Minute, "Lunch break", 3},
		{480, 60 * time.Minute, "Sprint planning", 0},
		{600, 60 * time.Minute, "Pickup kids from school", 3},
		{720, 120 * time.Minute, "Cinema: new release", 3},
	}
	for _, e := range events {
		evStart := start.Add(time.Duration(e.offsetMin) * time.Minute)
		evEnd := evStart.Add(e.duration)
		view.Timed = append(view.Timed, eventView{
			TimeStr: evStart.Format("15:04"),
			Summary: e.summary,
			AllDay:  false,
			Ongoing: !evStart.After(now) && evEnd.After(now),
			Source:  shortCalendarLabel("calendar.x"),
			Color:   colors[e.source%len(colors)],
		})
	}
	return view
}

// mockDepartures returns many upcoming departures across multiple lines,
// including a delayed and a cancelled entry.
func mockDepartures(now time.Time) *departuresView {
	view := &departuresView{}
	rows := []struct {
		line      string
		direction string
		afterMin  int
		delay     int
		cancelled bool
	}{
		{"S8", "Hagen Hbf", 1, 0, false},
		{"709", "D-G'heim, Krankenhaus", 4, 1, false},
		{"S11", "D-Flughafen Terminal", 6, 0, false},
		{"U70", "Krefeld Rheinstr.", 9, 0, false},
		{"S8", "Hagen Hbf", 12, 3, false},
		{"709", "D-G'heim, Krankenhaus", 14, 0, false},
		{"RE5", "Koblenz Hbf", 17, 0, true},
		{"S11", "Bergisch Gladbach", 21, 0, false},
		{"U76", "Krefeld Hbf", 24, 2, false},
		{"709", "D-G'heim, Krankenhaus", 29, 0, false},
		{"S8", "Mönchengladbach Hbf", 32, 0, false},
		{"S28", "Wuppertal Vohwinkel", 36, 0, false},
		{"RE6", "Köln/Bonn Flughafen", 41, 5, false},
		{"709", "Benrath Bf", 44, 0, false},
		{"S11", "D-Flughafen Terminal", 51, 0, false},
		{"S8", "Hagen Hbf", 52, 0, false},
	}
	for _, r := range rows {
		row := departureRow{
			Line:      r.line,
			Direction: r.direction,
			TimeStr:   now.Add(time.Duration(r.afterMin) * time.Minute).Format("15:04"),
			Minutes:   r.afterMin,
			Delay:     r.delay,
			Cancelled: r.cancelled,
		}
		row.MinutesStr = formatMinutesUntil(r.afterMin)
		view.Departures = append(view.Departures, row)
	}
	return view
}

