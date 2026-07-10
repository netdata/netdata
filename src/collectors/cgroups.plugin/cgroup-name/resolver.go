// SPDX-License-Identifier: GPL-3.0-or-later

package main

import "context"

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
	config     invocationConfig
	logger     invocationLogger
	budget     invocationBudget
	tlsConfigs kubernetesTLSConfigCache
}

func newResolver(args []string, config invocationConfig) *resolver {
	return &resolver{
		config: config,
		logger: newInvocationLogger(args, config.logLevel),
		budget: invocationBudget{timeout: config.timeout},
	}
}

func (r *resolver) resolve(ctx context.Context, cgroupPath, cgroup string) resolution {
	if r.budgetExpired() {
		return r.deadlineRetry()
	}
	if isKubernetesCgroup(cgroup) {
		return r.resolveKubernetes(ctx, cgroupPath, cgroup)
	}

	result, ok := r.resolveNonKubernetes(ctx, cgroup)
	if !ok {
		if r.budgetExpired() {
			return r.deadlineRetry()
		}
		result = resolution{name: cgroup, exitCode: exitSuccess}
	} else if result.exitCode == exitRetry && r.budgetExpired() {
		return r.deadlineRetry()
	}
	if len(result.name) > 100 {
		result.name = result.name[:100]
	}
	return result
}

func (r *resolver) deadlineRetry() resolution {
	r.logCallBreakdown()
	return resolution{exitCode: exitRetry}
}
