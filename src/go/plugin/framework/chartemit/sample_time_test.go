// SPDX-License-Identifier: GPL-3.0-or-later

package chartemit

import (
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
)

// testSample is the collection time the tests pass to ApplyPlan; testEnd is the
// END line it renders.
var testSample = func() netdataapi.SampleTime {
	var clock netdataapi.SampleClock
	return clock.Next(time.Unix(1700000000, 250000000))
}()

const testEnd = "END 1700000000 250000 1"
