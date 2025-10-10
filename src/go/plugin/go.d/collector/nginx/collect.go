// SPDX-License-Identifier: GPL-3.0-or-later

package nginx

import (
	"fmt"
	"io"

	"github.com/netdata/netdata/go/plugins/pkg/stm"
	"github.com/netdata/netdata/go/plugins/pkg/web"
)

func (c *Collector) collect() (map[string]int64, error) {
	req, err := web.NewHTTPRequest(c.RequestConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request to '%s': %w'", c.URL, err)
	}

	var status *stubStatus
	var perr error

	if err := web.DoHTTP(c.httpClient).Request(req, func(body io.Reader) error {
		if status, perr = parseStubStatus(body); perr != nil {
			return perr
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return stm.ToMap(status), nil
}
