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
	return t.sdk.Exchange(ctx, req, res)
}

// Close closes the device descriptor. It sends no IPMI command and the SDK ignores ctx.
func (t *openTransport) Close(ctx context.Context) error {
	return t.sdk.Close(ctx)
}

// lanTransport owns one remote session and its SDK keepalive.
type lanTransport struct {
	sdk *client.Client
}

func newTransport(cfg Config) (transport, error) {
	if cfg.Driver == "" || cfg.Driver == "open" {
		return newOpenTransport(cfg)
	}
	sdk, err := client.NewClient(cfg.Hostname, cfg.Port, cfg.Username, cfg.Password)
	if err != nil {
		return nil, fmt.Errorf("create IPMI client: %w", err)
	}
	privilege := map[string]types.PrivilegeLevel{
		"user":          types.PrivilegeLevelUser,
		"operator":      types.PrivilegeLevelOperator,
		"administrator": types.PrivilegeLevelAdministrator,
	}[cfg.PrivilegeLevel]
	sdk.WithInterface(client.Interface(cfg.Driver)).WithTimeout(cfg.Timeout).
		WithRetry(0).WithMaxPrivilegeLevel(privilege)
	return &lanTransport{sdk: sdk}, nil
}

func (t *lanTransport) Connect(ctx context.Context) error {
	return t.sdk.Connect(ctx)
}

func (t *lanTransport) Exchange(ctx context.Context, req types.Request, res types.Response) error {
	return t.sdk.Exchange(ctx, req, res)
}

func (t *lanTransport) Close(ctx context.Context) error {
	// A failed or canceled operation still needs to release its BMC session.
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	return t.sdk.Close(cleanupCtx)
}
