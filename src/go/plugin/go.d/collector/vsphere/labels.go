// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"fmt"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

const (
	vmGuestLabelHostName = "guest_hostname"
	vmGuestLabelIP       = "guest_ip"
	vmGuestLabelOS       = "guest_os"
)

var validVMGuestLabels = []string{
	vmGuestLabelHostName,
	vmGuestLabelIP,
	vmGuestLabelOS,
}

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
	if c.InventoryPathLabel {
		if path := c.inventoryPath(resourceID); path != "" {
			labels["inventory_path"] = path
		}
	}
	for key, value := range c.userMetadataLabels(resourceID) {
		if key != "" && value != "" {
			labels[key] = value
		}
	}
	if vm := c.resources.VMs.Get(resourceID); vm != nil {
		for _, label := range c.VMGuestLabels {
			switch label {
			case vmGuestLabelHostName:
				if vm.GuestHostName != "" {
					labels[vmGuestLabelHostName] = vm.GuestHostName
				}
			case vmGuestLabelIP:
				if vm.GuestIPAddress != "" {
					labels[vmGuestLabelIP] = vm.GuestIPAddress
				}
			case vmGuestLabelOS:
				if vm.GuestFullName != "" {
					labels[vmGuestLabelOS] = vm.GuestFullName
				}
			}
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

func (c *Collector) inventoryPath(resourceID string) string {
	if c.resources == nil {
		return ""
	}
	if vm := c.resources.VMs.Get(resourceID); vm != nil {
		return inventoryPathFromParts(vm.Hier.DC.Name, vm.Hier.Cluster.Name, vm.Hier.Host.Name, vm.Name)
	}
	if host := c.resources.Hosts.Get(resourceID); host != nil {
		return inventoryPathFromParts(host.Hier.DC.Name, host.Hier.Cluster.Name, host.Name)
	}
	if ds := c.resources.Datastores.Get(resourceID); ds != nil {
		return inventoryPathFromParts(ds.Hier.DC.Name, ds.Name)
	}
	if cluster := c.resources.Clusters.Get(resourceID); cluster != nil {
		return inventoryPathFromParts(cluster.Hier.DC.Name, cluster.Name)
	}
	if rp := c.resources.ResourcePools.Get(resourceID); rp != nil {
		return inventoryPathFromParts(rp.Hier.DC.Name, rp.Hier.Cluster.Name, rp.Name)
	}
	return ""
}

func inventoryPathFromParts(parts ...string) string {
	var clean []string
	for _, part := range parts {
		part = strings.Trim(part, "/ ")
		if part == "" {
			continue
		}
		clean = append(clean, part)
	}
	if len(clean) == 0 {
		return ""
	}
	return "/" + strings.Join(clean, "/")
}

func validateStringAllowlist(name string, values, allowed []string) error {
	allowedSet := make(map[string]bool, len(allowed))
	for _, value := range allowed {
		allowedSet[value] = true
	}

	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if !allowedSet[value] {
			return fmt.Errorf("%s has invalid value %q (valid: %s)", name, value, strings.Join(allowed, ", "))
		}
		if seen[value] {
			return fmt.Errorf("%s has duplicate value %q", name, value)
		}
		seen[value] = true
	}
	return nil
}
