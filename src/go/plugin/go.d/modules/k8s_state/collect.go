// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_state

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	corev1 "k8s.io/api/core/v1"
)

const precision = 1000

var (
	containerWaitingStateReasons = []string{
		"PodInitializing",
		"ContainerCreating",
		"CrashLoopBackOff",
		"CreateContainerConfigError",
		"ErrImagePull",
		"ImagePullBackOff",
		"CreateContainerError",
		"InvalidImageName",
		"Other",
	}
	containerTerminatedStateReasons = []string{
		"OOMKilled",
		"Completed",
		"Error",
		"ContainerCannotRun",
		"DeadlineExceeded",
		"Evicted",
		"Other",
	}
)

func (ks *KubeState) collect() (map[string]int64, error) {
	if ks.discoverer == nil {
		return nil, errors.New("nil discoverer")
	}

	ks.once.Do(func() {
		ks.startTime = time.Now()
		in := make(chan resource)

		ks.wg.Add(1)
		go func() { defer ks.wg.Done(); ks.runUpdateState(in) }()

		ks.wg.Add(1)
		go func() { defer ks.wg.Done(); ks.discoverer.run(ks.ctx, in) }()

		ks.kubeClusterID = ks.getKubeClusterID()
		ks.kubeClusterName = ks.getKubeClusterName()
		if chart := ks.Charts().Get(discoveryStatusChart.ID); chart != nil {
			chart.Labels = []module.Label{
				{Key: labelKeyClusterID, Value: ks.kubeClusterID, Source: module.LabelSourceK8s},
				{Key: labelKeyClusterName, Value: ks.kubeClusterName, Source: module.LabelSourceK8s},
			}
		}
	})

	mx := map[string]int64{
		"discovery_node_discoverer_state": 1,
		"discovery_pod_discoverer_state":  1,
	}

	if !ks.discoverer.ready() || time.Since(ks.startTime) < ks.initDelay {
		return mx, nil
	}

	ks.state.Lock()
	defer ks.state.Unlock()

	ks.collectKubeState(mx)

	return mx, nil
}

func (ks *KubeState) collectKubeState(mx map[string]int64) {
	for _, ns := range ks.state.nodes {
		ns.resetStats()
	}
	ks.collectPodsState(mx)
	ks.collectNodesState(mx)
}

