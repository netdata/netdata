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

	st, ok := c.state.cronJobs[r.source()]
	if !ok {
		st = newCronJobState()
		c.state.cronJobs[r.source()] = st

		st.uid = string(cj.UID)
		st.name = cj.Name
		st.namespace = cj.Namespace
		st.creationTime = cj.CreationTimestamp.Time
	}

	st.suspend = false
	if cj.Spec.Suspend != nil {
		st.suspend = *cj.Spec.Suspend
	}

	st.lastScheduleTime = nil
	st.lastSuccessfulTime = nil

	if cj.Status.LastScheduleTime != nil {
		st.lastScheduleTime = ptr(cj.Status.LastScheduleTime.Time)
	}
	if cj.Status.LastSuccessfulTime != nil {
		st.lastSuccessfulTime = ptr(cj.Status.LastSuccessfulTime.Time)
	}
}
