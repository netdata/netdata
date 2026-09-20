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
//   - provide exactly one chart capability for metric jobs: static YAML or a native set.
type CollectorV2 interface {
	Init(context.Context) error
	Check(context.Context) error
	Collect(context.Context) error
	Cleanup(context.Context)

	GetBase() *Base
	Configuration() any
	VirtualNode() *vnodes.VirtualNode

	MetricStore() metrix.CollectorStore
}

// StaticChartTemplateProvider supplies one document, captured once after Check.
// Existing collectors may continue composing or embedding YAML through this API.
type StaticChartTemplateProvider interface {
	ChartTemplateYAML() string
}

// ChartTemplateSetProvider supplies the complete desired native set. The getter
// is captured after Check and once after each successful Collect, before metric
// commit. It must be cheap and return the same immutable pointer until content
// changes. Nil and the zero value are invalid. For no authored entries, construct
// an empty set with chartengine.NewTemplateSet(chartengine.TemplateSetSpec{}).
// Calls run on the job goroutine, serialized with Collect. If other goroutines
// share the stored pointer across replacements, synchronize its reads and writes.
type ChartTemplateSetProvider interface {
	ChartTemplateSet() *chartengine.TemplateSet
}

// CollectorV2Runner is an optional long-running V2 collector hook.
//
// Run is called only after the runtime job starts. It is not called during
// config validation or autodetection. Implementations should block until ctx is
// canceled, and must return promptly after ctx.Done(). Returning before
// cancellation is treated as an unexpected runner stop and is logged by the job
// runtime.
type CollectorV2Runner interface {
	Run(context.Context) error
}

// ConfiguredVnodeConsumer is an optional CollectorV1 or CollectorV2 capability. It receives an
// owned snapshot before Init/Check and,
// when configuration changes, synchronously before Collect on the job goroutine.
// Consumers must not treat this as a source of generated host identity.
type ConfiguredVnodeConsumer interface {
	SetConfiguredVnode(vnodes.VirtualNode)
}

// FunctionAvailability lets a running collector instance decide which
// job-backed Functions it can currently serve.
//
// Implementations must be cheap and non-blocking. When a collector does not
// implement this interface, every running job is available for every
// job-backed Function it declares or shares.
type FunctionAvailability interface {
	FunctionAvailable(functionID string) bool
}

// CollectorV2EnginePolicy allows a V2 collector to provide chartengine policy
// (series selector + autogen behavior), captured once after Check. Overrides
// apply to global policy; native entry restrictions remain in force.
type CollectorV2EnginePolicy interface {
	EnginePolicy() chartengine.EnginePolicy
}

// Base is a helper struct. All modules should embed this struct.
type Base struct {
	*logger.Logger
}

func (b *Base) GetBase() *Base { return b }

func (b *Base) VirtualNode() *vnodes.VirtualNode { return nil }
