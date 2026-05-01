// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import "time"

func (c *Collector) applyGoBGPSummaries(client gobgpClientAPI, families []familyStats, selectedFamilies map[string]bool, scrape *scrapeMetrics) {
	refs := selectedGoBGPFamilyRefs(families, selectedFamilies)

	for _, ref := range refs {
		routes, err := client.GetTable(ref)
		if err != nil {
			scrape.noteQueryError(err, false)
			c.Debugf("collect GoBGP table summary for %s: %v", ref.ID, err)
			continue
		}
		for i := range families {
			if families[i].ID == ref.ID {
				families[i].RIBRoutes = int64(routes)
				break
			}
		}
	}

	if !c.CollectRIBSummaries {
		return
	}

	summaries := c.collectGoBGPValidationSummaries(client, refs, scrape)
	for i := range families {
		summary, ok := summaries[families[i].ID]
		if !ok || !summary.HasCorrectness {
			continue
		}
		families[i].HasCorrectness = true
		families[i].CorrectnessValid = summary.Valid
		families[i].CorrectnessInvalid = summary.Invalid
		families[i].CorrectnessNotFound = summary.NotFound
	}
}

func (c *Collector) collectGoBGPValidationSummaries(client gobgpClientAPI, refs []*gobgpFamilyRef, scrape *scrapeMetrics) map[string]gobgpValidationSummary {
	now := time.Now()
	if cached, ok := c.gobgpValidationCache.get(now, c.RIBSummaryEvery.Duration(), refs); ok {
		return cached
	}

	summaries := make(map[string]gobgpValidationSummary, len(refs))
	for _, ref := range refs {
		if !gobgpValidationSupported(ref) {
			continue
		}
		scrape.noteDeepQueryAttempt()
		summary, err := client.ListPathValidation(ref)
		if err != nil {
			scrape.noteDeepQueryError()
			c.Debugf("collect GoBGP validation summary for %s: %v", ref.ID, err)
			if cached, ok := c.gobgpValidationCache.getAny(refs); ok {
				return cached
			}
			return summaries
		}
		summaries[ref.ID] = summary
	}

	c.gobgpValidationCache.store(now, summaries)
	return summaries
}

func gobgpValidationSupported(ref *gobgpFamilyRef) bool {
	if ref == nil {
		return false
	}

	// Upstream GoBGP fills validation state only for global and Adj-RIB tables.
	// VRF ListPath requests return the routes, but the server leaves validation unset.
	return ref.VRF == "" || ref.VRF == "default"
}

func selectedGoBGPFamilyRefs(families []familyStats, selected map[string]bool) []*gobgpFamilyRef {
	seen := make(map[string]bool)
	var refs []*gobgpFamilyRef

	for _, family := range families {
		if !selected[family.ID] {
			continue
		}
		if seen[family.ID] {
			continue
		}
		ref, ok := gobgpFamilyRefFromStats(family)
		if !ok {
			continue
		}
		seen[family.ID] = true
		refs = append(refs, ref)
	}

	return refs
}

func (c *gobgpValidationCache) get(now time.Time, maxAge time.Duration, refs []*gobgpFamilyRef) (map[string]gobgpValidationSummary, bool) {
	if c.at.IsZero() || maxAge <= 0 {
		return nil, false
	}
	if now.Sub(c.at) < maxAge && c.covers(refs) {
		return c.summaries, true
	}
	return nil, false
}

func (c *gobgpValidationCache) getAny(refs []*gobgpFamilyRef) (map[string]gobgpValidationSummary, bool) {
	if c.at.IsZero() || !c.covers(refs) {
		return nil, false
	}
	return c.summaries, true
}

func (c *gobgpValidationCache) covers(refs []*gobgpFamilyRef) bool {
	if c.summaries == nil {
		return false
	}
	for _, ref := range refs {
		if !gobgpValidationSupported(ref) {
			continue
		}
		if _, ok := c.summaries[ref.ID]; !ok {
			return false
		}
	}
	return true
}

func (c *gobgpValidationCache) store(now time.Time, summaries map[string]gobgpValidationSummary) {
	c.at = now
	c.summaries = summaries
}
