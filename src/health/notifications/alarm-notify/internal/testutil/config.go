// SPDX-License-Identifier: GPL-3.0-or-later

package testutil

import (
	"context"
	"io"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/notifier"
)

type Document[C any] struct {
	Version      int
	Destinations map[string]C
	Routing      notifier.Routing
}

type capturedConfig[C any] struct{ config C }

func (*capturedConfig[C]) Send(context.Context, event.Event) error { return nil }

// ReadConfig exposes decoded values to provider config tests through the real
// document loader and typed factory, without exposing a sender's private state.
func ReadConfig[C any](r io.Reader, provider string, validate func(C) error) (Document[C], error) {
	registry := config.Registry{provider: config.Factory(func(value C) (notifier.Sender, error) {
		if err := validate(value); err != nil {
			return nil, err
		}
		return &capturedConfig[C]{config: value}, nil
	})}
	plan, err := config.Read(r, registry)
	if err != nil {
		return Document[C]{}, err
	}
	result := Document[C]{Version: 1, Destinations: make(map[string]C, len(plan.Destinations)), Routing: plan.Routing}
	for name, sender := range plan.Destinations {
		result.Destinations[name] = sender.(*capturedConfig[C]).config
	}
	return result, nil
}
