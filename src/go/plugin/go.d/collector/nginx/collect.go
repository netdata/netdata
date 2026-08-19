// SPDX-License-Identifier: GPL-3.0-or-later

package nginx

import (
	"context"
	"fmt"
	"io"

	"github.com/netdata/netdata/go/plugins/pkg/stm"
	"github.com/netdata/netdata/go/plugins/pkg/web"
)

func (c *Collector) collect(ctx context.Context) (map[string]int64, error) {
	req, err := web.NewHTTPRequest(c.RequestConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request to '%s': %w'", c.URL, err)
	}
	req = req.WithContext(ctx)

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
