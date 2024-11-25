// SPDX-License-Identifier: GPL-3.0-or-later

package apache

import (
	"errors"
	"net/http"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (a *Apache) validateConfig() error {
	if a.URL == "" {
		return errors.New("url not set")
	}
	if !strings.HasSuffix(a.URL, "?auto") {
		return errors.New("invalid URL, should ends in '?auto'")
	}
	return nil
}

func (a *Apache) initHTTPClient() (*http.Client, error) {
	return web.NewHTTPClient(a.ClientConfig)
}
