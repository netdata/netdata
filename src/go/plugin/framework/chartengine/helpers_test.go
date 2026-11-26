// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import "github.com/netdata/netdata/go/plugins/pkg/metrix"

func buildPlan(engine *Engine, reader metrix.Reader) (Plan, error) {
	return prepareAndCommitPlan(engine, reader)
}
