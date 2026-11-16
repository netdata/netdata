// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import (
	"context"
	"errors"

	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/meraki/merakiapi"
)

func (c *Collector) initClient(ctx context.Context) error {
	cfg := web.ClientConfig{
		Timeout:   c.Timeout,
		ProxyURL:  c.ProxyURL,
		TLSConfig: c.TLSConfig,
	}
	transport, err := web.NewHTTPTransport(ctx, cfg)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// The underlying error can echo the proxy URL, which may hold credentials.
		return errors.New("cannot initialize Meraki HTTP transport; check proxy and TLS configuration")
	}
	httpClient := web.NewHTTPClientWithTransport(cfg, &pacedTransport{
		base: transport,
	})

	client, err := merakiapi.New(
		merakiapi.Config{
			BaseURL:         c.URL,
			APIKey:          c.APIKey,
			BearerTokenFile: c.BearerTokenFile,
		},
		httpClient,
	)
	if err != nil {
		httpClient.CloseIdleConnections()
		return err
	}
	c.httpClient = httpClient
	c.client = client
	return ctx.Err()
}
