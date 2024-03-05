// SPDX-License-Identifier: GPL-3.0-or-later

package phpdaemon

import (
	"errors"

	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"
)

func (p *PHPDaemon) validateConfig() error {
	if p.URL == "" {
		return errors.New("url not set")
	}
	if _, err := web.NewHTTPRequest(p.Request); err != nil {
		return err
	}
	return nil
}

func (p *PHPDaemon) initClient() (*client, error) {
	httpClient, err := web.NewHTTPClient(p.Client)
	if err != nil {
		return nil, err
	}
	return newAPIClient(httpClient, p.Request), nil
}
