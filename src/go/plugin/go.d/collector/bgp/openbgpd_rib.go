// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"bytes"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
)

type openbgpdRIBCache struct {
	collectedAt time.Time
	families    map[string]openbgpdRIBSummary
	requested   []string
}

type openbgpdRIBSummary struct {
	RIBRoutes int64
	Valid     int64
	Invalid   int64
	NotFound  int64
}

type openbgpdRIBResponse struct {
	RIB []openbgpdRoute `json:"rib"`
}

type openbgpdRoute struct {
	Prefix string `json:"prefix"`
	OVS    string `json:"ovs"`
}

func (c *Collector) collectOpenBGPDRIBSummaries(client openbgpdClientAPI, families []familyStats, selectedFamilies map[string]bool, scrape *scrapeMetrics) map[string]openbgpdRIBSummary {
	if !c.CollectRIBSummaries {
		return nil
	}

	requested := selectedOpenBGPDRIBFamilyIDs(selectOpenBGPDRIBFamilies(families, selectedFamilies))
	if len(requested) == 0 {
		return nil
	}

	if cached, ok := c.openbgpdRIBCache.getIfFresh(c.RIBSummaryEvery.Duration(), requested); ok {
		return cached
	}

	summaries, err := c.collectOpenBGPDRequestedRIBSummaries(client, requested, scrape)
	if err != nil {
		if cached, ok := c.openbgpdRIBCache.get(requested); ok {
			return cached
		}
		return nil
	}

	c.openbgpdRIBCache.set(requested, summaries)
	return summaries
}

func (c *Collector) collectOpenBGPDRequestedRIBSummaries(client openbgpdClientAPI, requested []string, scrape *scrapeMetrics) (map[string]openbgpdRIBSummary, error) {
	summaries, err := c.collectOpenBGPDFilteredRIBSummaries(client, requested, scrape)
	if err == nil {
		return summaries, nil
	}
	if !errors.Is(err, errOpenBGPDFilteredRIBUnsupported) {
		if len(summaries) > 0 {
			return summaries, nil
		}
		return nil, err
	}

	data, err := client.RIB()
	if err != nil {
		scrape.noteQueryError(err, false)
		return nil, err
	}

	summaries, err = parseOpenBGPDRIBSummaries(data)
	if err != nil {
		scrape.noteParseError(false)
		return nil, err
	}

	return filterOpenBGPDRIBSummaries(summaries, requested), nil
}

func (c *Collector) collectOpenBGPDFilteredRIBSummaries(client openbgpdClientAPI, requested []string, scrape *scrapeMetrics) (map[string]openbgpdRIBSummary, error) {
	summaries := make(map[string]openbgpdRIBSummary, len(requested))

	for _, familyID := range requested {
		filter, ok := openbgpdRIBFilterForFamilyID(familyID)
		if !ok {
			continue
		}

		data, err := client.RIBFiltered(filter)
		if err != nil {
			if errors.Is(err, errOpenBGPDFilteredRIBUnsupported) {
				return nil, err
			}
			scrape.noteQueryError(err, false)
			return nil, err
		}

		summary, err := parseOpenBGPDRIBSummary(data)
		if err != nil {
			scrape.noteParseError(false)
			return nil, err
		}
		summaries[familyID] = summary
	}

	return summaries, nil
}

func parseOpenBGPDRIBSummaries(data []byte) (map[string]openbgpdRIBSummary, error) {
	if len(bytes.TrimSpace(data)) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("{}")) {
		return nil, nil
	}

	var resp openbgpdRIBResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	summaries := make(map[string]openbgpdRIBSummary)
	for _, route := range resp.RIB {
		afi, safi, ok := classifyOpenBGPDRouteFamily(route.Prefix)
		if !ok {
			continue
		}

		id := makeFamilyID("default", afi, safi)
		summary := summaries[id]
		summary.RIBRoutes++

		switch normalizeOpenBGPDOVS(route.OVS) {
		case "valid":
			summary.Valid++
		case "invalid":
			summary.Invalid++
		case "not_found":
			summary.NotFound++
		}

		summaries[id] = summary
	}

	return summaries, nil
}

func parseOpenBGPDRIBSummary(data []byte) (openbgpdRIBSummary, error) {
	if len(bytes.TrimSpace(data)) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("{}")) {
		return openbgpdRIBSummary{}, nil
	}

	var resp openbgpdRIBResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return openbgpdRIBSummary{}, err
	}

	var summary openbgpdRIBSummary
	for _, route := range resp.RIB {
		summary.RIBRoutes++

		switch normalizeOpenBGPDOVS(route.OVS) {
		case "valid":
			summary.Valid++
		case "invalid":
			summary.Invalid++
		case "not_found":
			summary.NotFound++
		}
	}

	return summary, nil
}

