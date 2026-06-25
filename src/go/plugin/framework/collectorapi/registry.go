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

		// Optional: FunctionProvider fields for exposing data functions
		// If Methods is non-nil, this module provides functions
		Methods func() []funcapi.MethodConfig

		// Optional: MethodHandler returns a handler for method requests.
		// MethodScopeAgent module methods are dispatched with nil job, except
		// that single-instance collectors receive their running canonical job.
		// MethodScopeInstance module methods and JobMethods are dispatched with
		// the selected running job.
		// When the canonical single-instance job is not running, dispatch returns
		// unavailable before calling MethodHandler.
		// The handler implements funcapi.MethodHandler interface with:
		// - MethodParams(ctx, method) for dynamic params
		// - Handle(ctx, method, params) for request handling
		// When nil, methods are disabled for this module.
		MethodHandler func(job RuntimeJob) funcapi.MethodHandler

		// Optional: JobMethods returns methods to register when a job starts.
		// Each method is registered as "moduleName:methodID" and unregistered when the job stops.
		// This enables per-job function registration instead of static module-level functions.
		// If nil, no per-job methods are registered.
		JobMethods func(job RuntimeJob) []funcapi.MethodConfig

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
	if creator.Methods != nil && creator.JobMethods != nil {
		panic(fmt.Sprintf("%s has both Methods and JobMethods defined (mutually exclusive)", name))
	}
	if creator.FunctionOnly && creator.Methods == nil && creator.JobMethods == nil {
		panic(fmt.Sprintf("%s is FunctionOnly but has no Methods or JobMethods defined", name))
	}
	r[name] = creator
}

func (r Registry) Lookup(name string) (Creator, bool) {
	v, ok := r[name]
	return v, ok
}
