// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_state

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"
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
	c.collectDeploymentState(mx)
	c.collectCronJobState(mx)
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

func (c *Collector) collectDeploymentState(mx map[string]int64) {
	now := time.Now()

	maps.DeleteFunc(c.state.deployments, func(s string, ds *deploymentState) bool {
		if ds.deleted {
			c.removeDeploymentCharts(ds)
			return true
		}

		if ds.new {
			ds.new = false
			c.addDeploymentCharts(ds)
		}

		px := fmt.Sprintf("deploy_%s_", ds.id())

		mx[px+"age"] = int64(now.Sub(ds.creationTime).Seconds())
		mx[px+"desired_replicas"] = ds.replicas
		mx[px+"current_replicas"] = ds.availableReplicas
		mx[px+"ready_replicas"] = ds.readyReplicas

		mx[px+"condition_available"] = 0
		mx[px+"condition_progressing"] = 0
		mx[px+"condition_replica_failure"] = 0

		for _, cond := range ds.conditions {
			v := metrix.Bool(cond.Status == corev1.ConditionTrue)
			switch cond.Type {
			case appsv1.DeploymentAvailable:
				// https://github.com/kubernetes/kubernetes/blob/2b3da7dfc846fec7c4044a320f8f38b4a45367a3/pkg/controller/deployment/sync.go#L518-L525
				mx[px+"condition_available"] = v
			case appsv1.DeploymentProgressing:
				mx[px+"condition_progressing"] = v
			case appsv1.DeploymentReplicaFailure:
				mx[px+"condition_replica_failure"] = v
			}
		}

		return false
	})
}

func (c *Collector) collectCronJobState(mx map[string]int64) {
	now := time.Now()

	maps.DeleteFunc(c.state.jobs, func(s string, st *jobState) bool {
		return st.deleted
	})

	maps.DeleteFunc(c.state.cronJobs, func(s string, st *cronJobState) bool {
		if st.deleted {
			c.removeCronJobCharts(st)
			return true
		}
		if st.new {
			c.addCronJobCharts(st)
			st.new = false
		}

		px := fmt.Sprintf("cronjob_%s_", st.id())

		mx[px+"age"] = int64(now.Sub(st.creationTime).Seconds())
		if st.lastScheduleTime != nil {
			mx[px+"last_schedule_seconds_ago"] = int64(now.Sub(*st.lastScheduleTime).Seconds())
		}
		if st.lastSuccessfulTime != nil {
			mx[px+"last_successful_seconds_ago"] = int64(now.Sub(*st.lastSuccessfulTime).Seconds())
		}

		mx[px+"running_jobs"] = 0
		mx[px+"failed_jobs"] = 0
		mx[px+"complete_jobs"] = 0
		mx[px+"complete_jobs"] = 0
		mx[px+"suspended_jobs"] = 0

		mx[px+"suspend_status_enabled"] = metrix.Bool(!st.suspend)
		mx[px+"suspend_status_suspended"] = metrix.Bool(st.suspend)

		mx[px+"failed_jobs_reason_pod_failure_policy"] = 0
		mx[px+"failed_jobs_reason_backoff_limit_exceeded"] = 0
		mx[px+"failed_jobs_reason_deadline_exceeded"] = 0

		mx[px+"last_execution_status_succeeded"] = 0
		mx[px+"last_execution_status_failed"] = 0

		var lastExecutedEndTime time.Time
		var lastCompleteTime time.Time

		for _, job := range c.state.jobs {
			if job.controller.kind != "CronJob" || job.controller.uid != st.uid || job.startTime == nil {
				continue
			}
			if job.active > 0 {
				mx[px+"running_jobs"]++
				continue
			}

			for _, cond := range job.conditions {
				if cond.Status != corev1.ConditionTrue {
					continue
				}

				switch cond.Type {
				case batchv1.JobComplete:
					mx[px+"complete_jobs"]++
					if job.completionTime != nil {
						if job.completionTime.After(lastExecutedEndTime) {
							lastExecutedEndTime = *job.completionTime
							mx[px+"last_execution_status_succeeded"] = 1
							mx[px+"last_execution_status_failed"] = 0
						}
						if job.completionTime.After(lastCompleteTime) {
							lastCompleteTime = *job.completionTime
							mx[px+"last_completion_duration"] = int64(job.completionTime.Sub(*job.startTime).Seconds())
						}
					}
				case batchv1.JobFailed:
					mx[px+"failed_jobs"]++
					mx[px+"failed_jobs_reason_pod_failure_policy"] += metrix.Bool(cond.Reason == batchv1.JobReasonPodFailurePolicy)
					mx[px+"failed_jobs_reason_backoff_limit_exceeded"] += metrix.Bool(cond.Reason == batchv1.JobReasonBackoffLimitExceeded)
					mx[px+"failed_jobs_reason_deadline_exceeded"] += metrix.Bool(cond.Reason == batchv1.JobReasonDeadlineExceeded)
					if cond.LastTransitionTime.Time.After(lastExecutedEndTime) {
						lastExecutedEndTime = cond.LastTransitionTime.Time
						mx[px+"last_execution_status_succeeded"] = 0
						mx[px+"last_execution_status_failed"] = 1
					}
				case batchv1.JobSuspended:
					mx[px+"suspended_jobs"]++
				}
			}
		}

		return false
	})
}

func condStatusToInt(cs corev1.ConditionStatus) int64 {
	return metrix.Bool(cs == corev1.ConditionTrue)
}

func calcPercentage(value, total int64) int64 {
	if total == 0 {
		return 0
	}
	return int64(float64(value) / float64(total) * 100 * precision)
}
