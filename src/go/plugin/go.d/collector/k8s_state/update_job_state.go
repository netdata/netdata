// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_state

import (
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *Collector) updateJobState(r resource) {
	if r.value() == nil {
		if rs, ok := c.state.jobs[r.source()]; ok {
			rs.deleted = true
		}
		return
	}

	job, err := toJob(r)
	if err != nil {
		c.Warning(err)
		return
	}

	if !slices.ContainsFunc(job.OwnerReferences, func(ref metav1.OwnerReference) bool {
		return ref.Controller != nil && *ref.Controller && ref.Kind == "CronJob"
	}) {
		return
	}

	st, ok := c.state.jobs[r.source()]
	if !ok {
		st = newJobState()
		c.state.jobs[r.source()] = st

		st.uid = string(job.UID)
		st.name = job.Name
		st.namespace = job.Namespace
		st.creationTime = job.CreationTimestamp.Time

		for _, ref := range job.OwnerReferences {
			if ref.Controller != nil && *ref.Controller {
				st.controller.kind = ref.Kind
				st.controller.name = ref.Name
				st.controller.uid = string(ref.UID)
			}
		}
	}

	if job.Status.StartTime != nil {
		st.startTime = ptr(job.Status.StartTime.Time)
	}
	if job.Status.CompletionTime != nil {
		st.completionTime = ptr(job.Status.CompletionTime.Time)
	}
	st.active = job.Status.Active
	st.conditions = job.Status.Conditions
}
