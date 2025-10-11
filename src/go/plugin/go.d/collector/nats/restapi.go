// SPDX-License-Identifier: GPL-3.0-or-later

package nats

import (
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/web"
)

// https://docs.nats.io/running-a-nats-service/nats_admin/monitoring

const (
	// https://docs.nats.io/running-a-nats-service/nats_admin/monitoring#general-information
	urlPathVarz = "/varz"
	// https://docs.nats.io/running-a-nats-service/nats_admin/monitoring#health
	urlPathHealthz = "/healthz"
	// https://docs.nats.io/running-a-nats-service/nats_admin/monitoring#account-statistics
	urlPathAccstatz = "/accstatz"
	// https://docs.nats.io/running-a-nats-service/nats_admin/monitoring#route-information
	urlPathRoutez = "/routez"
	// https://docs.nats.io/running-a-nats-service/nats_admin/monitoring#gateway-information
	urlPathGatewayz = "/gatewayz"
	// https://docs.nats.io/running-a-nats-service/nats_admin/monitoring#leaf-node-information
	urlPathLeafz = "/leafz"
	// https://docs.nats.io/running-a-nats-service/nats_admin/monitoring#jetstream-information
	urlPathJsz = "/jsz"
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
	Uptime           string            `json:"uptime"`
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
type (
	accstatzResponse struct {
		AccStats []accountInfo `json:"account_statz"`
	}
	accountInfo struct {
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
	}
)

// https://github.com/nats-io/nats-server/blob/v2.10.24/server/monitor.go#L752
type (
	routezResponse struct {
		Routes []routeInfo `json:"routes"`
	}
	routeInfo struct {
		Rid      uint64 `json:"rid"`
		RemoteID string `json:"remote_id"`
		InMsgs   int64  `json:"in_msgs"`
		OutMsgs  int64  `json:"out_msgs"`
		InBytes  int64  `json:"in_bytes"`
		OutBytes int64  `json:"out_bytes"`
		NumSubs  uint32 `json:"subscriptions"`
	}
)

type (
	// https://github.com/nats-io/nats-server/blob/v2.10.24/server/monitor.go#L1875
	gatewayzResponse struct {
		Name             string                          `json:"name"`
		OutboundGateways map[string]*remoteGatewayInfo   `json:"outbound_gateways"`
		InboundGateways  map[string][]*remoteGatewayInfo `json:"inbound_gateways"`
	}
	remoteGatewayInfo struct {
		IsConfigured bool           `json:"configured"`
		Connection   connectionInfo `json:"connection"`
	}
	connectionInfo struct {
		Cid      uint64 `json:"cid"`
		Uptime   string `json:"uptime"`
		InMsgs   int64  `json:"in_msgs"`
		OutMsgs  int64  `json:"out_msgs"`
		InBytes  int64  `json:"in_bytes"`
		OutBytes int64  `json:"out_bytes"`
		NumSubs  uint32 `json:"subscriptions"`
	}
)

type (
	// https://github.com/nats-io/nats-server/blob/v2.10.24/server/monitor.go#L2163
	leafzResponse struct {
		Leafs []leafInfo `json:"leafs"`
	}
	leafInfo struct {
		Name     string `json:"name"` // remote server name or id
		Account  string `json:"account"`
		IP       string `json:"ip"`
		Port     int    `json:"port"`
		RTT      string `json:"rtt,omitempty"`
		InMsgs   int64  `json:"in_msgs"`
		OutMsgs  int64  `json:"out_msgs"`
		InBytes  int64  `json:"in_bytes"`
		OutBytes int64  `json:"out_bytes"`
		NumSubs  uint32 `json:"subscriptions"`
	}
)

// https://github.com/nats-io/nats-server/blob/v2.10.24/server/monitor.go#L2801
type (
	jszResponse struct {
		Disabled       bool   `json:"disabled"`
		Streams        int    `json:"streams"`
		Consumers      int    `json:"consumers"`
		Messages       uint64 `json:"messages"`
		Bytes          uint64 `json:"bytes"`
		Memory         uint64 `json:"memory"`
		Store          uint64 `json:"storage"`
		ReservedMemory uint64 `json:"reserved_memory"`
		ReservedStore  uint64 `json:"reserved_storage"`
		Accounts       int    `json:"accounts"`
		HAAssets       int    `json:"ha_assets"`
		Api            struct {
			Total    uint64 `json:"total"`
			Errors   uint64 `json:"errors"`
			Inflight uint64 `json:"inflight"`
		} `json:"api"`
		Meta *jszMetaClusterInfo `json:"meta_cluster"`
	}
	jszMetaClusterInfo struct {
		Name     string         `json:"name"`
		Leader   string         `json:"leader"`
		Peer     string         `json:"peer"`
		Replicas []*jszPeerInfo `json:"replicas"`
		Size     int            `json:"cluster_size"`
		Pending  int            `json:"pending"`
	}
	jszPeerInfo struct {
		Name    string        `json:"name"`
		Current bool          `json:"current"`
		Offline bool          `json:"offline"`
		Active  time.Duration `json:"active"`
		Lag     uint64        `json:"lag"`
		Peer    string        `json:"peer"`
	}
)
