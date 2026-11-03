// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
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
			Disabled:    true,
		},
		CreateV2:        func() collectorapi.CollectorV2 { return NewSocketCollector() },
		Config:          func() any { return &SocketConfig{} },
		SharedFunctions: socketMethods,
		MethodHandler:   socketFunctionHandler,
	})
}

type SocketConfig struct {
	Vnode       string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery int    `yaml:"update_every,omitempty" json:"update_every"`
	Enabled     bool   `yaml:"enabled" json:"enabled"`
	MapsPerCore bool   `yaml:"per_core_stats" json:"per_core_stats"`
}

type SocketCollector struct {
	collectorapi.Base
	Config      SocketConfig
	handle      *SocketLegacyHandle
	store       metrix.CollectorStore
	state       *socketGlobalState
	sharedStore *ebpfSharedMemoryStore

	// Function support (network-protocols)
	fnStore *socketFunctionStore
}

func NewSocketCollector() *SocketCollector {
	return &SocketCollector{
		Config: SocketConfig{
			Enabled:     false,
			UpdateEvery: socketDefaultUpdateEvery,
			MapsPerCore: true,
		},
		store:       metrix.NewCollectorStore(),
		state:       &socketGlobalState{},
		sharedStore: GetAppsIntegration().Store(),
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
		MapsPerCore: c.Config.MapsPerCore,
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
	_, err := c.handle.Runtime.Snapshot(c.Config.MapsPerCore)
	return err
}

func (c *SocketCollector) Collect(ctx context.Context) error {
	if c.handle == nil || c.handle.Runtime == nil {
		return errors.New("socket not initialized")
	}

	snapshot, err := c.handle.Runtime.Snapshot(c.Config.MapsPerCore)
	if err != nil {
		c.Infof("snapshot error: %v", err)
		return nil
	}

	// Compute deltas from the snapshot using global state
	publish, ok := c.state.Update(snapshot)
	if !ok {
		return nil
	}

	// Update function store with latest metrics for network-protocols function
	if c.fnStore != nil {
		c.fnStore.update(publish)
	}

	// Socket has no standalone charts; preserve its per-PID integration output.
	if c.sharedStore != nil {
		entries, err := c.handle.Runtime.SnapshotPerPID()
		if err != nil {
			c.sharedStore.MarkSocketInactive()
			return nil
		}
		c.sharedStore.UpdateSocketApps(entries, uint32(c.Config.UpdateEvery))
		if pub, err := GetAppsIntegration().PublisherSharedMemory(); err == nil {
			if err := c.sharedStore.Publish(pub, ebpfgoSHMFlagSocket); err != nil {
				c.Debugf("failed to publish socket SHM: %v", err)
			}
		}
	}

	return nil
}

func (c *SocketCollector) Cleanup(ctx context.Context) {
	if c.handle != nil {
		c.handle.Close()
	}
}

func (c *SocketCollector) ChartTemplateYAML() string {
	return ""
}

func (c *SocketCollector) MetricStore() metrix.CollectorStore {
	return c.store
}

func socketMethods() []funcapi.FunctionConfig {
	return []funcapi.FunctionConfig{{
		ID:           socketFunctionName,
		FunctionName: socketFunctionName,
		Name:         "Network protocols",
		UpdateEvery:  socketFunctionUpdateEvery,
		Help:         socketFunctionHelp,
		Tags:         socketFunctionTags,
		RawRequest:   true,
	}}
}

func socketFunctionHandler(job collectorapi.RuntimeJob) funcapi.MethodHandler {
	c, ok := job.Collector().(*SocketCollector)
	if !ok || c == nil {
		return nil
	}
	return socketMethodHandler{collector: c}
}

type socketMethodHandler struct{ collector *SocketCollector }

func (socketMethodHandler) MethodParams(context.Context, string) ([]funcapi.ParamConfig, error) {
	return nil, nil
}

func (socketMethodHandler) Handle(context.Context, string, funcapi.ResolvedParams) *funcapi.FunctionResponse {
	return funcapi.ErrorResponse(400, "network-protocols requires a raw Function request")
}

func (socketMethodHandler) Cleanup(context.Context) {}

func (h socketMethodHandler) HandleRaw(_ context.Context, _ funcapi.RawMethodRequest) *funcapi.FunctionResponse {
	payload, err := h.collector.handleNetworkProtocolsFunction()
	if err != nil {
		return funcapi.UnavailableResponse(err.Error())
	}
	var response map[string]any
	if err := json.Unmarshal([]byte(payload), &response); err != nil {
		return funcapi.InternalErrorResponse("invalid network-protocols response: %v", err)
	}
	return funcapi.RawResponse(response)
}

var _ funcapi.RawMethodHandler = socketMethodHandler{}

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
