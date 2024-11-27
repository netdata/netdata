// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_state

func (c *Collector) updateNodeState(r resource) {
	if r.value() == nil {
		if ns, ok := c.state.nodes[r.source()]; ok {
			ns.deleted = true
		}
		return
	}

	node, err := toNode(r)
	if err != nil {
		c.Warning(err)
		return
	}

	if myNodeName != "" && node.Name != myNodeName {
		return
	}

	ns, ok := c.state.nodes[r.source()]
	if !ok {
		ns = newNodeState()
		c.state.nodes[r.source()] = ns
	}

	if !ok {
		ns.name = node.Name
		ns.creationTime = node.CreationTimestamp.Time
		ns.allocatableCPU = int64(node.Status.Allocatable.Cpu().AsApproximateFloat64() * 1000)
		ns.allocatableMem = node.Status.Allocatable.Memory().Value()
		ns.allocatablePods = node.Status.Allocatable.Pods().Value()
		copyLabels(ns.labels, node.Labels)
	}

	ns.unSchedulable = node.Spec.Unschedulable
	ns.conditions = node.Status.Conditions
}