func (ks *KubeState) collectPodsState(mx map[string]int64) {
	now := time.Now()
	for _, ps := range ks.state.pods {
		// Skip cronjobs (each of them is a unique container because name contains hash)
		// to avoid overwhelming Netdata with high cardinality metrics.
		// Related issue https://github.com/netdata/netdata/issues/16412
		if ps.controllerKind == "Job" {
			continue
		}

		if ps.deleted {
			delete(ks.state.pods, podSource(ps.namespace, ps.name))
			ks.removePodCharts(ps)
			continue
		}
		if ps.new {
			ps.new = false
			ks.addPodCharts(ps)
			ps.unscheduled = ps.nodeName == ""
		} else if ps.unscheduled && ps.nodeName != "" {
			ps.unscheduled = false
			ks.updatePodChartsNodeLabel(ps)
		}

		ns := ks.state.nodes[nodeSource(ps.nodeName)]
		if ns != nil {
			ns.stats.pods++
			ns.stats.reqCPU += ps.reqCPU
			ns.stats.limitCPU += ps.limitCPU
			ns.stats.reqMem += ps.reqMem
			ns.stats.limitMem += ps.limitMem
			ns.stats.podsCondPodReady += condStatusToInt(ps.condPodReady)
			ns.stats.podsCondPodScheduled += condStatusToInt(ps.condPodScheduled)
			ns.stats.podsCondPodInitialized += condStatusToInt(ps.condPodInitialized)
			ns.stats.podsCondContainersReady += condStatusToInt(ps.condContainersReady)
			ns.stats.podsReadinessReady += boolToInt(ps.condPodReady == corev1.ConditionTrue)
			ns.stats.podsReadinessUnready += boolToInt(ps.condPodReady != corev1.ConditionTrue)
			ns.stats.podsPhasePending += boolToInt(ps.phase == corev1.PodPending)
			ns.stats.podsPhaseRunning += boolToInt(ps.phase == corev1.PodRunning)
			ns.stats.podsPhaseSucceeded += boolToInt(ps.phase == corev1.PodSucceeded)
			ns.stats.podsPhaseFailed += boolToInt(ps.phase == corev1.PodFailed)
			for _, cs := range ps.initContainers {
				ns.stats.initContainers++
				ns.stats.initContStateRunning += boolToInt(cs.stateRunning)
				ns.stats.initContStateWaiting += boolToInt(cs.stateWaiting)
				ns.stats.initContStateTerminated += boolToInt(cs.stateTerminated)
			}
			for _, cs := range ps.containers {
				ns.stats.containers++
				ns.stats.contStateRunning += boolToInt(cs.stateRunning)
				ns.stats.contStateWaiting += boolToInt(cs.stateWaiting)
				ns.stats.contStateTerminated += boolToInt(cs.stateTerminated)
			}
		}

		px := fmt.Sprintf("pod_%s_", ps.id())

		mx[px+"cond_podready"] = condStatusToInt(ps.condPodReady)
		mx[px+"cond_podscheduled"] = condStatusToInt(ps.condPodScheduled)
		mx[px+"cond_podinitialized"] = condStatusToInt(ps.condPodInitialized)
		mx[px+"cond_containersready"] = condStatusToInt(ps.condContainersReady)
		mx[px+"phase_running"] = boolToInt(ps.phase == corev1.PodRunning)
		mx[px+"phase_failed"] = boolToInt(ps.phase == corev1.PodFailed)
		mx[px+"phase_succeeded"] = boolToInt(ps.phase == corev1.PodSucceeded)
		mx[px+"phase_pending"] = boolToInt(ps.phase == corev1.PodPending)
		mx[px+"age"] = int64(now.Sub(ps.creationTime).Seconds())
		mx[px+"cpu_requests_used"] = ps.reqCPU
		mx[px+"cpu_limits_used"] = ps.limitCPU
		mx[px+"mem_requests_used"] = ps.reqMem
		mx[px+"mem_limits_used"] = ps.limitMem

		mx[px+"init_containers"] = int64(len(ps.initContainers))
		mx[px+"containers"] = int64(len(ps.containers))

		mx[px+"init_containers_state_running"] = 0
		mx[px+"init_containers_state_waiting"] = 0
		mx[px+"init_containers_state_terminated"] = 0
		for _, cs := range ps.initContainers {
			mx[px+"init_containers_state_running"] += boolToInt(cs.stateRunning)
			mx[px+"init_containers_state_waiting"] += boolToInt(cs.stateWaiting)
			mx[px+"init_containers_state_terminated"] += boolToInt(cs.stateTerminated)
		}
		mx[px+"containers_state_running"] = 0
		mx[px+"containers_state_waiting"] = 0
		mx[px+"containers_state_terminated"] = 0
		for _, cs := range ps.containers {
			if cs.new {
				cs.new = false
				ks.addContainerCharts(ps, cs)
			}
			mx[px+"containers_state_running"] += boolToInt(cs.stateRunning)
			mx[px+"containers_state_waiting"] += boolToInt(cs.stateWaiting)
			mx[px+"containers_state_terminated"] += boolToInt(cs.stateTerminated)

			ppx := fmt.Sprintf("%scontainer_%s_", px, cs.name)
			mx[ppx+"state_running"] = boolToInt(cs.stateRunning)
			mx[ppx+"state_waiting"] = boolToInt(cs.stateWaiting)
			mx[ppx+"state_terminated"] = boolToInt(cs.stateTerminated)
			mx[ppx+"readiness"] = boolToInt(cs.ready)
			mx[ppx+"restarts"] = cs.restarts

			for _, v := range containerWaitingStateReasons {
				mx[ppx+"state_waiting_reason_"+v] = 0
			}
			if v := cs.waitingReason; v != "" {
				if !slices.Contains(containerWaitingStateReasons, cs.waitingReason) {
					v = "Other"
				}
				mx[ppx+"state_waiting_reason_"+v] = 1
			}

			for _, v := range containerTerminatedStateReasons {
				mx[ppx+"state_terminated_reason_"+v] = 0
			}
			if v := cs.terminatedReason; v != "" {
				if !slices.Contains(containerTerminatedStateReasons, cs.terminatedReason) {
					v = "Other"
				}
				mx[ppx+"state_terminated_reason_"+v] = 1
			}
		}
	}
}

