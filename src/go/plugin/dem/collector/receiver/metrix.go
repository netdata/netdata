// SPDX-License-Identifier: GPL-3.0-or-later
package receiver

import (
	"sync/atomic"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

type collectorMetrics struct {
	state                               metrix.StateSetInstrument
	success, clientErrors, serverErrors metrix.SnapshotCounter
}

// requestCounts counts served requests by response status class.
type requestCounts struct {
	success, clientErrors, serverErrors atomic.Uint64
}

func (r *requestCounts) observe(status int) {
	switch {
	case status < 400:
		r.success.Add(1)
	case status < 500:
		r.clientErrors.Add(1)
	default:
		r.serverErrors.Add(1)
	}
}

func newCollectorMetrics(m metrix.SnapshotMeter) collectorMetrics {
	return collectorMetrics{
		state: m.StateSet(
			"receiver_state",
			metrix.WithStateSetStates("serving", "unavailable"),
			metrix.WithStateSetMode(metrix.ModeEnum),
		),
		success:      m.Counter("http_success"),
		clientErrors: m.Counter("http_client_errors"),
		serverErrors: m.Counter("http_server_errors"),
	}
}
