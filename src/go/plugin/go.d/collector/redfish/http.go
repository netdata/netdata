// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"context"
	"net/http"

	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
	"github.com/netdata/netdata/go/plugins/pkg/web"
)

func newHTTPClient(ctx context.Context, cfg Config) (*http.Client, error) {
	client, err := web.NewHTTPClient(ctx, web.ClientConfig{
		Timeout:           cfg.Timeout,
		NotFollowRedirect: true,
		ProxyURL:          cfg.ProxyURL,
		TLSConfig: tlscfg.TLSConfig{
			TLSCA:              cfg.TLSCA,
			TLSCert:            cfg.TLSCert,
			TLSKey:             cfg.TLSKey,
			InsecureSkipVerify: cfg.TLSSkipVerify,
		},
	})
	if err != nil {
		return nil, err
	}
	// Redfish applies its own exact-origin redirect policy.
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return client, nil
}
