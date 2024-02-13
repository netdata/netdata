// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_state

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
)

func (ks *KubeState) updatePodState(r resource) {
	if r.value() == nil {
		if ps, ok := ks.state.pods[r.source()]; ok {
			ps.deleted = true
		}
		return
	}

	pod, err := toPod(r)
	if err != nil {
		ks.Warning(err)
		return
	}

	ps, ok := ks.state.pods[r.source()]
	if !ok {
		ps = newPodState()
		ks.state.pods[r.source()] = ps
	}

	if !ok {
		ps.name = pod.Name
		ps.nodeName = pod.Spec.NodeName
		ps.namespace = pod.Namespace
		ps.creationTime = pod.CreationTimestamp.Time
		ps.uid = string(pod.UID)
		ps.qosClass = strings.ToLower(string(pod.Status.QOSClass))
		copyLabels(ps.labels, pod.Labels)
		for _, ref := range pod.OwnerReferences {
			if ref.Controller != nil && *ref.Controller {
				ps.controllerKind = ref.Kind
				ps.controllerName = ref.Name
			}
		}
		var res struct{ rCPU, lCPU, rMem, lMem, irCPU, ilCPU, irMem, ilMem int64 }
		for _, cntr := range pod.Spec.Containers {
			res.rCPU += int64(cntr.Resources.Requests.Cpu().AsApproximateFloat64() * 1000)
			res.lCPU += int64(cntr.Resources.Limits.Cpu().AsApproximateFloat64() * 1000)
			res.rMem += cntr.Resources.Requests.Memory().Value()
			res.lMem += cntr.Resources.Limits.Memory().Value()
		}
		for _, cntr := range pod.Spec.InitContainers {
			res.irCPU += int64(cntr.Resources.Requests.Cpu().AsApproximateFloat64() * 1000)
			res.ilCPU += int64(cntr.Resources.Limits.Cpu().AsApproximateFloat64() * 1000)
			res.irMem += cntr.Resources.Requests.Memory().Value()
			res.ilMem += cntr.Resources.Limits.Memory().Value()
		}
		ps.reqCPU = max(res.rCPU, res.irCPU)
		ps.limitCPU = max(res.lCPU, res.ilCPU)
		ps.reqMem = max(res.rMem, res.irMem)
		ps.limitMem = max(res.lMem, res.ilMem)
	}
	if ps.nodeName == "" {
		ps.nodeName = pod.Spec.NodeName
	}

	for _, c := range pod.Status.Conditions {
		switch c.Type {
		case corev1.ContainersReady:
			ps.condContainersReady = c.Status
		case corev1.PodInitialized:
			ps.condPodInitialized = c.Status
		case corev1.PodReady:
			ps.condPodReady = c.Status
		case corev1.PodScheduled:
			ps.condPodScheduled = c.Status
		}
	}

	ps.phase = pod.Status.Phase

	for _, cs := range ps.containers {
		for _, r := range cs.stateWaitingReasons {
			r.active = false
		}
		for _, r := range cs.stateTerminatedReasons {
			r.active = false
		}
	}

	for _, cntr := range pod.Status.ContainerStatuses {
		cs, ok := ps.containers[cntr.Name]
		if !ok {
			cs = newContainerState()
			ps.containers[cntr.Name] = cs
		}
		if !ok {
			cs.name = cntr.Name
			cs.podName = pod.Name
			cs.namespace = pod.Namespace
			cs.nodeName = pod.Spec.NodeName
			cs.uid = extractContainerID(cntr.ContainerID)
		}
		cs.ready = cntr.Ready
		cs.restarts = int64(cntr.RestartCount)
		cs.stateRunning = cntr.State.Running != nil
		cs.stateWaiting = cntr.State.Waiting != nil
		cs.stateTerminated = cntr.State.Terminated != nil

		if cntr.State.Waiting != nil {
			reason := cntr.State.Waiting.Reason
			r, ok := cs.stateWaitingReasons[reason]
			if !ok {
				r = &containerStateReason{new: true, reason: reason}
				cs.stateWaitingReasons[reason] = r
			}
			r.active = true
		}

		if cntr.State.Terminated != nil {
			reason := cntr.State.Terminated.Reason
			r, ok := cs.stateTerminatedReasons[reason]
			if !ok {
				r = &containerStateReason{new: true, reason: reason}
				cs.stateTerminatedReasons[reason] = r
			}
			r.active = true
		}
	}

	for _, cntr := range pod.Status.InitContainerStatuses {
		cs, ok := ps.initContainers[cntr.Name]
		if !ok {
			cs = newContainerState()
			ps.initContainers[cntr.Name] = cs
		}
		if !ok {
			cs.name = cntr.Name
			cs.podName = pod.Name
			cs.namespace = pod.Namespace
			cs.nodeName = pod.Spec.NodeName
			cs.uid = extractContainerID(cntr.ContainerID)
		}
		cs.ready = cntr.Ready
		cs.restarts = int64(cntr.RestartCount)
		cs.stateRunning = cntr.State.Running != nil
		cs.stateWaiting = cntr.State.Waiting != nil
		cs.stateTerminated = cntr.State.Terminated != nil
	}
}

func max(a, b int64) int64 {
	if a < b {
		return b
	}
	return a
}

func extractContainerID(id string) string {
	// docker://d98...
	if i := strings.LastIndexByte(id, '/'); i != -1 {
		id = id[i+1:]
	}
	return id
}
