// SPDX-License-Identifier: GPL-3.0-or-later

package tomcat

import (
	"errors"
	"net/http"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (a *Tomcat) validateConfig() error {
	if a.URL == "" {
		return errors.New("url not set")
	}
	if !strings.HasSuffix(a.URL, "status?XML=true") {
		return errors.New("invalid URL, should end in 'status?XML=true'")
	}
	return nil
}

func (a *Tomcat) initHTTPClient() (*http.Client, error) {
	return web.NewHTTPClient(a.Client)
}
