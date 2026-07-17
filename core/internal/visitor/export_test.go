package visitor

import "time"

// SetNow overrides the clock so tests can cross a date boundary without
// waiting for midnight. Compiled only under test.
func SetNow(h *Hasher, now func() time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.now = now
}
