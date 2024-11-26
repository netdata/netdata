// SPDX-License-Identifier: GPL-3.0-or-later

package rabbitmq

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (c *Collector) collectQueues(mx map[string]int64) error {
	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathAPIQueues)
	if err != nil {
		return fmt.Errorf("failed to create queues stats request: %w", err)
	}

	var resp []apiQueueResp

	if err := c.webClient().RequestJSON(req, &resp); err != nil {
		return err
	}

	for _, q := range resp {
		c.cache.getQueue(q).seen = true

		px := fmt.Sprintf("queue_%s_vhost_%s_node_%s_", q.Name, q.Vhost, q.Node)

		for k, v := range stm.ToMap(q) {
			mx[px+k] = v
		}

		// https://github.com/rabbitmq/rabbitmq-server/blob/8b554474a65857aa60b72b2dda4b6fa9b78f349b/deps/rabbitmq_management/priv/www/js/formatters.js#L552
		st := q.State
		if q.IdleSince != nil {
			st = "idle"
		}
		for _, v := range []string{"running", "idle", "terminated", "down", "crashed", "stopped", "minority"} {
			mx[px+"status_"+v] = metrix.Bool(v == st)
		}
	}

	return nil
}
