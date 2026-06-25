// SPDX-License-Identifier: GPL-3.0-or-later

package collectorapi

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
)

const (
	UpdateEvery        = 1
	AutoDetectionRetry = 0
	Priority           = chartengine.Priority
)

// Defaults is a set of module default parameters.
type Defaults struct {
	UpdateEvery        int
	AutoDetectionRetry int
	Priority           int
	Disabled           bool
}

// InstancePolicy controls how many runtime instances a collector module may
// have in a single go.d process.
type InstancePolicy int

const (
	// InstancePolicyPerJob preserves the existing model: every accepted job
	// config may create its own runtime collector instance.
	InstancePolicyPerJob InstancePolicy = iota

	// InstancePolicySingle allows exactly one canonical runtime collector
	// instance per module. Configs for single-instance collectors must resolve
	// to name == module after defaults are applied; dyncfg exposes them as a
	// module-level single config, not as a template plus jobs.
	InstancePolicySingle
)

func (p InstancePolicy) valid() bool {
	switch p {
	case InstancePolicyPerJob, InstancePolicySingle:
		return true
	default:
		return false
	}
}

type (
	// Creator is a Job builder.
	// Optional function fields (Methods/MethodHandler) enable the FunctionProvider pattern:
	// modules that set these fields can expose data functions to the UI.
	Creator struct {
		Defaults
		Create          func() CollectorV1
		CreateV2        func() CollectorV2
		JobConfigSchema string
		Config          func() any

		// InstancePolicy defaults to InstancePolicyPerJob when omitted.
		InstancePolicy InstancePolicy

		// Optional: SharedFunctions declares static job-backed Functions shared
		// by all jobs of this module. InstancePolicy controls whether they are
		// selected through __job or routed to the canonical single instance.
		SharedFunctions func() []funcapi.FunctionConfig

		// Optional: AgentFunctions declares static process-backed Functions that
		// do not depend on collector jobs and do not use __job.
		AgentFunctions func() []funcapi.FunctionConfig

		// Optional: MethodHandler returns a handler for method requests.
		// AgentFunctions are dispatched with nil job. SharedFunctions are
		// dispatched with the selected running job; for single-instance
		// collectors they receive the running canonical job.
		// When the canonical single-instance job is not running, dispatch returns
		// unavailable before calling MethodHandler.
		// The handler implements funcapi.MethodHandler interface with:
		// - MethodParams(ctx, method) for dynamic params
		// - Handle(ctx, method, params) for request handling
		// When nil, methods are disabled for this module.
		MethodHandler func(job RuntimeJob) funcapi.MethodHandler

		// Optional: InstanceFunctions returns Function declarations owned by one
		// runtime job. Each returned Function ID is published as "moduleName:functionID"
		// while the owning job is running and available. If nil, no instance Functions
		// are declared. A collector can implement FunctionAvailability to gate
		// publication for each returned Function ID.
		InstanceFunctions func(job RuntimeJob) []funcapi.FunctionConfig

		// FunctionOnly indicates this module provides only functions, no metrics.
		// Jobs created from this module skip data collection and chart creation.
		// The module must still implement Init() and Check() for connectivity validation.
		FunctionOnly bool
	}
	// Registry is a collection of Creators.
	Registry map[string]Creator
)

// DefaultRegistry DefaultRegistry.
var DefaultRegistry = Registry{}

// Register registers a module in the DefaultRegistry.
func Register(name string, creator Creator) {
	DefaultRegistry.Register(name, creator)
}

// Register registers a module.
func (r Registry) Register(name string, creator Creator) {
	if _, ok := r[name]; ok {
		panic(fmt.Sprintf("%s is already in registry", name))
	}
	if !creator.InstancePolicy.valid() {
		panic(fmt.Sprintf("%s has invalid InstancePolicy %d", name, creator.InstancePolicy))
	}
	if (creator.SharedFunctions != nil || creator.AgentFunctions != nil) && creator.InstanceFunctions != nil {
		panic(fmt.Sprintf("%s has both static Functions and InstanceFunctions defined (mutually exclusive)", name))
	}
	if id, ok := duplicateStaticFunctionID(creator); ok {
		panic(fmt.Sprintf("%s has duplicate static Function ID %q", name, id))
	}
	if creator.FunctionOnly && creator.SharedFunctions == nil && creator.AgentFunctions == nil && creator.InstanceFunctions == nil {
		panic(fmt.Sprintf("%s is FunctionOnly but has no Functions or InstanceFunctions defined", name))
	}
	r[name] = creator
}

func duplicateStaticFunctionID(creator Creator) (string, bool) {
	seen := make(map[string]struct{})
	for _, fn := range staticFunctions(creator) {
		if fn.ID == "" {
			continue
		}
		if _, ok := seen[fn.ID]; ok {
			return fn.ID, true
		}
		seen[fn.ID] = struct{}{}
	}
	return "", false
}

func staticFunctions(creator Creator) []funcapi.FunctionConfig {
	var functions []funcapi.FunctionConfig
	if creator.SharedFunctions != nil {
		functions = append(functions, creator.SharedFunctions()...)
	}
	if creator.AgentFunctions != nil {
		functions = append(functions, creator.AgentFunctions()...)
	}
	return functions
}

func (r Registry) Lookup(name string) (Creator, bool) {
	v, ok := r[name]
	return v, ok
}
