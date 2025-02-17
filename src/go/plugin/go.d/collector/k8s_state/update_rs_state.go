// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_state

func (c *Collector) updateReplicaSetState(r resource) {
	if r.value() == nil {
		if rs, ok := c.state.replicasets[r.source()]; ok {
			rs.deleted = true
		}
		return
	}

	replicaset, err := toReplicaset(r)
	if err != nil {
		c.Warning(err)
		return
	}

	rs, ok := c.state.replicasets[r.source()]
	if !ok {
		rs = newReplicasetState()
		c.state.replicasets[r.source()] = rs
	}

	if !ok {
		rs.name = replicaset.Name
		rs.namespace = replicaset.Namespace
		rs.uid = string(replicaset.UID)
		for _, ref := range replicaset.OwnerReferences {
			if ref.Controller != nil && *ref.Controller {
				rs.controllerKind = ref.Kind
				rs.controllerName = ref.Name
			}
		}

		rs.creationTime = replicaset.CreationTimestamp.Time
		rs.replicas = int64(replicaset.Status.Replicas)
		rs.availableReplicas = int64(replicaset.Status.AvailableReplicas)
		rs.readyReplicas = int64(replicaset.Status.ReadyReplicas)
	}
}
