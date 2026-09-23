// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"time"
)

const (
	// chartIDCollisionLogKey rate-limits collision warnings per job logger.
	chartIDCollisionLogKey    = "chartengine:chart-id-collision"
	chartIDCollisionLogPeriod = time.Hour
)

// chartIDCollisions counts authored series routes one build rejected because
// another template owns their rendered chart ID, keeping the first as an example.
type chartIDCollisions struct {
	routes   int
	chartID  string
	owner    string
	rejected string
}

func (c *chartIDCollisions) add(chartID, ownerTemplateID, rejectedTemplateID string) {
	if c.routes == 0 {
		c.chartID, c.owner, c.rejected = chartID, ownerTemplateID, rejectedTemplateID
	}
	c.routes++
}

// warnChartIDCollisions reports a successful build's rejected authored routes.
// The owner keeps the chart; the warning names both templates so the author
// can give each chart a distinct ID.
func (e *Engine) warnChartIDCollisions(ctx *planBuildContext) {
	c := ctx.collisions
	if c.routes == 0 || e == nil || e.state.log == nil {
		return
	}
	e.state.log.Limit(chartIDCollisionLogKey, 1, chartIDCollisionLogPeriod).Warningf(
		"chartengine: dropped %d series routes to charts owned by another template (chart '%s' owned by %s, rejected %s); give each chart a unique id or context",
		c.routes,
		c.chartID,
		describeChartTemplate(ctx.index, c.owner),
		describeChartTemplate(ctx.index, c.rejected),
	)
}

func describeChartTemplate(index matchIndex, templateID string) string {
	if chart, ok := index.chartsByID[templateID]; ok && chart.EntryID != "" {
		return fmt.Sprintf("entry '%s' chart %s", chart.EntryID, chart.LocalTemplateID)
	}
	return "template " + templateID
}
