// SPDX-License-Identifier: GPL-3.0-or-later
package functions

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
)

// samplingLabels describes the site's sampling for the Sites table,
// e.g. "25% of sessions" and "10% of measured + errors, poor vitals".
func samplingLabels(s query.Sampling) (measured, investigated string) {
	measured = percent(s.MeasureRate) + " of sessions"
	investigated = percent(s.InvestigateRate) + " of measured"
	if s.InvestigateRate < 1 {
		var keep []string
		if s.KeepErrors {
			keep = append(keep, "errors")
		}
		if s.KeepPoorVitals {
			keep = append(keep, "poor vitals")
		}
		if len(keep) > 0 {
			investigated += " + " + strings.Join(keep, ", ")
		}
	}
	return measured, investigated
}

// samplingNote explains, for the Sessions and Errors tables, why they hold
// fewer sessions than the charts count; "" when nothing is sampled.
func samplingNote(site query.Site) string {
	s := site.Sampling
	if s.MeasureRate >= 1 && s.InvestigateRate >= 1 {
		return ""
	}
	measured, investigated := samplingLabels(s)
	return fmt.Sprintf("%s: measuring %s, keeping %s in full", site.Label, measured, investigated)
}

func percent(rate float64) string {
	// Significant digits suppress multiplication noise without rounding tiny rates to zero.
	return strconv.FormatFloat(rate*100, 'g', 15, 64) + "%"
}
