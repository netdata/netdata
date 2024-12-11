// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_state

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"

	corev1 "k8s.io/api/core/v1"
)

const precision = 1000

var (
	podStatusReasons = []string{
		"Evicted",
		"NodeAffinity",
		"NodeLost",
		"Shutdown",
		"UnexpectedAdmissionError",
		"Other",
	}

	containerWaitingStateReasons = []string{
		"ContainerCreating",
		"CrashLoopBackOff",
		"CreateContainerConfigError",
		"CreateContainerError",
		"ErrImagePull",
		"ImagePullBackOff",
		"InvalidImageName",
		"PodInitializing",
		"Other",
	}
	containerTerminatedStateReasons = []string{
		"Completed",
		"ContainerCannotRun",
		"DeadlineExceeded",
		"Error",
		"Evicted",
		"OOMKilled",
		"Other",
	}
)

var (
	nodeConditionStatuses = []string{
		"Ready",
		"DiskPressure",
		"MemoryPressure",
		"NetworkUnavailable",
		"PIDPressure",
	}
)

func (c *Collector) collect() (map[string]int64, error) {
	if c.discoverer == nil {
		return nil, errors.New("nil discoverer")
	}

	c.once.Do(func() {
		c.startTime = time.Now()
		in := make(chan resource)

		c.wg.Add(1)
		go func() { defer c.wg.Done(); c.runUpdateState(in) }()

		c.wg.Add(1)
		go func() { defer c.wg.Done(); c.discoverer.run(c.ctx, in) }()

		c.kubeClusterID = c.getKubeClusterID()
		c.kubeClusterName = c.getKubeClusterName()

		if chart := c.Charts().Get(discoveryStatusChart.ID); chart != nil {
			chart.Labels = []module.Label{
				{Key: labelKeyClusterID, Value: c.kubeClusterID, Source: module.LabelSourceK8s},
				{Key: labelKeyClusterName, Value: c.kubeClusterName, Source: module.LabelSourceK8s},
			}
		}
	})

	mx := map[string]int64{
		"discovery_node_discoverer_state": 1,
		"discovery_pod_discoverer_state":  1,
	}

	if !c.discoverer.ready() || time.Since(c.startTime) < c.initDelay {
		return mx, nil
	}

	c.state.Lock()
	defer c.state.Unlock()

	c.collectKubeState(mx)

	return mx, nil
}

func (c *Collector) collectKubeState(mx map[string]int64) {
	for _, ns := range c.state.nodes {
		ns.resetStats()
	}
	c.collectPodsState(mx)
	c.collectNodesState(mx)
}

