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
// task, never on the kernel loop.
type RunShutdownBarrier interface {
	BeforeFunctionCatalogClose(context.Context, uint64) error
}
