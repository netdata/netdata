// SPDX-License-Identifier: GPL-3.0-or-later

package hdfs

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (h *HDFS) validateConfig() error {
	if h.URL == "" {
		return errors.New("url not set")
	}
	return nil
}

func (h *HDFS) createClient() (*client, error) {
	httpClient, err := web.NewHTTPClient(h.Client)
	if err != nil {
		return nil, err
	}

	return newClient(httpClient, h.Request), nil
}