func (ks *KubeState) collectNodesState(mx map[string]int64) {
	now := time.Now()
	for _, ns := range ks.state.nodes {
		if ns.deleted {
			delete(ks.state.nodes, nodeSource(ns.name))
			ks.removeNodeCharts(ns)
			continue
		}
		if ns.new {
			ns.new = false
			ks.addNodeCharts(ns)
		}

		px := fmt.Sprintf("node_%s_", ns.id())

		for typ, cond := range ns.conditions {
			if cond.new {
				cond.new = false
				ks.addNodeConditionToCharts(ns, typ)
			}
			mx[px+"cond_"+strings.ToLower(typ)] = condStatusToInt(cond.status)
		}

		mx[px+"age"] = int64(now.Sub(ns.creationTime).Seconds())
		mx[px+"alloc_pods_util"] = calcPercentage(ns.stats.pods, ns.allocatablePods)
		mx[px+"pods_readiness_ready"] = ns.stats.podsReadinessReady
		mx[px+"pods_readiness_unready"] = ns.stats.podsReadinessUnready
		mx[px+"pods_readiness"] = calcPercentage(ns.stats.podsReadinessReady, ns.stats.pods)
		mx[px+"pods_phase_running"] = ns.stats.podsPhaseRunning
		mx[px+"pods_phase_failed"] = ns.stats.podsPhaseFailed
		mx[px+"pods_phase_succeeded"] = ns.stats.podsPhaseSucceeded
		mx[px+"pods_phase_pending"] = ns.stats.podsPhasePending
		mx[px+"pods_cond_podready"] = ns.stats.podsCondPodReady
		mx[px+"pods_cond_podscheduled"] = ns.stats.podsCondPodScheduled
		mx[px+"pods_cond_podinitialized"] = ns.stats.podsCondPodInitialized
		mx[px+"pods_cond_containersready"] = ns.stats.podsCondContainersReady
		mx[px+"pods_cond_containersready"] = ns.stats.podsCondContainersReady
		mx[px+"schedulability_schedulable"] = boolToInt(!ns.unSchedulable)
		mx[px+"schedulability_unschedulable"] = boolToInt(ns.unSchedulable)
		mx[px+"alloc_pods_available"] = ns.allocatablePods - ns.stats.pods
		mx[px+"alloc_pods_allocated"] = ns.stats.pods
		mx[px+"alloc_cpu_requests_util"] = calcPercentage(ns.stats.reqCPU, ns.allocatableCPU)
		mx[px+"alloc_cpu_limits_util"] = calcPercentage(ns.stats.limitCPU, ns.allocatableCPU)
		mx[px+"alloc_mem_requests_util"] = calcPercentage(ns.stats.reqMem, ns.allocatableMem)
		mx[px+"alloc_mem_limits_util"] = calcPercentage(ns.stats.limitMem, ns.allocatableMem)
		mx[px+"alloc_cpu_requests_used"] = ns.stats.reqCPU
		mx[px+"alloc_cpu_limits_used"] = ns.stats.limitCPU
		mx[px+"alloc_mem_requests_used"] = ns.stats.reqMem
		mx[px+"alloc_mem_limits_used"] = ns.stats.limitMem
		mx[px+"init_containers"] = ns.stats.initContainers
		mx[px+"containers"] = ns.stats.containers
		mx[px+"containers_state_running"] = ns.stats.contStateRunning
		mx[px+"containers_state_waiting"] = ns.stats.contStateWaiting
		mx[px+"containers_state_terminated"] = ns.stats.contStateTerminated
		mx[px+"init_containers_state_running"] = ns.stats.initContStateRunning
		mx[px+"init_containers_state_waiting"] = ns.stats.initContStateWaiting
		mx[px+"init_containers_state_terminated"] = ns.stats.initContStateTerminated
	}
}

func boolToInt(v bool) int64 {
	if v {
		return 1
	}
	return 0
}

func condStatusToInt(cs corev1.ConditionStatus) int64 {
	switch cs {
	case corev1.ConditionFalse:
		return 0
	case corev1.ConditionTrue:
		return 1
	case corev1.ConditionUnknown:
		return 0
	default:
		return 0
	}
}

func calcPercentage(value, total int64) int64 {
	if total == 0 {
		return 0
	}
	return int64(float64(value) / float64(total) * 100 * precision)
}