func classifyOpenBGPDRouteFamily(prefix string) (afi, safi string, ok bool) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "", "", false
	}

	if strings.HasPrefix(prefix, "rd ") {
		fields := strings.Fields(prefix)
		if len(fields) == 0 {
			return "", "", false
		}
		last := fields[len(fields)-1]
		if strings.Contains(last, ":") {
			return "ipv6", "vpn", true
		}
		return "ipv4", "vpn", true
	}

	if strings.HasPrefix(prefix, "[2]:") || strings.HasPrefix(prefix, "[3]:") || strings.HasPrefix(prefix, "[5]:") {
		return "l2vpn", "evpn", true
	}

	if strings.Contains(prefix, ":") {
		return "ipv6", "unicast", true
	}
	return "ipv4", "unicast", true
}

func normalizeOpenBGPDOVS(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "valid":
		return "valid"
	case "invalid":
		return "invalid"
	case "not-found", "not_found":
		return "not_found"
	default:
		return ""
	}
}

func applyOpenBGPDRIBSummaries(families []familyStats, summaries map[string]openbgpdRIBSummary) {
	if len(summaries) == 0 {
		return
	}

	for i := range families {
		summary, ok := summaries[families[i].ID]
		if !ok {
			continue
		}
		families[i].RIBRoutes = summary.RIBRoutes
		families[i].HasCorrectness = true
		families[i].CorrectnessValid = summary.Valid
		families[i].CorrectnessInvalid = summary.Invalid
		families[i].CorrectnessNotFound = summary.NotFound
	}
}

func selectOpenBGPDRIBFamilies(families []familyStats, selectedFamilies map[string]bool) []familyStats {
	if len(families) == 0 || len(selectedFamilies) == 0 {
		return nil
	}

	requested := make([]familyStats, 0, len(families))
	for _, family := range families {
		if !selectedFamilies[family.ID] {
			continue
		}
		if !openbgpdRIBSummarySupported(family) {
			continue
		}
		requested = append(requested, family)
	}
	return requested
}

func selectedOpenBGPDRIBFamilyIDs(families []familyStats) []string {
	if len(families) == 0 {
		return nil
	}

	requested := make([]string, 0, len(families))
	for _, family := range families {
		requested = append(requested, family.ID)
	}
	sort.Strings(requested)
	return requested
}

func filterOpenBGPDRIBSummaries(src map[string]openbgpdRIBSummary, requested []string) map[string]openbgpdRIBSummary {
	if len(src) == 0 || len(requested) == 0 {
		return nil
	}

	dst := make(map[string]openbgpdRIBSummary)
	for _, familyID := range requested {
		if summary, ok := src[familyID]; ok {
			dst[familyID] = summary
		}
	}
	if len(dst) == 0 {
		return nil
	}
	return dst
}

func openbgpdRIBFilterForFamilyID(familyID string) (string, bool) {
	switch familyID {
	case "default_ipv4_unicast":
		return "ipv4", true
	case "default_ipv6_unicast":
		return "ipv6", true
	case "default_ipv4_vpn":
		return "vpnv4", true
	case "default_ipv6_vpn":
		return "vpnv6", true
	default:
		return "", false
	}
}

func (c *openbgpdRIBCache) getIfFresh(ttl time.Duration, requested []string) (map[string]openbgpdRIBSummary, bool) {
	if ttl <= 0 {
		return nil, false
	}
	if c.collectedAt.IsZero() || time.Since(c.collectedAt) >= ttl {
		return nil, false
	}
	return c.get(requested)
}

func (c *openbgpdRIBCache) get(requested []string) (map[string]openbgpdRIBSummary, bool) {
	if c.collectedAt.IsZero() {
		return nil, false
	}
	if !equalStringSlices(c.requested, requested) {
		return nil, false
	}
	return cloneOpenBGPDRIBSummaries(c.families), true
}

func (c *openbgpdRIBCache) set(requested []string, summaries map[string]openbgpdRIBSummary) {
	c.collectedAt = time.Now()
	c.requested = cloneStringSlice(requested)
	c.families = cloneOpenBGPDRIBSummaries(summaries)
}

func cloneOpenBGPDRIBSummaries(src map[string]openbgpdRIBSummary) map[string]openbgpdRIBSummary {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]openbgpdRIBSummary, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func cloneStringSlice(src []string) []string {
	if len(src) == 0 {
		return nil
	}
	dst := make([]string, len(src))
	copy(dst, src)
	return dst
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
