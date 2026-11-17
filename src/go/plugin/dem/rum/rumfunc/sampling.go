// SPDX-License-Identifier: GPL-3.0-or-later

package rumfunc

import (
	"context"
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

// samplingCaveat ends the help of the methods that read saved sessions: sampling
// decides what history holds, so it is not a population count, and a session
// promoted by a later problem replays its earlier events into past ranges.
const samplingCaveat = "Saved history may follow an earlier sampling policy and be incomplete, so it does not " +
	"estimate all visitors or errors. A session kept after a later problem also saves its earlier events, so a " +
	"past range can grow."

// sampled marks a method that reads saved sessions: its help ends with the
// sampling caveat, followed at request time by the current sampling policy.
func sampled(m method) method {
	m.Help += " " + samplingCaveat
	m.samplingHelp = true
	return m
}

// samplingHelp follows the method help with the current sampling policy of the
// selected sites.
func (h *Handler) samplingHelp(ctx context.Context, help string, args map[string]string) (string, error) {
	sites, err := h.deps.Sites(ctx, args["site"], args["vnode_guid"])
	if err != nil {
		return "", err
	}
	var notes []string
	problemBiased := false
	for _, site := range sites {
		notes = append(notes, samplingNote(site))
		sampling := site.Sampling
		if sampling.InvestigateRate < 1 && (sampling.KeepErrors || sampling.KeepPoorVitals) {
			problemBiased = true
		}
	}
	if len(notes) > 0 {
		help += " Current sampling policy: " + strings.Join(notes, "; ") + "."
		if problemBiased {
			help += " Sessions with the listed problems are kept beyond the baseline, so they are over-represented."
		}
	}
	return help, nil
}
