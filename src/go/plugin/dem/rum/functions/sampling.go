// SPDX-License-Identifier: GPL-3.0-or-later
package functions

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
)

// samplingLabels describes the site's sampling for the Sites table.
// Rates describe the current policy, not observed or historical proportions.
func samplingLabels(s query.Sampling) (measured, investigated string) {
	measured = percent(s.MeasureRate) + " of new browser sessions"
	if s.MeasureRate == 0 {
		measured = "off (0%)"
	}
	investigated = percent(s.InvestigateRate) + " baseline of measured sessions"
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

// samplingNote names current configuration separately from historical evidence.
func samplingNote(site query.Site) string {
	s := site.Sampling
	measured, investigated := samplingLabels(s)
	return fmt.Sprintf("%s: collection %s; retained/exported detail %s", site.Label, measured, investigated)
}

func percent(rate float64) string {
	// Significant digits suppress multiplication noise without rounding tiny rates to zero.
	return strconv.FormatFloat(rate*100, 'g', 15, 64) + "%"
}
