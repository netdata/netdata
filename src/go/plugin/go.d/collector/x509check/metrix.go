// SPDX-License-Identifier: GPL-3.0-or-later

package x509check

import "github.com/netdata/netdata/go/plugins/pkg/metrix"

type collectorMetrics struct {
	meter                 metrix.SnapshotMeter
	expiry                metrix.SnapshotGauge
	revocationStatus      metrix.StateSetInstrument
	revocationCheckStatus metrix.StateSetInstrument
}

func newCollectorMetrics(store metrix.CollectorStore) collectorMetrics {
	meter := store.Write().SnapshotMeter("")
	return collectorMetrics{
		meter:  meter,
		expiry: meter.Gauge("expiry"),
		revocationStatus: meter.StateSet("revocation_status",
			metrix.WithStateSetMode(metrix.ModeEnum),
			metrix.WithStateSetStates("not_revoked", "revoked"),
		),
		revocationCheckStatus: meter.StateSet("revocation_check_status",
			metrix.WithStateSetMode(metrix.ModeEnum),
			metrix.WithStateSetStates(checkOutcomes...),
		),
	}
}
