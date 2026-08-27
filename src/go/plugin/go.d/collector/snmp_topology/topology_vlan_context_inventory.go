// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strconv"
	"strings"
)

type topologyVLANContext struct {
	vlanID   string
	vlanName string
}

func (c *topologyBuilder) vtpVLANContexts() []topologyVLANContext {
	if !c.chargeWork(uint64(len(c.vlanNameByID))) {
		return nil
	}
	contexts := make([]topologyVLANContext, 0, len(c.vlanNameByID))
	for vlanID, mapping := range c.vlanNameByID {
		id := strings.TrimSpace(vlanID)
		if id == "" {
			continue
		}
		if _, err := strconv.Atoi(id); err != nil {
			continue
		}
		contexts = append(contexts, topologyVLANContext{
			vlanID:   id,
			vlanName: resolvedVLANName(mapping),
		})
	}

	sortTopologyVLANContextsWithBuilder(c, contexts)
	return contexts
}

func resolvedVLANName(mapping vlanNameMapping) string {
	if mapping.ambiguous {
		return ""
	}
	return strings.TrimSpace(mapping.name)
}

func sortTopologyVLANContexts(contexts []topologyVLANContext) {
	sortTopologyVLANContextsWithBuilder(&topologyBuilder{}, contexts)
}

func sortTopologyVLANContextsWithBuilder(builder *topologyBuilder, contexts []topologyVLANContext) {
	sortBuilderSlice(builder, contexts, func(i, j int) bool {
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
