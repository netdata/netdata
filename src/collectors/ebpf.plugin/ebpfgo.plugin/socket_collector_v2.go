// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed "charts/socket.yaml"
var socketChartTemplateV2 string

//go:embed "config_schemas/socket_schema.json"
var socketConfigSchema string

func init() {
	collectorapi.Register("socket", collectorapi.Creator{
		JobConfigSchema: socketConfigSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: 1,
		},
		CreateV2: func() collectorapi.CollectorV2 { return NewSocketCollector() },
		Config:   func() any { return &SocketConfig{} },
	})
}

type SocketConfig struct {
	Vnode       string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery int    `yaml:"update_every,omitempty" json:"update_every"`
	Enabled     bool   `yaml:"enabled" json:"enabled"`
}

type SocketCollector struct {
	collectorapi.Base
	Config    SocketConfig
	handle    *SocketLegacyHandle
	publisher *PublisherService
	state     *socketGlobalState

	// Function support (network-protocols)
	fnStore *socketFunctionStore
}

func NewSocketCollector() *SocketCollector {
	return &SocketCollector{
		Config: SocketConfig{
			Enabled:     true,
			UpdateEvery: socketDefaultUpdateEvery,
		},
		publisher: GetPublisher(),
		state:     &socketGlobalState{},
	}
}

func (c *SocketCollector) Configuration() any {
	return c.Config
}

func (c *SocketCollector) Init(ctx context.Context) error {
	legacyCfg, err := resolveSocketLegacyConfig()
	if err == nil && legacyCfg.Enabled {
		c.Config.Enabled = true
	}

	if !c.Config.Enabled {
		return errors.New("socket disabled in configuration")
	}

	handle, err := LoadSocketLegacy(SocketLegacyConfig{
		Enabled:     c.Config.Enabled,
		UpdateEvery: c.Config.UpdateEvery,
	})
	if err != nil {
		return fmt.Errorf("failed to load socket: %v", err)
	}

	if handle == nil || handle.Runtime == nil {
		return errors.New("socket runtime initialization failed")
	}

	c.handle = handle

	// Initialize function store for network-protocols function
	c.fnStore = newSocketFunctionStore(c.Config.UpdateEvery)

	return nil
}

func (c *SocketCollector) Check(ctx context.Context) error {
	if c.handle == nil || c.handle.Runtime == nil {
		return errors.New("socket not initialized")
	}
	_, err := c.handle.Runtime.Snapshot()
	return err
}

func (c *SocketCollector) Collect(ctx context.Context) error {
	if c.handle == nil || c.handle.Runtime == nil {
		return errors.New("socket not initialized")
	}

	snapshot, err := c.handle.Runtime.Snapshot()
	if err != nil {
		c.Infof("snapshot error: %v", err)
		return nil
	}

	// Compute deltas from the snapshot using global state
	publish, ok := c.state.Update(snapshot)
	if !ok {
		return nil
	}

	// Write all metrics to the publisher store
	meter := c.publisher.MetricStore().Write().SnapshotMeter("")
	meter.Counter("tcp_cleanup_rbuf").ObserveTotal(float64(publish.tcpDimReceivedCalls))
	meter.Counter("tcp_cleanup_rbuf_err").ObserveTotal(float64(publish.tcpDimReceivedErr))
	meter.Counter("tcp_sendmsg").ObserveTotal(float64(publish.tcpDimSentCalls))
	meter.Counter("tcp_sendmsg_err").ObserveTotal(float64(publish.tcpDimSentErr))
	meter.Counter("tcp_close").ObserveTotal(float64(publish.tcpCloseCalls))
	meter.Counter("tcp_retransmit").ObserveTotal(float64(publish.tcpRetransmit))
	meter.Counter("tcp_connect_v4").ObserveTotal(float64(publish.tcpV4Conn))
	meter.Counter("tcp_connect_v6").ObserveTotal(float64(publish.tcpV6Conn))
	meter.Counter("udp_recvmsg").ObserveTotal(float64(publish.udpRecvCalls))
	meter.Counter("udp_sendmsg").ObserveTotal(float64(publish.udpSendCalls))
	meter.Counter("udp_recvmsg_err").ObserveTotal(float64(publish.udpRecvErr))
	meter.Counter("udp_sendmsg_err").ObserveTotal(float64(publish.udpSendErr))
	meter.Counter("inbound_tcp").ObserveTotal(float64(publish.inboundTCP))
	meter.Counter("inbound_udp").ObserveTotal(float64(publish.inboundUDP))
	meter.Counter("tcp_bytes_sent").ObserveTotal(float64(publish.tcpBytesSent))
	meter.Counter("tcp_bytes_received").ObserveTotal(float64(publish.tcpBytesReceived))
	meter.Counter("udp_bytes_sent").ObserveTotal(float64(publish.udpBytesSent))
	meter.Counter("udp_bytes_received").ObserveTotal(float64(publish.udpBytesReceived))

	// Update function store with latest metrics for network-protocols function
	if c.fnStore != nil {
		c.fnStore.update(publish)
	}

	return nil
}

func (c *SocketCollector) Cleanup(ctx context.Context) {
	if c.handle != nil {
		c.handle.Close()
	}
}

func (c *SocketCollector) ChartTemplateYAML() string {
	return socketChartTemplateV2
}

func (c *SocketCollector) MetricStore() metrix.CollectorStore {
	return c.publisher.MetricStore()
}

// Function support: network-protocols
// These methods enable the socket collector to serve network-protocols function calls
// through the agent's function routing system.

// handleNetworkProtocolsFunction serves the network-protocols function request.
// Returns a JSON table response with TCP/UDP socket statistics.
func (c *SocketCollector) handleNetworkProtocolsFunction() (string, error) {
	if c.fnStore == nil {
		return "", fmt.Errorf("function store not initialized")
	}

	publish, hasData := c.fnStore.snapshot()
	if !hasData {
		return "", fmt.Errorf("no data available yet")
	}

	// Build the proper network-protocols JSON table response
	now := time.Now().Unix()
	expires := now + int64(c.Config.UpdateEvery)

	payload, err := buildNetworkProtocolsJSON(publish, c.Config.UpdateEvery, expires)
	if err != nil {
		return "", fmt.Errorf("failed to marshal response: %v", err)
	}

	return payload, nil
}
