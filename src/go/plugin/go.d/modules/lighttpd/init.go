// SPDX-License-Identifier: GPL-3.0-or-later

package lighttpd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (l *Lighttpd) validateConfig() error {
	if l.URL == "" {
		return errors.New("url not set")
	}
	if !strings.HasSuffix(l.URL, "?auto") {
		return fmt.Errorf("bad URL '%s', should ends in '?auto'", l.URL)
	}
	return nil
}

func (l *Lighttpd) initApiClient() (*apiClient, error) {
	client, err := web.NewHTTPClient(l.Client)
	if err != nil {
		return nil, err
	}
	return newAPIClient(client, l.Request), nil
}
