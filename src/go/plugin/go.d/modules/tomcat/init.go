// SPDX-License-Identifier: GPL-3.0-or-later

package tomcat

import (
	"fmt"
	"net/http"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (t *Tomcat) validateConfig() error {
	if t.URL == "" {
		return fmt.Errorf("url not set")
	}
	return nil
}

func (t *Tomcat) initHTTPClient() (*http.Client, error) {
	return web.NewHTTPClient(t.Client)
}
