// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"fmt"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

type userMetadataPatternTerm struct {
	matcher  matcher.Matcher
	positive bool
}

type userMetadataPatternMatcher []userMetadataPatternTerm

func newUserMetadataPatternMatcher(name string, patterns []string) (matcher.Matcher, error) {
	var terms userMetadataPatternMatcher
	hasPositive := false
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		term := userMetadataPatternTerm{positive: true}
		if pattern[0] == '!' {
			term.positive = false
			pattern = strings.TrimSpace(pattern[1:])
		}
		if pattern == "" {
			return nil, fmt.Errorf("%s has invalid empty negative pattern", name)
		}
		hasPositive = hasPositive || term.positive
		m, err := matcher.NewGlobMatcher(pattern)
		if err != nil {
			return nil, fmt.Errorf("%s has invalid pattern: %w", name, err)
		}
		term.matcher = m
		terms = append(terms, term)
	}
	if len(terms) == 0 {
		return matcher.FALSE(), nil
	}
	if !hasPositive {
		return nil, fmt.Errorf("%s must include at least one positive pattern", name)
	}
	return terms, nil
}

func (m userMetadataPatternMatcher) Match(b []byte) bool {
	return m.MatchString(string(b))
}

func (m userMetadataPatternMatcher) MatchString(s string) bool {
	for _, term := range m {
		if term.matcher.MatchString(s) {
			return term.positive
		}
	}
	return false
}

func (c *Collector) resourceEnrichmentLabels(resourceID string) []metrix.Label {
	if c.resources == nil {
		return nil
	}

	labels := make(map[string]string)
	for key, value := range c.userMetadataLabels(resourceID) {
		if key != "" && value != "" {
			labels[key] = value
		}
	}

	if len(labels) == 0 {
		return nil
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]metrix.Label, 0, len(keys))
	for _, key := range keys {
		out = append(out, metrix.Label{Key: key, Value: labels[key]})
	}
	return out
}

func (c *Collector) userMetadataLabels(resourceID string) map[string]string {
	if c.resources == nil {
		return nil
	}
	if vm := c.resources.VMs.Get(resourceID); vm != nil {
		return vm.Labels
	}
	if host := c.resources.Hosts.Get(resourceID); host != nil {
		return host.Labels
	}
	if ds := c.resources.Datastores.Get(resourceID); ds != nil {
		return ds.Labels
	}
	if cluster := c.resources.Clusters.Get(resourceID); cluster != nil {
		return cluster.Labels
	}
	if rp := c.resources.ResourcePools.Get(resourceID); rp != nil {
		return rp.Labels
	}
	if sp := c.resources.StoragePods.Get(resourceID); sp != nil {
		return sp.Labels
	}
	return nil
}
