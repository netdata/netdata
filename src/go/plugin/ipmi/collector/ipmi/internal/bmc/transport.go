// SPDX-License-Identifier: GPL-3.0-or-later

package bmc

import (
	"context"
	"fmt"
	"time"

	"github.com/bougou/go-ipmi/pkg/client"
	"github.com/bougou/go-ipmi/pkg/types"
)

// transport is the command/response boundary. Tests replace it below wire
// decoding, so responses still go through the SDK's Unpack.
type transport interface {
	Connect(context.Context) error
	Exchange(context.Context, types.Request, types.Response) error
	Close(context.Context) error
}

// openTransport is the SDK client of a local Linux OpenIPMI device.
type openTransport struct {
	sdk *client.Client
	cfg Config
}

func newOpenTransport(cfg Config) (transport, error) {
	// Only NewOpenClient initializes the state that ConnectOpen requires.
	sdk, err := client.NewOpenClient()
	if err != nil {
		return nil, fmt.Errorf("create IPMI client: %w", err)
	}
	sdk.WithTimeout(cfg.Timeout)
	return &openTransport{
		sdk: sdk,
		cfg: cfg,
	}, nil
}

func (t *openTransport) Connect(ctx context.Context) error {
	return t.sdk.ConnectOpen(ctx, t.cfg.Device)
}

func (t *openTransport) Exchange(ctx context.Context, req types.Request, res types.Response) error {
	timeout, err := receiveTimeout(ctx, t.cfg.Timeout)
	if err != nil {
		return err
	}
	// The SDK's receive ignores the context, so its timeout also carries the
	// remaining budget. Reader serializes commands and the local interface has
	// no keepalive, so the temporary setting cannot race.
	t.sdk.WithTimeout(timeout)
	defer t.sdk.WithTimeout(t.cfg.Timeout)
	return t.sdk.Exchange(ctx, req, res)
}

// Close closes the device descriptor. It sends no IPMI command and the SDK ignores ctx.
func (t *openTransport) Close(ctx context.Context) error {
	return t.sdk.Close(ctx)
}

// receiveTimeout returns the configured timeout capped by the context's remaining budget.
func receiveTimeout(ctx context.Context, configured time.Duration) (time.Duration, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		return configured, nil
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return 0, context.DeadlineExceeded
	}
	return min(configured, remaining), nil
}
