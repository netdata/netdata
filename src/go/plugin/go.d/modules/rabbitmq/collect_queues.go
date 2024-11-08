// SPDX-License-Identifier: GPL-3.0-or-later

package rabbitmq

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (r *RabbitMQ) collectQueues(mx map[string]int64) error {
	req, err := web.NewHTTPRequestWithPath(r.RequestConfig, urlPathAPIQueues)
	if err != nil {
		return fmt.Errorf("failed to create queues stats request: %w", err)
	}

	var resp []apiQueueResp

	if err := r.webClient().RequestJSON(req, &resp); err != nil {
		return err
	}

	for _, q := range resp {
		r.cache.getQueue(q).seen = true

		px := fmt.Sprintf("queue_%s_vhost_%s_node_%s_", q.Name, q.Vhost, q.Node)

		for k, v := range stm.ToMap(q) {
			mx[px+k] = v
		}
	}

	return nil
}
