// SPDX-License-Identifier: GPL-3.0-or-later

package rabbitmq

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (r *RabbitMQ) collectOverview(mx map[string]int64) error {
	req, err := web.NewHTTPRequestWithPath(r.RequestConfig, urlPathAPIOverview)
	if err != nil {
		return fmt.Errorf("failed to create overview stats request: %w", err)
	}

	var resp apiOverviewResp

	if err := r.webClient().RequestJSON(req, &resp); err != nil {
		return err
	}

	for k, v := range stm.ToMap(resp) {
		mx[k] = v
	}

	return nil
}
