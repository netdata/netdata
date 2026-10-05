// SPDX-License-Identifier: GPL-3.0-or-later

package rum

import (
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/otlp"
)

// processor preserves the measurement, history enqueue and export order for
// each admitted observation. The route lease covers the complete sequence.
type processor struct {
	aggregator *aggregate.Aggregator
	logs       *otlp.Logs
	traces     *otlp.Traces
}

func (p *processor) Ingest(b *beacon.Beacon) {
	result := p.aggregator.Ingest(b)
	if p.logs != nil {
		p.logs.Ingest(b, result)
	}
	if p.traces != nil {
		p.traces.Ingest(b, result)
	}
}

func (p *processor) Reject(site, reason string) {
	p.aggregator.Reject(site, reason)
}
