// SPDX-License-Identifier: GPL-3.0-or-later

package elasticsearch

import (
	"errors"
	"net/http"

	"context"

	"github.com/netdata/netdata/go/plugins/pkg/web"
)

func (c *Collector) validateConfig(ctx context.Context) error {
	if c.URL == "" {
		return errors.New("URL not set")
	}
	if !(c.DoNodeStats || c.DoClusterHealth || c.DoClusterStats || c.DoIndicesStats) {
		return errors.New("all API calls are disabled")
	}
	if _, err := web.NewHTTPRequest(ctx, c.RequestConfig, c.CredentialFiles()); err != nil {
		return err
	}
	return nil
}

func (c *Collector) initHTTPClient(ctx context.Context) (*http.Client, error) {
	return web.NewHTTPClient(ctx, c.ClientConfig, c.CredentialFiles())
}
