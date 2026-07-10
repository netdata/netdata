// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"sync"
)

const (
	exitSuccess = 0
	exitFatal   = 1
	exitRetry   = 2
	exitDisable = 3
)

type resolution struct {
	name     string
	labels   labelSet
	exitCode int
}

type resolver struct {
	config      invocationConfig
	logger      invocationLogger
	budget      invocationBudget
	tlsWarnOnce sync.Once
}

func newResolver(args []string, config invocationConfig) *resolver {
	return &resolver{
		config: config,
		logger: newInvocationLogger(args, config.logLevel),
		budget: invocationBudget{timeout: config.timeout},
	}
}

func (r *resolver) resolve(ctx context.Context, cgroupPath, cgroup string) resolution {
	if isKubernetesCgroup(cgroup) {
		return r.resolveKubernetes(ctx, cgroupPath, cgroup)
	}

	result, ok := r.resolveNonKubernetes(ctx, cgroup)
	if !ok {
		if r.budgetExpired() {
			r.logCallBreakdown()
			return resolution{exitCode: exitRetry}
		}
		result = resolution{name: cgroup, exitCode: exitSuccess}
	}
	if len(result.name) > 100 {
		result.name = result.name[:100]
	}
	return result
}
