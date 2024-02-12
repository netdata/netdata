// SPDX-License-Identifier: GPL-3.0-or-later

package apache

import (
	"errors"
	"net/http"
	"strings"

	"github.com/netdata/go.d.plugin/pkg/web"
)

func (a Apache) verifyConfig() error {
	if a.URL == "" {
		return errors.New("url not set")
	}
	if !strings.HasSuffix(a.URL, "?auto") {
		return errors.New("invalid URL, should ends in '?auto'")
	}
	return nil
}

func (a Apache) initHTTPClient() (*http.Client, error) {
	return web.NewHTTPClient(a.Client)
}
