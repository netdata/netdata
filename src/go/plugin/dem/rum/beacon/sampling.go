// SPDX-License-Identifier: GPL-3.0-or-later

package beacon

import "hash/fnv"

// SessionSampled decides whether a session is in the investigate sample.
// The decision depends only on the session id, so every beacon of a
// session gets the same answer and a session is kept or dropped whole.
func SessionSampled(sessionID string, rate float64) bool {
	if rate >= 1 {
		return true
	}
	if rate <= 0 {
		return false
	}
	h := fnv.New64a()
	h.Write([]byte(sessionID))
	return float64(h.Sum64()%1_000_000)/1_000_000 < rate
}

// poorAbove are the Core Web Vitals "poor" thresholds (values above them).
var poorAbove = map[string]float64{LCP: 4000, INP: 500, CLS: 0.25, FCP: 3000, TTFB: 1800}

// Poor reports whether the vital rates "poor".
func (v Vital) Poor() bool {
	limit, ok := poorAbove[v.Name]
	return ok && v.Value > limit
}

// HasPoorVital reports whether any vital in the beacon rates "poor".
func (b *Beacon) HasPoorVital() bool {
	for _, v := range b.Vitals {
		if v.Poor() {
			return true
		}
	}
	return false
}
