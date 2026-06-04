package homeassistant

import (
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// WeatherData holds the combined current + forecast weather information.
type WeatherData struct {
	Temperature    float64
	Condition      string
	TempHigh       float64
	TempLow        float64
	Sunrise        time.Time
	Sunset         time.Time
	HourlyForecast []HourlyForecast
}

// HourlyForecast holds a single hourly forecast entry.
type HourlyForecast struct {
	Time        time.Time
	Temperature float64
	Condition   string
}

// weatherAttributes maps the attributes returned by a weather entity state.
type weatherAttributes struct {
	Temperature float64 `json:"temperature"`
	Humidity    float64 `json:"humidity"`
	Pressure    float64 `json:"pressure"`
	WindSpeed   float64 `json:"wind_speed"`
}

// forecastEntry represents a single entry from weather/get_forecasts response.
type forecastEntry struct {
	Datetime      string  `json:"datetime"`
	Temperature   float64 `json:"temperature"`
	TempLow       float64 `json:"templow"`
	Condition     string  `json:"condition"`
	Precipitation float64 `json:"precipitation"`
}

// forecastServiceResponse is the response shape from weather/get_forecasts.
// It maps entity_id -> { "forecast": [...] }.
type forecastServiceResponse struct {
	Forecast []forecastEntry `json:"forecast"`
}

// sunAttributes holds sunrise/sunset from the sun.sun entity.
type sunAttributes struct {
	NextRising  string `json:"next_rising"`
	NextSetting string `json:"next_setting"`
}

// GetWeather fetches current weather state and today's forecast from Home Assistant.
func (c *Client) GetWeather(entityID string) (*WeatherData, error) {
	// Get current conditions from the weather entity state.
	state, err := c.GetState(entityID)
	if err != nil {
		return nil, fmt.Errorf("fetching weather state: %w", err)
	}

	var attrs weatherAttributes
	if err := json.Unmarshal(state.Attributes, &attrs); err != nil {
		return nil, fmt.Errorf("decoding weather attributes: %w", err)
	}

	data := &WeatherData{
		Temperature: attrs.Temperature,
		Condition:   state.State, // e.g. "sunny", "cloudy", "rainy"
	}

	// Get daily forecast for high/low.
	forecastResp, err := c.CallService("weather", "get_forecasts", map[string]any{
		"entity_id": entityID,
		"type":      "daily",
	})
	if err != nil {
		log.Printf("forecast service call error: %v", err)
		return data, nil // return what we have if forecast fails
	}

	if raw, ok := forecastResp[entityID]; ok {
		var fr forecastServiceResponse
		if err := json.Unmarshal(raw, &fr); err == nil && len(fr.Forecast) > 0 {
			today := fr.Forecast[0]
			data.TempHigh = today.Temperature
			data.TempLow = today.TempLow
		}
	}

	// Get hourly forecast for upcoming hours (2-hour increments).
	hourlyResp, err := c.CallService("weather", "get_forecasts", map[string]any{
		"entity_id": entityID,
		"type":      "hourly",
	})
	if err != nil {
		log.Printf("hourly forecast service call error: %v", err)
	} else if raw, ok := hourlyResp[entityID]; ok {
		var fr forecastServiceResponse
		if err := json.Unmarshal(raw, &fr); err == nil {
			now := time.Now()
			for _, entry := range fr.Forecast {
				t, err := time.Parse(time.RFC3339, entry.Datetime)
				if err != nil {
					continue
				}
				// Skip past hours and only take every 2 hours.
				if t.Before(now) {
					continue
				}
				if len(data.HourlyForecast) > 0 {
					last := data.HourlyForecast[len(data.HourlyForecast)-1].Time
					if t.Sub(last) < 2*time.Hour {
						continue
					}
				}
				data.HourlyForecast = append(data.HourlyForecast, HourlyForecast{
					Time:        t,
					Temperature: entry.Temperature,
					Condition:   entry.Condition,
				})
				if len(data.HourlyForecast) >= 5 {
					break
				}
			}
		}
	}

	// Get sunrise/sunset from the sun entity (always available in HA).
	sunState, err := c.GetState("sun.sun")
	if err != nil {
		log.Printf("sun state error: %v", err)
		return data, nil // return what we have if sun fails
	}

	var sun sunAttributes
	if err := json.Unmarshal(sunState.Attributes, &sun); err == nil {
		if t, err := time.Parse(time.RFC3339Nano, sun.NextRising); err == nil {
			data.Sunrise = t
		}
		if t, err := time.Parse(time.RFC3339Nano, sun.NextSetting); err == nil {
			data.Sunset = t
		}
	}

	return data, nil
}
