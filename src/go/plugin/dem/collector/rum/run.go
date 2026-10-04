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
	traceDestination := ""
	if c.TracingOn() {
		traceDestination = c.Tracing.ExportTo
	}
	exporter, err := otlp.New(ctx, c.OTLP, c.Name, traceDestination, c.aggregator, c.redactor)
	if err != nil {
		return err
	}
	defer exporter.Close()
	writer := history.NewWriter(c.Name, c.deps.History, c.aggregator, c.redactor)
	c.aggregator.SetHistorySink(writer)
	state := diagnostics.New(c.Site)
	route := httpapi.NewRoute(c.Site, &processor{
		aggregator: c.aggregator,
		exporter:   exporter,
	}, state)
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
	workers.Go(func() { exporter.Run(workersCtx) })
	probeCtx, stopProbes := context.WithCancel(ctx)
	probesDone := make(chan struct{})
	client := &http.Client{
		Timeout: 15 * time.Second,
	}
	go func() {
		defer close(probesDone)
		state.RunReachability(probeCtx, 5*time.Minute, c.probeBase, client)
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
// diagnostic owner probes the trusted-proxy address it learned before promoting it.
func (c *Collector) probeBase() string {
	if c.PublicURL != "" {
		return c.PublicURL
	}
	return c.deps.Registry.Availability().PublicURL
}
