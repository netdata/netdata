// SPDX-License-Identifier: GPL-3.0-or-later

package collectorapi

import "sync"

var obsoleteLock = &sync.Mutex{}
var obsoleteCharts = true

func ObsoleteCharts(b bool) {
	obsoleteLock.Lock()
	obsoleteCharts = b
	obsoleteLock.Unlock()
}

func ShouldObsoleteCharts() bool {
	obsoleteLock.Lock()
	defer obsoleteLock.Unlock()
	return obsoleteCharts
}
