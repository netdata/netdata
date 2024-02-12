// SPDX-License-Identifier: GPL-3.0-or-later

package elasticsearch

import (
	"errors"
	"net/http"

	"github.com/netdata/go.d.plugin/pkg/web"
)

func (es *Elasticsearch) validateConfig() error {
	if es.URL == "" {
		return errors.New("URL not set")
	}
	if !(es.DoNodeStats || es.DoClusterHealth || es.DoClusterStats || es.DoIndicesStats) {
		return errors.New("all API calls are disabled")
	}
	if _, err := web.NewHTTPRequest(es.Request); err != nil {
		return err
	}
	return nil
}

func (es *Elasticsearch) initHTTPClient() (*http.Client, error) {
	return web.NewHTTPClient(es.Client)
}
