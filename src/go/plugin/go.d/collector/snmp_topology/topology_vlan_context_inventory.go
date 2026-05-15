// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"sort"
	"strconv"
	"strings"
)

func (c *topologyCache) vtpVLANContexts() []topologyVLANContext {
	c.mu.RLock()
	defer c.mu.RUnlock()

	contexts := make([]topologyVLANContext, 0, len(c.vlanIDToName))
	for vlanID, vlanName := range c.vlanIDToName {
		id := strings.TrimSpace(vlanID)
		if id == "" {
			continue
		}
		if _, err := strconv.Atoi(id); err != nil {
			continue
		}
		contexts = append(contexts, topologyVLANContext{
			vlanID:   id,
			vlanName: strings.TrimSpace(vlanName),
		})
	}

	sortTopologyVLANContexts(contexts)
	return contexts
}

func sortTopologyVLANContexts(contexts []topologyVLANContext) {
	sort.Slice(contexts, func(i, j int) bool {
		left, leftErr := strconv.Atoi(contexts[i].vlanID)
		right, rightErr := strconv.Atoi(contexts[j].vlanID)
		if leftErr == nil && rightErr == nil && left != right {
			return left < right
		}
		if contexts[i].vlanID != contexts[j].vlanID {
			return contexts[i].vlanID < contexts[j].vlanID
		}
		return contexts[i].vlanName < contexts[j].vlanName
	})
}