func (c *Collector) collectPodsState(mx map[string]int64) {
	now := time.Now()
	for _, ps := range c.state.pods {
		// Skip cronjobs (each of them is a unique container because the name contains hash)
		// to avoid overwhelming Netdata with high cardinality metrics.
		// Related issue https://github.com/netdata/netdata/issues/16412
		if ps.controllerKind == "Job" {
			continue
		}

		if ps.deleted {
			delete(c.state.pods, podSource(ps.namespace, ps.name))
			c.removePodCharts(ps)
			continue
		}

		if ps.new {
			ps.new = false
			c.addPodCharts(ps)
			ps.unscheduled = ps.nodeName == ""
		} else if ps.unscheduled && ps.nodeName != "" {
			ps.unscheduled = false
			c.updatePodChartsNodeLabel(ps)
		}

		ns := c.state.nodes[nodeSource(ps.nodeName)]
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
			ns.stats.podsReadinessReady += metrix.Bool(ps.condPodReady == corev1.ConditionTrue)
			ns.stats.podsReadinessUnready += metrix.Bool(ps.condPodReady != corev1.ConditionTrue)
			ns.stats.podsPhasePending += metrix.Bool(ps.phase == corev1.PodPending)
			ns.stats.podsPhaseRunning += metrix.Bool(ps.phase == corev1.PodRunning)
			ns.stats.podsPhaseSucceeded += metrix.Bool(ps.phase == corev1.PodSucceeded)
			ns.stats.podsPhaseFailed += metrix.Bool(ps.phase == corev1.PodFailed)

			for _, cs := range ps.initContainers {
				ns.stats.initContainers++
				ns.stats.initContStateRunning += metrix.Bool(cs.stateRunning)
				ns.stats.initContStateWaiting += metrix.Bool(cs.stateWaiting)
				ns.stats.initContStateTerminated += metrix.Bool(cs.stateTerminated)
			}

			for _, cs := range ps.containers {
				ns.stats.containers++
				ns.stats.contStateRunning += metrix.Bool(cs.stateRunning)
				ns.stats.contStateWaiting += metrix.Bool(cs.stateWaiting)
				ns.stats.contStateTerminated += metrix.Bool(cs.stateTerminated)
			}
		}

		px := fmt.Sprintf("pod_%s_", ps.id())

		mx[px+"cond_podready"] = condStatusToInt(ps.condPodReady)
		mx[px+"cond_podscheduled"] = condStatusToInt(ps.condPodScheduled)
		mx[px+"cond_podinitialized"] = condStatusToInt(ps.condPodInitialized)
		mx[px+"cond_containersready"] = condStatusToInt(ps.condContainersReady)
		mx[px+"phase_running"] = metrix.Bool(ps.phase == corev1.PodRunning)
		mx[px+"phase_failed"] = metrix.Bool(ps.phase == corev1.PodFailed)
		mx[px+"phase_succeeded"] = metrix.Bool(ps.phase == corev1.PodSucceeded)
		mx[px+"phase_pending"] = metrix.Bool(ps.phase == corev1.PodPending)
		mx[px+"age"] = int64(now.Sub(ps.creationTime).Seconds())

		for _, v := range podStatusReasons {
			mx[px+"status_reason_"+v] = 0
		}
		if v := ps.statusReason; v != "" {
			if !slices.Contains(podStatusReasons, v) {
				v = "Other"
			}
			mx[px+"status_reason_"+v] = 1
		}

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
			mx[px+"init_containers_state_running"] += metrix.Bool(cs.stateRunning)
			mx[px+"init_containers_state_waiting"] += metrix.Bool(cs.stateWaiting)
			mx[px+"init_containers_state_terminated"] += metrix.Bool(cs.stateTerminated)
		}
		mx[px+"containers_state_running"] = 0
		mx[px+"containers_state_waiting"] = 0
		mx[px+"containers_state_terminated"] = 0

		for _, cs := range ps.containers {
			if cs.new {
				cs.new = false
				c.addContainerCharts(ps, cs)
			}
			mx[px+"containers_state_running"] += metrix.Bool(cs.stateRunning)
			mx[px+"containers_state_waiting"] += metrix.Bool(cs.stateWaiting)
			mx[px+"containers_state_terminated"] += metrix.Bool(cs.stateTerminated)

			ppx := fmt.Sprintf("%scontainer_%s_", px, cs.name)
			mx[ppx+"state_running"] = metrix.Bool(cs.stateRunning)
			mx[ppx+"state_waiting"] = metrix.Bool(cs.stateWaiting)
			mx[ppx+"state_terminated"] = metrix.Bool(cs.stateTerminated)
			mx[ppx+"readiness"] = metrix.Bool(cs.ready)
			mx[ppx+"restarts"] = cs.restarts

			for _, v := range containerWaitingStateReasons {
				mx[ppx+"state_waiting_reason_"+v] = 0
			}
			if v := cs.waitingReason; v != "" {
				if !slices.Contains(containerWaitingStateReasons, v) {
					v = "Other"
				}
				mx[ppx+"state_waiting_reason_"+v] = 1
			}

			for _, v := range containerTerminatedStateReasons {
				mx[ppx+"state_terminated_reason_"+v] = 0
			}
			if v := cs.terminatedReason; v != "" {
				if !slices.Contains(containerTerminatedStateReasons, v) {
					v = "Other"
				}
				mx[ppx+"state_terminated_reason_"+v] = 1
			}
		}
	}
}

func (c *Collector) collectNodesState(mx map[string]int64) {
	now := time.Now()
	for _, ns := range c.state.nodes {
		if ns.deleted {
			delete(c.state.nodes, nodeSource(ns.name))
			c.removeNodeCharts(ns)
			continue
		}
		if ns.new {
			ns.new = false
			c.addNodeCharts(ns)
		}

		px := fmt.Sprintf("node_%s_", ns.id())

		for _, v := range nodeConditionStatuses {
			mx[px+"cond_"+v] = 0
		}
		for _, v := range ns.conditions {
			mx[px+"cond_"+string(v.Type)] = condStatusToInt(v.Status)
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
		mx[px+"schedulability_schedulable"] = metrix.Bool(!ns.unSchedulable)
		mx[px+"schedulability_unschedulable"] = metrix.Bool(ns.unSchedulable)
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
