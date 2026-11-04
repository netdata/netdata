// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runner"
)

// Standalone administration owns an independent execution scope. In-process
// updates instead qualify through the existing monitoring engine.
func qualifyRuntime(ctx context.Context, source runner.Source) (*runner.Qualification, error) {
	return runner.New(source, nil, nil).Qualify(ctx, source)
}
