// SPDX-License-Identifier: GPL-3.0-or-later

package elasticsearch

import (
	"errors"

	"context"

	"github.com/netdata/netdata/go/plugins/pkg/web"
)

func (c *Collector) validateConfig() error {
	if c.URL == "" {
		return errors.New("URL not set")
	}
	if !(c.DoNodeStats || c.DoClusterHealth || c.DoClusterStats || c.DoIndicesStats) {
		return errors.New("all API calls are disabled")
	}
	return nil
}

func (c *Collector) initHTTPClient(ctx context.Context) (*web.HTTPClient, error) {
	client, err := web.NewHTTPClient(ctx, c.ClientConfig, c.CredentialFiles())
	if err != nil {
		return nil, err
	}
	if _, err := client.NewRequest(ctx, c.RequestConfig); err != nil {
		client.CloseIdleConnections()
		return nil, err
	}
	return client, nil
}
