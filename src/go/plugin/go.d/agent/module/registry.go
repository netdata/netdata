// SPDX-License-Identifier: GPL-3.0-or-later

package module

import (
	"context"
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

// Type aliases for backward compatibility during migration.
// These types are now defined in funcapi package.
type (
	MethodConfig     = funcapi.MethodConfig
	FunctionResponse = funcapi.FunctionResponse
	ChartConfig      = funcapi.ChartConfig
	GroupByConfig    = funcapi.GroupByConfig
)

type (
	// Creator is a Job builder.
	// Optional function fields (Methods/HandleMethod) enable the FunctionProvider pattern:
	// modules that set these fields can expose data functions to the UI.
	Creator struct {
		Defaults
		Create          func() Module
		JobConfigSchema string
		Config          func() any

		// Optional: FunctionProvider fields for exposing data functions
		// If Methods is non-nil, this module provides functions
		Methods func() []MethodConfig

		// Optional: MethodParams returns dynamic required params for a job+method.
		// Use this to provide job-specific options (e.g., based on DB capabilities).
		// When nil, MethodConfig.RequiredParams is used as-is.
		MethodParams func(ctx context.Context, job *Job, method string) ([]funcapi.ParamConfig, error)

		// HandleMethod handles a function request for a specific job
		// ctx: context with timeout from function request
		// job: the job instance to query
		// method: the method name (e.g., "top-queries")
		// params: resolved required params (includes __sort)
		// Returns: FunctionResponse with data or error
		HandleMethod func(ctx context.Context, job *Job, method string, params funcapi.ResolvedParams) *FunctionResponse
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
