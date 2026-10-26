// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import "time"

// Resource states have the discovery snapshot's validity, independently of Monitor
// query success. Inventory can outlive that validity; state observations cannot.
func (c *Collector) observeResourceStates(now time.Time) bool {
	if c.Discovery.RefreshEvery == 0 || !now.Before(c.discovery.ExpiresAt) {
		return false
	}

	observed := false
	for _, profile := range c.runtime.Profiles {
		if len(profile.ResourceStates) == 0 {
			continue
		}
		for _, resource := range c.discovery.ByProfile[profile.Name] {
			values := labelValues(profile.LabelKeys, profileResourceLabels(resource, profile))
			for _, state := range profile.ResourceStates {
				active, ok := state.Values[stringsLowerTrim(resource.Properties[state.Property])]
				if !ok {
					continue
				}
				state.Instrument.WithHostScope(resource.HostScope).WithLabelValues(values...).Enable(active)
				observed = true
			}
		}
	}
	return observed
}
