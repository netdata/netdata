// SPDX-License-Identifier: GPL-3.0-or-later

package rum

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/ingest"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/otlp"
	"github.com/netdata/netdata/go/plugins/plugin/dem/runtimehub"
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
	traceDestination := ""
	if c.TracingOn() {
		traceDestination = c.Tracing.ExportTo
	}
	exporter, err := otlp.New(ctx, c.OTLP, c.Name, traceDestination, c.aggregator, c.redactor)
	if err != nil {
		return err
	}
	defer exporter.Close()
	writer := history.New(c.Name, c.deps.History, c.aggregator, c.redactor)
	c.aggregator.SetHistorySink(writer)
	route := ingest.NewRoute(c.Site, beacon.MultiSink{c.aggregator, exporter})
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return err
	}
	retire, err := c.deps.Hub.Register(
		c.Name,
		&runtimehub.Site{
			Route:      route,
			Aggregator: c.aggregator,
			Generation: hex.EncodeToString(id[:]),
			Redactor:   c.redactor,
		},
	)
	if err != nil {
		return err
	}
	workersCtx, stopWorkers := context.WithCancel(context.Background())
	var workers sync.WaitGroup
	workers.Go(func() { writer.Run(workersCtx) })
	workers.Go(func() { exporter.Run(workersCtx) })
	probeCtx, stopProbes := context.WithCancel(ctx)
	probesDone := make(chan struct{})
	client := &http.Client{
		Timeout: 15 * time.Second,
	}
	go func() {
		defer close(probesDone)
		route.RunReachability(probeCtx, 5*time.Minute, c.probeBase, client)
	}()
	ready()
	<-ctx.Done()
	stopProbes()
	retire()
	<-probesDone
	client.CloseIdleConnections()
	stopWorkers()
	workers.Wait()
	return nil
}

// probeBase supplies only an explicitly configured address. With none, the
// route probes the trusted-proxy address it learned before promoting it.
func (c *Collector) probeBase() string {
	if c.PublicURL != "" {
		return c.PublicURL
	}
	return c.deps.Hub.Availability().PublicURL
}
