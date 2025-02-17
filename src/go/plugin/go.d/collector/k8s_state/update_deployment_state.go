// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_state

func (c *Collector) updateDeploymentState(r resource) {
	if r.value() == nil {
		if rs, ok := c.state.deployments[r.source()]; ok {
			rs.deleted = true
		}
		return
	}

	deploy, err := toDeployment(r)
	if err != nil {
		c.Warning(err)
		return
	}

	ds, ok := c.state.deployments[r.source()]
	if !ok {
		ds = newDeploymentState()
		c.state.deployments[r.source()] = ds
	}

	if !ok {
		ds.name = deploy.Name
		ds.namespace = deploy.Namespace
		ds.uid = string(deploy.UID)
		ds.creationTime = deploy.CreationTimestamp.Time
	}

	ds.replicas = int64(deploy.Status.Replicas)
	ds.availableReplicas = int64(deploy.Status.AvailableReplicas)
	ds.readyReplicas = int64(deploy.Status.ReadyReplicas)
}
