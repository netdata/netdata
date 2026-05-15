// SPDX-License-Identifier: GPL-3.0-or-later

package secretresolver

import (
	"context"
	"time"
)

// Resolver resolves secret references in config maps.
// In v2 it only owns local builtin resolvers and the optional secretstore callback.
type Resolver struct {
	cmdTimeout time.Duration

	providers map[string]func(context.Context, string, string) (string, error)
}

// New creates a resolver with local builtin provider defaults.
func New() *Resolver {
	r := &Resolver{}
	r.ensureDefaults()
	return r
}

func (r *Resolver) ensureDefaults() {
	if r.cmdTimeout <= 0 {
		r.cmdTimeout = 10 * time.Second
	}
	if r.providers == nil {
		r.providers = map[string]func(context.Context, string, string) (string, error){
			"env":  r.resolveEnv,
			"file": r.resolveFile,
			"cmd":  r.resolveCmd,
		}
	}
}
