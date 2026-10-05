// SPDX-License-Identifier: GPL-3.0-or-later
package receiver

import "github.com/netdata/netdata/go/plugins/pkg/metrix"

type collectorMetrics struct {
	state    metrix.StateSetInstrument
	requests [3]metrix.SnapshotCounter
}

func newCollectorMetrics(m metrix.SnapshotMeter) collectorMetrics {
	return collectorMetrics{
		state: m.StateSet(
			"receiver_state",
			metrix.WithStateSetStates("serving", "unavailable"),
			metrix.WithStateSetMode(metrix.ModeEnum),
		),
		requests: [3]metrix.SnapshotCounter{
			m.Counter("http_success"),
			m.Counter("http_client_errors"),
			m.Counter("http_server_errors"),
		},
	}
}
