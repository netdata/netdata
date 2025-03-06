// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_state

func (c *Collector) updateCronJobState(r resource) {
	if r.value() == nil {
		if rs, ok := c.state.cronJobs[r.source()]; ok {
			rs.deleted = true
		}
		return
	}

	cj, err := toCronJob(r)
	if err != nil {
		c.Warning(err)
		return
	}

	_, ok := c.state.cronJobs[r.source()]
	if !ok {
		st := newCronJobState()
		c.state.cronJobs[r.source()] = st

		st.uid = string(cj.UID)
		st.name = cj.Name
		st.namespace = cj.Namespace
		st.creationTime = cj.CreationTimestamp.Time
	}
}
