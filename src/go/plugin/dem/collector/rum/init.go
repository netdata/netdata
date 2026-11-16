// SPDX-License-Identifier: GPL-3.0-or-later

package rum

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	redact "github.com/netdata/netdata/go/plugins/plugin/dem/internal/redact"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/otlp"
)

func (c *Collector) Init(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.deps.Registry == nil || c.deps.History == nil {
		return errors.New("missing runtime/history dependencies")
	}
	c.Site = c.Site.Effective()
	if err := c.Site.Validate(); err != nil {
		return err
	}
	if time.Duration(c.Window) < time.Minute || time.Duration(c.Window) > 30*time.Minute {
		return errors.New("window must be between 1m and 30m")
	}
	if c.EventLogsOn() {
		if err := otlp.ValidateDestination(ctx, c.EventLogs.Destination); err != nil {
			return fmt.Errorf("event_logs.destination: %w", err)
		}
	}
	if c.TracingOn() {
		if err := otlp.ValidateDestination(ctx, c.Tracing.Destination); err != nil {
			return fmt.Errorf("tracing.destination: %w", err)
		}
	}
	// Retained inactive credentials still need output redaction.
	c.redactor = redact.NewRedactor(c.Secrets()...)
	c.aggregator = aggregate.New(time.Duration(c.Window), aggregate.SiteCfg{
		Name:               c.Name,
		VnodeGUID:          c.vnodeGUID,
		DisplayName:        c.Label(),
		PageGroups:         c.PageGroups,
		Countries:          c.Countries,
		FrustrationSignals: c.FrustrationSignalsOn(),
		Investigate: aggregate.InvestigateCfg{
			Rate:           c.InvestigateRate(),
			KeepErrors:     c.KeepsErrors(),
			KeepPoorVitals: c.KeepsPoorVitals(),
		},
	})
	c.transport = newTransportCounters()
	m := c.store.Write().SnapshotMeter("")
	if c.DisplayName != "" && c.DisplayName != c.Name {
		m = m.WithLabels(metrix.Label{
			Key:   "display_name",
			Value: c.redactor.Apply(c.DisplayName),
		})
	}
	c.metrics = newCollectorMetrics(m, c.EventLogsOn(), c.TracingOn(), c.FrustrationSignalsOn())
	return nil
}
