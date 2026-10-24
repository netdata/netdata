// SPDX-License-Identifier: GPL-3.0-or-later

package l2topology

import "github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/pipeline"

// BuildL2ResultFromObservations converts normalized L2 observations into a
// deterministic L2 topology result. Callers that need a stable timestamp should
// set DiscoverOptions.CollectedAt explicitly.
func BuildL2ResultFromObservations(observations []L2Observation, opts DiscoverOptions) (Result, error) {
	return pipeline.BuildL2ResultFromObservations(observations, opts)
}
