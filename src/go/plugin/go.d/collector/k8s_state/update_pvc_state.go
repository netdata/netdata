// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_state

import corev1 "k8s.io/api/core/v1"

func (c *Collector) updatePVCState(r resource) {
	if r.value() == nil {
		if ps, ok := c.state.pvcs[r.source()]; ok {
			ps.deleted = true
		}
		return
	}

	pvc, err := toPVC(r)
	if err != nil {
		c.Warning(err)
		return
	}

	ps, ok := c.state.pvcs[r.source()]
	if !ok {
		ps = newPVCState()
		c.state.pvcs[r.source()] = ps

		ps.name = pvc.Name
		ps.namespace = pvc.Namespace
	}

	// The PVC was created again before the next collection removed the entry. Keep the entry and its chart: a new
	// entry would add the same chart ID again, which is rejected while the chart exists.
	ps.deleted = false

	ps.requested, ps.hasRequested = storageBytes(pvc.Spec.Resources.Requests)
	ps.capacity, ps.hasCapacity = storageBytes(pvc.Status.Capacity)
}

// storageBytes returns the storage quantity in bytes, and false if the list has no storage quantity.
func storageBytes(rl corev1.ResourceList) (int64, bool) {
	q, ok := rl[corev1.ResourceStorage]
	return q.Value(), ok
}
