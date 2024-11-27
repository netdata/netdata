// SPDX-License-Identifier: GPL-3.0-or-later

package consul

import (
	"net/http"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"
)

const (
	// https://developer.hashicorp.com/consul/api-docs/operator/autopilot#read-health
	urlPathOperationAutopilotHealth = "/v1/operator/autopilot/health"
)

type autopilotHealth struct {
	Servers []struct {
		ID          string
		SerfStatus  string
		Leader      bool
		LastContact string
		Healthy     bool
		Voter       bool
		StableSince time.Time
	}
}

func (c *Collector) collectAutopilotHealth(mx map[string]int64) error {
	req, err := c.createRequest(urlPathOperationAutopilotHealth)
	if err != nil {
		return err
	}

	var health autopilotHealth

	// The HTTP status code will indicate the health of the cluster: 200 is healthy, 429 is unhealthy.
	// https://github.com/hashicorp/consul/blob/c7ef04c5979dbc311ff3c67b7bf3028a93e8b0f1/agent/operator_endpoint.go#L325
	if err := c.client(http.StatusTooManyRequests).RequestJSON(req, &health); err != nil {
		return err
	}

	for _, srv := range health.Servers {
		if srv.ID == c.cfg.Config.NodeID {
			// SerfStatus: alive, left, failed or none:
			// https://github.com/hashicorp/consul/blob/c7ef04c5979dbc311ff3c67b7bf3028a93e8b0f1/agent/consul/operator_autopilot_endpoint.go#L124-L133
			mx["autopilot_server_sefStatus_alive"] = metrix.Bool(srv.SerfStatus == "alive")
			mx["autopilot_server_sefStatus_left"] = metrix.Bool(srv.SerfStatus == "left")
			mx["autopilot_server_sefStatus_failed"] = metrix.Bool(srv.SerfStatus == "failed")
			mx["autopilot_server_sefStatus_none"] = metrix.Bool(srv.SerfStatus == "none")
			// https://github.com/hashicorp/raft-autopilot/blob/d936f51c374c3b7902d5e4fdafe9f7d8d199ea53/types.go#L110
			mx["autopilot_server_healthy_yes"] = metrix.Bool(srv.Healthy)
			mx["autopilot_server_healthy_no"] = metrix.Bool(!srv.Healthy)
			mx["autopilot_server_voter_yes"] = metrix.Bool(srv.Voter)
			mx["autopilot_server_voter_no"] = metrix.Bool(!srv.Voter)
			mx["autopilot_server_stable_time"] = int64(time.Since(srv.StableSince).Seconds())
			mx["autopilot_server_stable_time"] = int64(time.Since(srv.StableSince).Seconds())
			if !srv.Leader {
				if v, err := time.ParseDuration(srv.LastContact); err == nil {
					mx["autopilot_server_lastContact_leader"] = v.Milliseconds()
				}
			}

			break
		}
	}

	return nil
}
