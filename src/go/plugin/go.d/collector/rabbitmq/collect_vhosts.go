// SPDX-License-Identifier: GPL-3.0-or-later

package rabbitmq

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/stm"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"
)

func (c *Collector) collectVhosts(mx map[string]int64) error {
	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathAPIVhosts)
	if err != nil {
		return fmt.Errorf("failed to create vhosts stats request: %w", err)
	}

	var resp []apiVhostResp

	if err := c.webClient().RequestJSON(req, &resp); err != nil {
		return err
	}

	for _, vhost := range resp {
		c.cache.getVhost(vhost.Name).seen = true

		px := fmt.Sprintf("vhost_%s_", vhost.Name)

		for k, v := range stm.ToMap(vhost) {
			mx[px+k] = v
		}

		st := getVhostStatus(vhost)
		for _, v := range []string{"running", "stopped", "partial"} {
			mx[px+"status_"+v] = metrix.Bool(v == st)
		}
	}

	return nil
}

func getVhostStatus(vhost apiVhostResp) string {
	// https://github.com/rabbitmq/rabbitmq-server/blob/c8394095990c2eb9e2f4b142e7816a653c9e5011/deps/rabbitmq_management/priv/www/js/formatters.js#L1058

	var ok, nok int

	for _, v := range vhost.ClusterState {
		switch v {
		case "stopped", "nodedown":
			nok++
		case "running":
			ok++
		}
	}

	switch {
	case nok == 0:
		return "running"
	case ok == 0:
		return "stopped"
	default:
		return "partial"
	}
}
