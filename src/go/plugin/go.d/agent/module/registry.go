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
		MethodHandler func(job *Job) funcapi.MethodHandler
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
	r[name] = creator
}

func (r Registry) Lookup(name string) (Creator, bool) {
	v, ok := r[name]
	return v, ok
}
