// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/topology/worklimit"
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
		name := resolvedVLANName(mapping)
		if c.workLimiter != nil {
			bytes, err := worklimit.StringBytes(vlanID, mapping.name)
			if err != nil || !c.chargeWork(bytes) {
				if err != nil {
					c.workErr = err
				}
				return nil
			}
		}
		id := strings.TrimSpace(vlanID)
		if id == "" {
			continue
		}
		_, err := strconv.Atoi(id)
		if err != nil {
			continue
		}
		contexts = append(contexts, topologyVLANContext{vlanID: id, vlanName: name})
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
	if builder.workLimiter == nil {
		sortBuilderSliceWithStringWork(builder, contexts, 0, func(i, j int) bool {
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
		return
	}

	maxStringBytes, err := worklimit.ChargeStringValues(builder.workLimiter, contexts, func(value topologyVLANContext) (uint64, error) {
		return worklimit.StringBytes(value.vlanID, value.vlanName)
	})
	if err != nil {
		builder.workErr = err
		return
	}
	if !builder.chargeWork(uint64(len(contexts))) {
		return
	}
	type entry struct {
		context topologyVLANContext
		number  int
	}
	entries := make([]entry, len(contexts))
	for i, context := range contexts {
		number, _ := strconv.Atoi(context.vlanID)
		entries[i] = entry{context: context, number: number}
	}
	if !sortBuilderSliceWithStringWork(builder, entries, maxStringBytes, func(i, j int) bool {
		if entries[i].number != entries[j].number {
			return entries[i].number < entries[j].number
		}
		if entries[i].context.vlanID != entries[j].context.vlanID {
			return entries[i].context.vlanID < entries[j].context.vlanID
		}
		return entries[i].context.vlanName < entries[j].context.vlanName
	}) {
		return
	}
	if !builder.chargeWork(uint64(len(contexts))) {
		return
	}
	for i := range entries {
		contexts[i] = entries[i].context
	}
}
