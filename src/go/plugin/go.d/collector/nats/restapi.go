// SPDX-License-Identifier: GPL-3.0-or-later

package nats

import (
	"time"
)

// https://docs.nats.io/running-a-nats-service/nats_admin/monitoring

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
	PingInterval     time.Duration     `json:"ping_interval"`
	MaxPingsOut      int               `json:"ping_max"`
	HTTPHost         string            `json:"http_host"`
	HTTPPort         int               `json:"http_port"`
	HTTPBasePath     string            `json:"http_base_path"`
	HTTPSPort        int               `json:"https_port"`
	AuthTimeout      float64           `json:"auth_timeout"`
	MaxControlLine   int32             `json:"max_control_line"`
	MaxPayload       int               `json:"max_payload"`
	MaxPending       int64             `json:"max_pending"`
	TLSTimeout       float64           `json:"tls_timeout"`
	WriteDeadline    time.Duration     `json:"write_deadline"`
	Start            time.Time         `json:"start"`
	Now              time.Time         `json:"now"`
	Uptime           string            `json:"uptime"`
	Mem              int64             `json:"mem"`
	Cores            int               `json:"cores"`
	MaxProcs         int               `json:"gomaxprocs"`
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
