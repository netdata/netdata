// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"strings"
)

// Count after routing has resolved ownership and aggregation, before lifecycle
// caps can truncate a candidate. Existing observedCount already deduplicates pairs.
func (ctx *planBuildContext) enforceContextLimit(limit int) map[string]int {
	if limit == 0 {
		return nil
	}
	counts := make(map[string]int)
	for _, chart := range ctx.chartsByID {
		if chart.observedCount > 0 {
			counts[chart.meta.Context] += chart.observedCount
		}
	}
	for id, chart := range ctx.chartsByID {
		count := counts[chart.meta.Context]
		if count <= limit {
			continue
		}
		// Scan borrowed this cache before admission. Detach it transactionally so
		// rejected identities cannot accumulate on a quiet published chart.
		// A different candidate owner may have displaced the chart that borrowed
		// this cache earlier in the scan; detach the published owner's cache too.
		if owner := ctx.materializedByID[id]; owner != nil && owner.scratchEntries != nil {
			ctx.journal.touchChart(owner)
			owner.scratchEntries = nil
			owner.scratchDeletes = 0
		}
		delete(ctx.chartsByID, id)
		ctx.observeRouteDiagnostic(PlanRouteDiagnostic{
			Decision:           PlanRouteContextRejected,
			Reason:             PlanRouteReasonContextSeriesLimit,
			ChartTemplateID:    chart.templateID,
			ChartID:            id,
			Context:            chart.meta.Context,
			ContextSeriesCount: count,
			ContextSeriesLimit: limit,
		})
	}
	return counts
}

const contextLimitLogExamples = 3
const contextLimitLogNameBytes = 120

// Fixed-size sorted examples keep logs deterministic and bounded without sorting
// or retaining all transitioning context names.
type contextLimitChanges struct {
	count  int
	names  [contextLimitLogExamples]string
	series [contextLimitLogExamples]int
	used   int
}

func (c *contextLimitChanges) add(name string, count int) {
	c.count++
	i := 0
	for i < c.used && c.names[i] < name {
		i++
	}
	if i == len(c.names) {
		return
	}
	for j := min(c.used, len(c.names)-1); j > i; j-- {
		c.names[j], c.series[j] = c.names[j-1], c.series[j-1]
	}
	c.names[i], c.series[i] = name, count
	c.used = min(c.used+1, len(c.names))
}

func (c *contextLimitChanges) examples() string {
	var out strings.Builder
	for i := 0; i < c.used; i++ {
		if i > 0 {
			out.WriteString(", ")
		}
		name := c.names[i]
		if len(name) > contextLimitLogNameBytes {
			name = name[:contextLimitLogNameBytes] + "..."
		}
		fmt.Fprintf(&out, "%q (%d)", name, c.series[i])
	}
	return out.String()
}

// Called under the engine lock only after successful publication commit. Missing
// contexts leave the reporting population without being called recovered.
func (e *Engine) commitContextLimits(counts map[string]int, reset bool) {
	previous := e.state.outputContextCounts
	e.state.outputContextCounts = counts
	if reset {
		previous = nil
	}
	limit := e.state.cfg.maxOutputSeriesPerContext
	if limit == 0 {
		return
	}
	var omitted, recovered contextLimitChanges
	for context, count := range counts {
		wasOmitted := previous[context] > limit
		if count > limit && !wasOmitted {
			omitted.add(context, count)
		} else if count <= limit && wasOmitted {
			recovered.add(context, count)
		}
	}
	if omitted.count == 0 && recovered.count == 0 {
		return
	}
	const message = "chartengine output limit per context=%d: newly omitted=%d [%s]; no longer omitted=%d [%s] (examples show output series counts)"
	if omitted.count > 0 {
		e.logWarningf(message, limit, omitted.count, omitted.examples(), recovered.count, recovered.examples())
	} else {
		e.logInfof(message, limit, omitted.count, omitted.examples(), recovered.count, recovered.examples())
	}
}
