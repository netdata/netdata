// SPDX-License-Identifier: GPL-3.0-or-later

package aggregate

import (
	"hash/fnv"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

// sessionTTL is how long a session lasts without a beacon before the next
// one starts a new session.
const sessionTTL = 30 * time.Minute

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

// poorVital reports whether a vital rates "poor" under the same thresholds
// as the charts and page selectors. Unsupported names never rate poor.
func poorVital(v beacon.Vital) bool {
	th, ok := thresholds[v.Name]
	return ok && th.isPoor(v.Value)
}

// hasPoorVital reports whether any vital in the beacon rates "poor".
func hasPoorVital(b *beacon.Beacon) bool {
	for _, v := range b.Vitals {
		if poorVital(v) {
			return true
		}
	}
	return false
}
