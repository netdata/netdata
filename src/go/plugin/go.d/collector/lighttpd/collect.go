// SPDX-License-Identifier: GPL-3.0-or-later

package lighttpd

import (
	"fmt"
	"io"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (c *Collector) collect() (map[string]int64, error) {
	req, err := web.NewHTTPRequest(c.RequestConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %v", err)
	}

	var status *serverStatus
	var perr error

	if err := web.DoHTTP(c.httpClient).Request(req, func(body io.Reader) error {
		if status, perr = parseResponse(body); perr != nil {
			return perr
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return stm.ToMap(status), nil
}
