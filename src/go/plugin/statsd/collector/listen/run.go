// SPDX-License-Identifier: GPL-3.0-or-later

package listen

import (
	"context"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/statsd/collector/listen/internal/server"
)

// runner is the part of *server.Server that serve drives.
type runner interface {
	Start()
	Failed() <-chan struct{}
	Close()
	Err() error
}

func (c *Collector) run(ctx context.Context, ready func()) error {
	if c.receiver == nil {
		return errNotInitialized
	}
	if ctx.Err() != nil {
		return nil
	}
	s, err := server.Listen(ctx, c.serverConfig())
	if err != nil {
		return err
	}
	return c.serve(ctx, s, ready)
}

// serverConfig expands every configured listener into the sockets it binds,
// in listener order.
func (c *Collector) serverConfig() server.Config {
	var endpoints []server.Endpoint
	for i, l := range c.Listeners {
		for _, protocol := range l.protocols() {
			endpoints = append(endpoints, server.Endpoint{
				Network: protocol,
				Address: l.Address,
				Name:    fmt.Sprintf("listeners[%d]", i),
			})
		}
	}
	r := c.receiver
	return server.Config{
		Endpoints: endpoints,
		MaxRecord: c.maxRecord,
		MaxConns:  c.MaxTCPConnections,
		Stats:     &c.diagnostics.transport,
		Ingest:    func(record string, now time.Time) { _ = r.ingest(record, now) },
		Now:       c.now,
		Log:       c.Logger,
	}
}

// serve owns an acquired server. Readiness follows successful acquisition, not
// traffic. On cancellation or listener loss, admission stops before sockets
// close, so a failed receiver cannot publish held or empty intervals.
func (c *Collector) serve(ctx context.Context, s runner, ready func()) error {
	c.receiver.start()
	s.Start()
	ready()
	select {
	case <-ctx.Done():
	case <-s.Failed():
	}
	c.receiver.stop()
	s.Close()
	return s.Err()
}
