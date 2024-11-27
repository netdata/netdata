// SPDX-License-Identifier: GPL-3.0-or-later

package tengine

import (
	"fmt"
	"io"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (c *Collector) collect() (map[string]int64, error) {
	req, err := web.NewHTTPRequest(c.RequestConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	var status *tengineStatus
	var perr error

	if err := web.DoHTTP(c.httpClient).Request(req, func(body io.Reader) error {
		if status, perr = parseStatus(body); perr != nil {
			return perr
		}
		return nil
	}); err != nil {
		return nil, err
	}

	mx := make(map[string]int64)

	for _, m := range *status {
		for k, v := range stm.ToMap(m) {
			mx[k] += v
		}
	}

	return mx, nil
}
