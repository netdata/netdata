// SPDX-License-Identifier: GPL-3.0-or-later

package netdataprovider

import (
	"context"
	"errors"
	"sync"

	"github.com/netdata/netdata/otel-facade-poc/internal/control"
	"go.opentelemetry.io/collector/confmap"
)

type provider struct {
	controller *control.Controller
}

func NewFactory() confmap.ProviderFactory {
	return confmap.NewProviderFactory(func(confmap.ProviderSettings) confmap.Provider { return &provider{} })
}

func (*provider) Scheme() string { return "netdata" }

func (p *provider) Retrieve(ctx context.Context, uri string, watcher confmap.WatcherFunc) (*confmap.Retrieved, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if uri != "netdata:local" {
		return nil, errors.New("the POC supports only netdata:local")
	}
	if p.controller == nil {
		var err error
		p.controller, err = control.OpenProcess()
		if err != nil {
			return nil, err
		}
	}
	config, changed, terminal, err := p.controller.Snapshot()
	if err != nil {
		return nil, err
	}
	if err := p.controller.Start(); err != nil {
		return nil, err
	}
	if watcher == nil {
		return confmap.NewRetrieved(config)
	}
	// Exactly one callback per snapshot. It may block inside the resolver, so
	// never call it from the command owner or while holding controller locks.
	closed, done := make(chan struct{}), make(chan struct{})
	var once sync.Once
	go func() {
		defer close(done)
		var event confmap.ChangeEvent
		select {
		case <-closed:
			return
		case <-changed:
		case <-terminal:
			event.Error = p.controller.TerminalError()
		}
		select {
		case <-closed:
			return
		default:
			watcher(&event)
		}
	}()
	return confmap.NewRetrieved(config, confmap.WithRetrievedClose(func(ctx context.Context) error {
		once.Do(func() { close(closed) })
		select {
		case <-done:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}))
}

func (p *provider) Shutdown(context.Context) error {
	if p.controller != nil {
		return p.controller.Close()
	}
	return nil
}
