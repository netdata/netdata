// SPDX-License-Identifier: GPL-3.0-or-later

package module

import "fmt"

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
	Creator struct {
		Defaults
		Create          func() Module
		JobConfigSchema string
		Config          func() any
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
