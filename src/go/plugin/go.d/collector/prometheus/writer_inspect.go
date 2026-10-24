// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"fmt"
	"slices"
	"strings"

	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
)

// WriterEligibility is a read-only application of the initialized writer's
// family/type/schema/value policy and current cached family contracts.
type WriterEligibility struct {
	Family         string
	WritableSeries int
}

// InspectWriterEligibility evaluates families without creating, reconciling,
// or refreshing handles and without writing samples. It is intended for
// diagnostic counterfactuals such as explaining selector exclusions.
func (c *Collector) InspectWriterEligibility(mfs prompkg.MetricFamilies) ([]WriterEligibility, error) {
	if c == nil || c.writer == nil {
		return nil, fmt.Errorf("prometheus writer is not initialized")
	}
	result := make([]WriterEligibility, 0, len(mfs))
	const allowUntypedFallback = true
	for _, mf := range mfs {
		item := WriterEligibility{Family: mf.Name()}
		if reason := c.writer.metricFamilySkipReason(mf); reason != "" {
			result = append(result, item)
			continue
		}
		typ, ok := c.writer.resolveFamilyType(mf, allowUntypedFallback)
		if !ok {
			result = append(result, item)
			continue
		}
		schema, reason := c.writer.inspectMetricFamilySchema(mf, typ, allowUntypedFallback)
		if reason != "" {
			result = append(result, item)
			continue
		}
		for _, metric := range mf.Metrics() {
			if reason := metricSchemaRejectionReason(metric, typ, schema, allowUntypedFallback); reason == "" {
				item.WritableSeries++
			}
		}
		result = append(result, item)
	}
	slices.SortFunc(result, func(a, b WriterEligibility) int { return strings.Compare(a.Family, b.Family) })
	return result, nil
}
