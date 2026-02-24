// SPDX-License-Identifier: GPL-3.0-or-later

package collectorapi

import (
	"context"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

// CollectorV1 is an interface that represents a module.
type CollectorV1 interface {
	// Init does initialization.
	// If it returns error, the job will be disabled.
	Init(context.Context) error

	// Check is called after Init.
	// If it returns error, the job will be disabled.
	Check(context.Context) error

	// Charts returns the chart definition.
	Charts() *Charts

	// Collect collects metrics.
	Collect(context.Context) map[string]int64

	// Cleanup Cleanup
	Cleanup(context.Context)

	GetBase() *Base

	Configuration() any

	VirtualNode() *vnodes.VirtualNode
}

// CollectorV2 is the collector contract for the new metrics+template runtime.
//
// Collectors implementing this interface:
//   - write metrics into CollectorStore during Collect(),
//   - provide chart template YAML consumed by chartengine.
type CollectorV2 interface {
	Init(context.Context) error
	Check(context.Context) error
	Collect(context.Context) error
	Cleanup(context.Context)

	GetBase() *Base
	Configuration() any
	VirtualNode() *vnodes.VirtualNode

	MetricStore() metrix.CollectorStore
	ChartTemplateYAML() string
}

// CollectorV2EnginePolicy allows a V2 collector to provide chartengine policy
// (series selector + autogen behavior).
type CollectorV2EnginePolicy interface {
	EnginePolicy() chartengine.EnginePolicy
}

// Base is a helper struct. All modules should embed this struct.
type Base struct {
	*logger.Logger
}

func (b *Base) GetBase() *Base { return b }

func (b *Base) VirtualNode() *vnodes.VirtualNode { return nil }
