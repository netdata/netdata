// SPDX-License-Identifier: GPL-3.0-or-later

package rum

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/diagnostics"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/httpapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/otlp"
	rumregistry "github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
)

// Run admits an independent site without waiting for receiver startup. Worker
// cancellation occurs only after exact route/read leases have drained.
func (c *Collector) Run(ctx context.Context, ready func()) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.aggregator == nil {
		return errors.New("site is not initialized")
	}
	p := &processor{
		aggregator: c.aggregator,
	}
	if c.EventLogsOn() {
		logs, err := otlp.NewLogs(ctx, c.EventLogs.Destination, c.Name, c.aggregator, c.redactor)
		if err != nil {
			return err
		}
		p.logs = logs
		defer logs.Close()
	}
	if c.TracingOn() {
		traces, err := otlp.NewTraces(ctx, c.Tracing.Destination, c.Name, c.aggregator, c.redactor)
		if err != nil {
			return err
		}
		p.traces = traces
		defer traces.Close()
	}
	writer := history.NewWriter(c.Name, c.deps.History, c.aggregator)
	c.aggregator.SetHistorySink(writer)
	state := diagnostics.New()
	route := httpapi.NewRoute(c.Site, p, state)
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return err
	}
	retire, err := c.deps.Registry.Register(
		c.Name,
		&rumregistry.Site{
			Route:       route,
			Diagnostics: state,
			Aggregator:  c.aggregator,
			Generation:  hex.EncodeToString(id[:]),
			Redactor:    c.redactor,
		},
	)
	if err != nil {
		return err
	}
	workersCtx, stopWorkers := context.WithCancel(context.Background())
	var workers sync.WaitGroup
	workers.Go(func() { writer.Run(workersCtx) })
	// Enabled exports share one absolute final-flush budget after route leases drain.
	var finalOnce sync.Once
	var finalCtx context.Context
	var finalCancel context.CancelFunc
	final := func() context.Context {
		finalOnce.Do(func() {
			finalCtx, finalCancel = context.WithTimeout(context.Background(), otlp.ShutdownFlushTimeout)
		})
		return finalCtx
	}
	defer func() {
		if finalCancel != nil {
			finalCancel()
		}
	}()
	if p.logs != nil {
		workers.Go(func() { p.logs.Run(workersCtx, final) })
	}
	if p.traces != nil {
		workers.Go(func() { p.traces.Run(workersCtx, final) })
	}
	ready()
	<-ctx.Done()
	retire()
	stopWorkers()
	workers.Wait()
	return nil
}
