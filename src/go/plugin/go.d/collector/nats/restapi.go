// SPDX-License-Identifier: GPL-3.0-or-later

package nats

import (
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

// https://docs.nats.io/running-a-nats-service/nats_admin/monitoring

const (
	// https://docs.nats.io/running-a-nats-service/nats_admin/monitoring#general-information
	urlPathVarz = "/varz"
	// https://docs.nats.io/running-a-nats-service/nats_admin/monitoring#health
	urlPathHealthz = "/healthz"
	// https://docs.nats.io/running-a-nats-service/nats_admin/monitoring#account-statistics
	urlPathAccstatz = "/accstatz"
)

var (
	urlQueryHealthzJsEnabledOnly = web.URLQuery("js-enabled-only", "true")
	urlQueryHealthzJsServerOnly  = web.URLQuery("js-server-only", "true")
	urlQueryAccstatz             = web.URLQuery("unused", "1")
)

// //https://github.com/nats-io/nats-server/blob/v2.10.24/server/server.go#L2851
var httpEndpoints = []string{
	"/",
	"/varz",
	"/connz",
	"/routez",
	"/gatewayz",
	"/leafz",
	"/subsz",
	"/stacksz",
	"/accountz",
	"/accstatz",
	"/jsz",
	"/healthz",
	"/ipqueuesz",
	"/raftz",
}

// https://github.com/nats-io/nats-server/blob/v2.10.24/server/monitor.go#L3125
type healthzResponse struct {
	Status *string `json:"status"`
}

// https://github.com/nats-io/nats-server/blob/v2.10.24/server/monitor.go#L1164
type varzResponse struct {
	ID               string            `json:"server_id"`
	Name             string            `json:"server_name"`
	Version          string            `json:"version"`
	Proto            int               `json:"proto"`
	Host             string            `json:"host"`
	Port             int               `json:"port"`
	IP               string            `json:"ip,omitempty"`
	MaxConn          int               `json:"max_connections"`
	MaxSubs          int               `json:"max_subscriptions,omitempty"`
	Start            time.Time         `json:"start"`
	Now              time.Time         `json:"now"`
	Mem              int64             `json:"mem"`
	CPU              float64           `json:"cpu"`
	Connections      int               `json:"connections"`
	TotalConnections uint64            `json:"total_connections"`
	Routes           int               `json:"routes"`
	Remotes          int               `json:"remotes"`
	Leafs            int               `json:"leafnodes"`
	InMsgs           int64             `json:"in_msgs"`
	OutMsgs          int64             `json:"out_msgs"`
	InBytes          int64             `json:"in_bytes"`
	OutBytes         int64             `json:"out_bytes"`
	SlowConsumers    int64             `json:"slow_consumers"`
	Subscriptions    uint32            `json:"subscriptions"`
	HTTPReqStats     map[string]uint64 `json:"http_req_stats"`
}

// https://github.com/nats-io/nats-server/blob/v2.10.24/server/monitor.go#L2279
type accstatzResponse struct {
	AccStats []struct {
		Account    string `json:"acc"`
		Conns      int    `json:"conns"`
		TotalConns int    `json:"total_conns"`
		LeafNodes  int    `json:"leafnodes"`
		NumSubs    uint32 `json:"num_subscriptions"`
		Sent       struct {
			Msgs  int64 `json:"msgs"`
			Bytes int64 `json:"bytes"`
		} `json:"sent"`
		Received struct {
			Msgs  int64 `json:"msgs"`
			Bytes int64 `json:"bytes"`
		} `json:"received"`
		SlowConsumers int64 `json:"slow_consumers"`
	} `json:"account_statz"`
}
