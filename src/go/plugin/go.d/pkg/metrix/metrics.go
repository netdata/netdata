// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import "github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"

// Observer is an interface that wraps the Observe method, which is used by
// Histogram and Summary to add observations.
type Observer interface {
	stm.Value
	Observe(v float64)
}

func Bool(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
