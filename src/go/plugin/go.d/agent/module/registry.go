// SPDX-License-Identifier: GPL-3.0-or-later

package module

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

const (
	UpdateEvery        = 1
	AutoDetectionRetry = 0
	Priority           = 70000
)

// Defaults is a set of module default parameters.
type Defaults struct {
	UpdateEvery        int
	AutoDetectionRetry int
	Priority           int
	Disabled           bool
}

type (
	// Creator is a Job builder.
	// Optional function fields (Methods/MethodHandler) enable the FunctionProvider pattern:
	// modules that set these fields can expose data functions to the UI.
	Creator struct {
		Defaults
		Create          func() Module
		CreateV2        func() ModuleV2
		JobConfigSchema string
		Config          func() any

		// Optional: FunctionProvider fields for exposing data functions
		// If Methods is non-nil, this module provides functions
		Methods func() []funcapi.MethodConfig

		// Optional: MethodHandler returns a handler for method requests on a specific job.
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
