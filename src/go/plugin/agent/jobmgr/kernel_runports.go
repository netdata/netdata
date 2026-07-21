// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
)

type RunFinalizer interface {
	FinalizeRun(context.Context, uint64) error
}

// RunShutdownBarrier performs blocking external withdrawal before the
// loop-owned Function catalog begins close. It runs as one supervised shutdown
// task, never on KernelLoop.
type RunShutdownBarrier interface {
	BeforeFunctionCatalogClose(context.Context, uint64) error
}

type RunShutdownBarrierFunc func(context.Context, uint64) error

func (fn RunShutdownBarrierFunc) BeforeFunctionCatalogClose(
	ctx context.Context,
	generation uint64,
) error {
	return fn(ctx, generation)
}

type RunFinalizerFunc func(context.Context, uint64) error

func (fn RunFinalizerFunc) FinalizeRun(ctx context.Context, generation uint64) error {
	return fn(ctx, generation)
}
