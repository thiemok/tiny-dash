//go:build picodebug

package main

import (
	"context"
	"log/slog"

	"github.com/soypat/cyw43439/whd"
)

// simulateOutage stands in for an access point that goes away: the chip
// disassociates, so the link really drops and the firmware reports it, and
// the next failJoins joins fail as if the access point were still missing.
func (w *wifiLink) simulateOutage(failJoins int) {
	w.failJoins = failJoins
	w.pause()
	err := doIoctlSet(w.dev, whd.WLC_DISASSOC, whd.IF_STA, nil)
	w.resume()
	w.logger.LogAttrs(context.Background(), slog.LevelWarn, "test: simulated AP outage",
		slog.Int("failJoins", failJoins), slog.Any("error", err))
}
