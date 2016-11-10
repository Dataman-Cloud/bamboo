package event_bus

import (
	"fmt"
)

func handleF5Update(h *Handlers) {
	reloadStart := time.Now()
	// TODO fetch app from marathon and register it to F5
	reloaded, err := ensureLatestConfig(h)
}
