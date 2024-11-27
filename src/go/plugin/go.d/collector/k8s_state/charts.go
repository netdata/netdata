// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_state

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

// NETDATA_CHART_PRIO_CGROUPS_CONTAINERS 40000
const prioDiscoveryDiscovererState = 50999

const (
	prioNodeAllocatableCPURequestsUtil = 50100 + iota
	prioNodeAllocatableCPURequestsUsed
	prioNodeAllocatableCPULimitsUtil
	prioNodeAllocatableCPULimitsUsed
	prioNodeAllocatableMemRequestsUtil
	prioNodeAllocatableMemRequestsUsed
	prioNodeAllocatableMemLimitsUtil
	prioNodeAllocatableMemLimitsUsed
	prioNodeAllocatablePodsUtil
	prioNodeAllocatablePodsUsage
	prioNodeConditions
	prioNodeSchedulability
	prioNodePodsReadiness
	prioNodePodsReadinessState
	prioNodePodsCondition
	prioNodePodsPhase
	prioNodeContainersCount
	prioNodeContainersState
	prioNodeInitContainersState
	prioNodeAge
)

const (
	prioPodCPURequestsUsed = 50300 + iota
	prioPodCPULimitsUsed
	prioPodMemRequestsUsed
	prioPodMemLimitsUsed
	prioPodCondition
	prioPodPhase
	prioPodStatusReason
	prioPodAge
	prioPodContainersCount
	prioPodContainersState
	prioPodInitContainersState
	prioPodContainerReadinessState
	prioPodContainerRestarts
	prioPodContainerState
	prioPodContainerWaitingStateReason
	prioPodContainerTerminatedStateReason
)

const (
	labelKeyPrefix = "k8s_"
	//labelKeyLabelPrefix      = labelKeyPrefix + "label_"
	//labelKeyAnnotationPrefix = labelKeyPrefix + "annotation_"
	labelKeyClusterID      = labelKeyPrefix + "cluster_id"
	labelKeyClusterName    = labelKeyPrefix + "cluster_name"
	labelKeyNamespace      = labelKeyPrefix + "namespace"
	labelKeyKind           = labelKeyPrefix + "kind"
	labelKeyPodName        = labelKeyPrefix + "pod_name"
	labelKeyNodeName       = labelKeyPrefix + "node_name"
	labelKeyPodUID         = labelKeyPrefix + "pod_uid"
	labelKeyControllerKind = labelKeyPrefix + "controller_kind"
	labelKeyControllerName = labelKeyPrefix + "controller_name"
	labelKeyContainerName  = labelKeyPrefix + "container_name"
	labelKeyContainerID    = labelKeyPrefix + "container_id"
	labelKeyQoSClass       = labelKeyPrefix + "qos_class"
)

var baseCharts = module.Charts{
	discoveryStatusChart.Copy(),
}

var nodeChartsTmpl = module.Charts{
	nodeAllocatableCPURequestsUtilChartTmpl.Copy(),
	nodeAllocatableCPURequestsUsedChartTmpl.Copy(),
	nodeAllocatableCPULimitsUtilChartTmpl.Copy(),
	nodeAllocatableCPULimitsUsedChartTmpl.Copy(),
	nodeAllocatableMemRequestsUtilChartTmpl.Copy(),
	nodeAllocatableMemRequestsUsedChartTmpl.Copy(),
	nodeAllocatableMemLimitsUtilChartTmpl.Copy(),
	nodeAllocatableMemLimitsUsedChartTmpl.Copy(),
	nodeAllocatablePodsUtilizationChartTmpl.Copy(),
	nodeAllocatablePodsUsageChartTmpl.Copy(),
	nodeConditionsChartTmpl.Copy(),
	nodeSchedulabilityChartTmpl.Copy(),
	nodePodsReadinessChartTmpl.Copy(),
	nodePodsReadinessStateChartTmpl.Copy(),
	nodePodsConditionChartTmpl.Copy(),
	nodePodsPhaseChartTmpl.Copy(),
	nodeContainersChartTmpl.Copy(),
	nodeContainersStateChartTmpl.Copy(),
	nodeInitContainersStateChartTmpl.Copy(),
	nodeAgeChartTmpl.Copy(),
}

var podChartsTmpl = module.Charts{
	podCPURequestsUsedChartTmpl.Copy(),
	podCPULimitsUsedChartTmpl.Copy(),
	podMemRequestsUsedChartTmpl.Copy(),
	podMemLimitsUsedChartTmpl.Copy(),
	podConditionChartTmpl.Copy(),
	podPhaseChartTmpl.Copy(),
	podStatusReasonChartTmpl.Copy(),
	podAgeChartTmpl.Copy(),
	podContainersCountChartTmpl.Copy(),
	podContainersStateChartTmpl.Copy(),
	podInitContainersStateChartTmpl.Copy(),
}

var containerChartsTmpl = module.Charts{
	containerReadinessStateChartTmpl.Copy(),
	containerRestartsChartTmpl.Copy(),
	containersStateChartTmpl.Copy(),
	containersStateWaitingChartTmpl.Copy(),
	containersStateTerminatedChartTmpl.Copy(),
}

var (
	// CPU resource
	nodeAllocatableCPURequestsUtilChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "node_%s.allocatable_cpu_requests_utilization",
		Title:    "CPU requests utilization",
		Units:    "%",
		Fam:      "node cpu resource",
		Ctx:      "k8s_state.node_allocatable_cpu_requests_utilization",
		Priority: prioNodeAllocatableCPURequestsUtil,
		Dims: module.Dims{
			{ID: "node_%s_alloc_cpu_requests_util", Name: "requests", Div: precision},
		},
	}
	nodeAllocatableCPURequestsUsedChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "node_%s.allocatable_cpu_requests_used",
		Title:    "CPU requests used",
		Units:    "millicpu",
		Fam:      "node cpu resource",
		Ctx:      "k8s_state.node_allocatable_cpu_requests_used",
		Priority: prioNodeAllocatableCPURequestsUsed,
		Dims: module.Dims{
			{ID: "node_%s_alloc_cpu_requests_used", Name: "requests"},
		},
	}
	nodeAllocatableCPULimitsUtilChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "node_%s.allocatable_cpu_limits_utilization",
		Title:    "CPU limits utilization",
		Units:    "%",
		Fam:      "node cpu resource",
		Ctx:      "k8s_state.node_allocatable_cpu_limits_utilization",
		Priority: prioNodeAllocatableCPULimitsUtil,
		Dims: module.Dims{
			{ID: "node_%s_alloc_cpu_limits_util", Name: "limits", Div: precision},
		},
	}
	nodeAllocatableCPULimitsUsedChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "node_%s.allocatable_cpu_limits_used",
		Title:    "CPU limits used",
		Units:    "millicpu",
		Fam:      "node cpu resource",
		Ctx:      "k8s_state.node_allocatable_cpu_limits_used",
		Priority: prioNodeAllocatableCPULimitsUsed,
		Dims: module.Dims{
			{ID: "node_%s_alloc_cpu_limits_used", Name: "limits"},
		},
	}
	// memory resource
	nodeAllocatableMemRequestsUtilChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "node_%s.allocatable_mem_requests_utilization",
		Title:    "Memory requests utilization",
		Units:    "%",
		Fam:      "node mem resource",
		Ctx:      "k8s_state.node_allocatable_mem_requests_utilization",
		Priority: prioNodeAllocatableMemRequestsUtil,
		Dims: module.Dims{
			{ID: "node_%s_alloc_mem_requests_util", Name: "requests", Div: precision},
		},
	}
	nodeAllocatableMemRequestsUsedChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "node_%s.allocatable_mem_requests_used",
		Title:    "Memory requests used",
		Units:    "bytes",
		Fam:      "node mem resource",
		Ctx:      "k8s_state.node_allocatable_mem_requests_used",
		Priority: prioNodeAllocatableMemRequestsUsed,
		Dims: module.Dims{
			{ID: "node_%s_alloc_mem_requests_used", Name: "requests"},
		},
	}
	nodeAllocatableMemLimitsUtilChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "node_%s.allocatable_mem_limits_utilization",
		Title:    "Memory limits utilization",
		Units:    "%",
		Fam:      "node mem resource",
		Ctx:      "k8s_state.node_allocatable_mem_limits_utilization",
		Priority: prioNodeAllocatableMemLimitsUtil,
		Dims: module.Dims{
			{ID: "node_%s_alloc_mem_limits_util", Name: "limits", Div: precision},
		},
	}
	nodeAllocatableMemLimitsUsedChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "node_%s.allocatable_mem_limits_used",
		Title:    "Memory limits used",
		Units:    "bytes",
		Fam:      "node mem resource",
		Ctx:      "k8s_state.node_allocatable_mem_limits_used",
		Priority: prioNodeAllocatableMemLimitsUsed,
		Dims: module.Dims{
			{ID: "node_%s_alloc_mem_limits_used", Name: "limits"},
		},
	}
	// pods resource
	nodeAllocatablePodsUtilizationChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "node_%s.allocatable_pods_utilization",
		Title:    "Pods resource utilization",
		Units:    "%",
		Fam:      "node pods resource",
		Ctx:      "k8s_state.node_allocatable_pods_utilization",
		Priority: prioNodeAllocatablePodsUtil,
		Dims: module.Dims{
			{ID: "node_%s_alloc_pods_util", Name: "allocated", Div: precision},
		},
	}
	nodeAllocatablePodsUsageChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "node_%s.allocated_pods_usage",
		Title:    "Pods resource usage",
		Units:    "pods",
		Fam:      "node pods resource",
		Ctx:      "k8s_state.node_allocatable_pods_usage",
		Type:     module.Stacked,
		Priority: prioNodeAllocatablePodsUsage,
		Dims: module.Dims{
			{ID: "node_%s_alloc_pods_available", Name: "available"},
			{ID: "node_%s_alloc_pods_allocated", Name: "allocated"},
		},
	}
	// condition
	nodeConditionsChartTmpl = func() module.Chart {
		chart := module.Chart{
			IDSep:    true,
			ID:       "node_%s.condition_status",
			Title:    "Condition status",
			Units:    "status",
			Fam:      "node condition",
			Ctx:      "k8s_state.node_condition",
			Priority: prioNodeConditions,
		}
		for _, v := range nodeConditionStatuses {
			chart.Dims = append(chart.Dims, &module.Dim{
				ID:   "node_%s_cond_" + v,
				Name: v,
			})
		}
		return chart
	}()
	nodeSchedulabilityChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "node_%s.schedulability",
		Title:    "Schedulability",
		Units:    "state",
		Fam:      "node schedulability",
		Ctx:      "k8s_state.node_schedulability",
		Priority: prioNodeSchedulability,
		Dims: module.Dims{
			{ID: "node_%s_schedulability_schedulable", Name: "schedulable"},
			{ID: "node_%s_schedulability_unschedulable", Name: "unschedulable"},
		},
	}
	// pods readiness
	nodePodsReadinessChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "node_%s.pods_readiness",
		Title:    "Pods readiness",
		Units:    "%",
		Fam:      "node pods readiness",
		Ctx:      "k8s_state.node_pods_readiness",
		Priority: prioNodePodsReadiness,
		Dims: module.Dims{
			{ID: "node_%s_pods_readiness", Name: "ready", Div: precision},
		},
	}
	nodePodsReadinessStateChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "node_%s.pods_readiness_state",
		Title:    "Pods readiness state",
		Units:    "pods",
		Fam:      "node pods readiness",
		Ctx:      "k8s_state.node_pods_readiness_state",
		Type:     module.Stacked,
		Priority: prioNodePodsReadinessState,
		Dims: module.Dims{
			{ID: "node_%s_pods_readiness_ready", Name: "ready"},
			{ID: "node_%s_pods_readiness_unready", Name: "unready"},
		},
	}
	// pods condition
	nodePodsConditionChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "node_%s.pods_condition",
		Title:    "Pods condition",
		Units:    "pods",
		Fam:      "node pods condition",
		Ctx:      "k8s_state.node_pods_condition",
		Priority: prioNodePodsCondition,
		Dims: module.Dims{
			{ID: "node_%s_pods_cond_podready", Name: "pod_ready"},
			{ID: "node_%s_pods_cond_podscheduled", Name: "pod_scheduled"},
			{ID: "node_%s_pods_cond_podinitialized", Name: "pod_initialized"},
			{ID: "node_%s_pods_cond_containersready", Name: "containers_ready"},
		},
	}
	// pods phase
	nodePodsPhaseChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "node_%s.pods_phase",
		Title:    "Pods phase",
		Units:    "pods",
		Fam:      "node pods phase",
		Ctx:      "k8s_state.node_pods_phase",
		Type:     module.Stacked,
		Priority: prioNodePodsPhase,
		Dims: module.Dims{
			{ID: "node_%s_pods_phase_running", Name: "running"},
			{ID: "node_%s_pods_phase_failed", Name: "failed"},
			{ID: "node_%s_pods_phase_succeeded", Name: "succeeded"},
			{ID: "node_%s_pods_phase_pending", Name: "pending"},
		},
	}
	// containers
	nodeContainersChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "node_%s.containers",
		Title:    "Containers",
		Units:    "containers",
		Fam:      "node containers",
		Ctx:      "k8s_state.node_containers",
		Priority: prioNodeContainersCount,
		Dims: module.Dims{
			{ID: "node_%s_containers", Name: "containers"},
			{ID: "node_%s_init_containers", Name: "init_containers"},
		},
	}
	nodeContainersStateChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "node_%s.containers_state",
		Title:    "Containers state",
		Units:    "containers",
		Fam:      "node containers",
		Ctx:      "k8s_state.node_containers_state",
		Type:     module.Stacked,
		Priority: prioNodeContainersState,
		Dims: module.Dims{
			{ID: "node_%s_containers_state_running", Name: "running"},
			{ID: "node_%s_containers_state_waiting", Name: "waiting"},
			{ID: "node_%s_containers_state_terminated", Name: "terminated"},
		},
	}
	nodeInitContainersStateChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "node_%s.init_containers_state",
		Title:    "Init containers state",
		Units:    "containers",
		Fam:      "node containers",
		Ctx:      "k8s_state.node_init_containers_state",
		Type:     module.Stacked,
		Priority: prioNodeInitContainersState,
		Dims: module.Dims{
			{ID: "node_%s_init_containers_state_running", Name: "running"},
			{ID: "node_%s_init_containers_state_waiting", Name: "waiting"},
			{ID: "node_%s_init_containers_state_terminated", Name: "terminated"},
		},
	}
	// age
	nodeAgeChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "node_%s.age",
		Title:    "Age",
		Units:    "seconds",
		Fam:      "node age",
		Ctx:      "k8s_state.node_age",
		Priority: prioNodeAge,
		Dims: module.Dims{
			{ID: "node_%s_age", Name: "age"},
		},
	}
)

func (c *Collector) newNodeCharts(ns *nodeState) *module.Charts {
	charts := nodeChartsTmpl.Copy()
	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, replaceDots(ns.id()))
		chart.Labels = c.newNodeChartLabels(ns)
		for _, d := range chart.Dims {
			d.ID = fmt.Sprintf(d.ID, ns.id())
		}
	}
	return charts
}

func (c *Collector) newNodeChartLabels(ns *nodeState) []module.Label {
	labels := []module.Label{
		{Key: labelKeyNodeName, Value: ns.name, Source: module.LabelSourceK8s},
		{Key: labelKeyClusterID, Value: c.kubeClusterID, Source: module.LabelSourceK8s},
		{Key: labelKeyClusterName, Value: c.kubeClusterName, Source: module.LabelSourceK8s},
	}
	return labels
}

func (c *Collector) addNodeCharts(ns *nodeState) {
	cs := c.newNodeCharts(ns)
	if err := c.Charts().Add(*cs...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeNodeCharts(ns *nodeState) {
	prefix := fmt.Sprintf("node_%s", replaceDots(ns.id()))
	for _, c := range *c.Charts() {
		if strings.HasPrefix(c.ID, prefix) {
			c.MarkRemove()
			c.MarkNotCreated()
		}
	}
}

var (
	podCPURequestsUsedChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "pod_%s.cpu_requests_used",
		Title:    "CPU requests used",
		Units:    "millicpu",
		Fam:      "pod allocated cpu",
		Ctx:      "k8s_state.pod_cpu_requests_used",
		Priority: prioPodCPURequestsUsed,
		Dims: module.Dims{
			{ID: "pod_%s_cpu_requests_used", Name: "requests"},
		},
	}
	podCPULimitsUsedChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "pod_%s.cpu_limits_used",
		Title:    "CPU limits used",
		Units:    "millicpu",
		Fam:      "pod allocated cpu",
		Ctx:      "k8s_state.pod_cpu_limits_used",
		Priority: prioPodCPULimitsUsed,
		Dims: module.Dims{
			{ID: "pod_%s_cpu_limits_used", Name: "limits"},
		},
	}
	podMemRequestsUsedChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "pod_%s.mem_requests_used",
		Title:    "Memory requests used",
		Units:    "bytes",
		Fam:      "pod allocated mem",
		Ctx:      "k8s_state.pod_mem_requests_used",
		Priority: prioPodMemRequestsUsed,
		Dims: module.Dims{
			{ID: "pod_%s_mem_requests_used", Name: "requests"},
		},
	}
	podMemLimitsUsedChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "pod_%s.mem_limits_used",
		Title:    "Memory limits used",
		Units:    "bytes",
		Fam:      "pod allocated mem",
		Ctx:      "k8s_state.pod_mem_limits_used",
		Priority: prioPodMemLimitsUsed,
		Dims: module.Dims{
			{ID: "pod_%s_mem_limits_used", Name: "limits"},
		},
	}
	podConditionChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "pod_%s.condition",
		Title:    "Condition",
		Units:    "state",
		Fam:      "pod condition",
		Ctx:      "k8s_state.pod_condition",
		Priority: prioPodCondition,
		Dims: module.Dims{
			{ID: "pod_%s_cond_podready", Name: "pod_ready"},
			{ID: "pod_%s_cond_podscheduled", Name: "pod_scheduled"},
			{ID: "pod_%s_cond_podinitialized", Name: "pod_initialized"},
			{ID: "pod_%s_cond_containersready", Name: "containers_ready"},
		},
	}
	podPhaseChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "pod_%s.phase",
		Title:    "Phase",
		Units:    "state",
		Fam:      "pod phase",
		Ctx:      "k8s_state.pod_phase",
		Priority: prioPodPhase,
		Dims: module.Dims{
			{ID: "pod_%s_phase_running", Name: "running"},
			{ID: "pod_%s_phase_failed", Name: "failed"},
			{ID: "pod_%s_phase_succeeded", Name: "succeeded"},
			{ID: "pod_%s_phase_pending", Name: "pending"},
		},
	}
	podStatusReasonChartTmpl = func() module.Chart {
		chart := module.Chart{
			IDSep:    true,
			ID:       "pod_%s.status_reason",
			Title:    "Status reason",
			Units:    "status",
			Fam:      "pod status",
			Ctx:      "k8s_state.pod_status_reason",
			Priority: prioPodStatusReason,
		}
		for _, v := range podStatusReasons {
			chart.Dims = append(chart.Dims, &module.Dim{
				ID:   "pod_%s_status_reason_" + v,
				Name: v,
			})
		}
		return chart
	}()
	podAgeChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "pod_%s.age",
		Title:    "Age",
		Units:    "seconds",
		Fam:      "pod age",
		Ctx:      "k8s_state.pod_age",
		Priority: prioPodAge,
		Dims: module.Dims{
			{ID: "pod_%s_age", Name: "age"},
		},
	}
	podContainersCountChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "pod_%s.containers_count",
		Title:    "Containers",
		Units:    "containers",
		Fam:      "pod containers",
		Ctx:      "k8s_state.pod_containers",
		Priority: prioPodContainersCount,
		Dims: module.Dims{
			{ID: "pod_%s_containers", Name: "containers"},
			{ID: "pod_%s_init_containers", Name: "init_containers"},
		},
	}
	podContainersStateChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "pod_%s.containers_state",
		Title:    "Containers state",
		Units:    "containers",
		Fam:      "pod containers",
		Ctx:      "k8s_state.pod_containers_state",
		Type:     module.Stacked,
		Priority: prioPodContainersState,
		Dims: module.Dims{
			{ID: "pod_%s_containers_state_running", Name: "running"},
			{ID: "pod_%s_containers_state_waiting", Name: "waiting"},
			{ID: "pod_%s_containers_state_terminated", Name: "terminated"},
		},
	}
	podInitContainersStateChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "pod_%s.init_containers_state",
		Title:    "Init containers state",
		Units:    "containers",
		Fam:      "pod containers",
		Ctx:      "k8s_state.pod_init_containers_state",
		Type:     module.Stacked,
		Priority: prioPodInitContainersState,
		Dims: module.Dims{
			{ID: "pod_%s_init_containers_state_running", Name: "running"},
			{ID: "pod_%s_init_containers_state_waiting", Name: "waiting"},
			{ID: "pod_%s_init_containers_state_terminated", Name: "terminated"},
		},
	}
)

func (c *Collector) newPodCharts(ps *podState) *module.Charts {
	charts := podChartsTmpl.Copy()
	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, replaceDots(ps.id()))
		chart.Labels = c.newPodChartLabels(ps)
		for _, d := range chart.Dims {
			d.ID = fmt.Sprintf(d.ID, ps.id())
		}
	}
	return charts
}

func (c *Collector) newPodChartLabels(ps *podState) []module.Label {
	labels := []module.Label{
		{Key: labelKeyNamespace, Value: ps.namespace, Source: module.LabelSourceK8s},
		{Key: labelKeyPodName, Value: ps.name, Source: module.LabelSourceK8s},
		{Key: labelKeyNodeName, Value: ps.nodeName, Source: module.LabelSourceK8s},
		{Key: labelKeyQoSClass, Value: ps.qosClass, Source: module.LabelSourceK8s},
		{Key: labelKeyControllerKind, Value: ps.controllerKind, Source: module.LabelSourceK8s},
		{Key: labelKeyControllerName, Value: ps.controllerName, Source: module.LabelSourceK8s},
		{Key: labelKeyClusterID, Value: c.kubeClusterID, Source: module.LabelSourceK8s},
		{Key: labelKeyClusterName, Value: c.kubeClusterName, Source: module.LabelSourceK8s},
	}
	return labels
}

func (c *Collector) addPodCharts(ps *podState) {
	charts := c.newPodCharts(ps)
	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) updatePodChartsNodeLabel(ps *podState) {
	prefix := fmt.Sprintf("pod_%s", replaceDots(ps.id()))
	for _, c := range *c.Charts() {
		if strings.HasPrefix(c.ID, prefix) {
			updateNodeLabel(c, ps.nodeName)
			c.MarkNotCreated()
		}
	}
}

func updateNodeLabel(c *module.Chart, nodeName string) {
	for i, l := range c.Labels {
		if l.Key == labelKeyNodeName {
			c.Labels[i].Value = nodeName
			break
		}
	}
}

func (c *Collector) removePodCharts(ps *podState) {
	prefix := fmt.Sprintf("pod_%s", replaceDots(ps.id()))
	for _, c := range *c.Charts() {
		if strings.HasPrefix(c.ID, prefix) {
			c.MarkRemove()
			c.MarkNotCreated()
		}
	}
}

var (
	containerReadinessStateChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "pod_%s_container_%s.readiness_state",
		Title:    "Readiness state",
		Units:    "state",
		Fam:      "container readiness",
		Ctx:      "k8s_state.pod_container_readiness_state",
		Priority: prioPodContainerReadinessState,
		Dims: module.Dims{
			{ID: "pod_%s_container_%s_readiness", Name: "ready"},
		},
	}
	containerRestartsChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "pod_%s_container_%s.restarts",
		Title:    "Restarts",
		Units:    "restarts",
		Fam:      "container restarts",
		Ctx:      "k8s_state.pod_container_restarts",
		Priority: prioPodContainerRestarts,
		Dims: module.Dims{
			{ID: "pod_%s_container_%s_restarts", Name: "restarts"},
		},
	}
	containersStateChartTmpl = module.Chart{
		IDSep:    true,
		ID:       "pod_%s_container_%s.state",
		Title:    "Container state",
		Units:    "state",
		Fam:      "container state",
		Ctx:      "k8s_state.pod_container_state",
		Priority: prioPodContainerState,
		Dims: module.Dims{
			{ID: "pod_%s_container_%s_state_running", Name: "running"},
			{ID: "pod_%s_container_%s_state_waiting", Name: "waiting"},
			{ID: "pod_%s_container_%s_state_terminated", Name: "terminated"},
		},
	}
	containersStateWaitingChartTmpl = func() module.Chart {
		chart := module.Chart{
			IDSep:    true,
			ID:       "pod_%s_container_%s.state_waiting_reason",
			Title:    "Container waiting state reason",
			Units:    "state",
			Fam:      "container waiting reason",
			Ctx:      "k8s_state.pod_container_waiting_state_reason",
			Priority: prioPodContainerWaitingStateReason,
		}
		for _, v := range containerWaitingStateReasons {
			chart.Dims = append(chart.Dims, &module.Dim{
				ID:   "pod_%s_container_%s_state_waiting_reason_" + v,
				Name: v,
			})
		}
		return chart
	}()
	containersStateTerminatedChartTmpl = func() module.Chart {
		chart := module.Chart{
			IDSep:    true,
			ID:       "pod_%s_container_%s.state_terminated_reason",
			Title:    "Container terminated state reason",
			Units:    "state",
			Fam:      "container terminated reason",
			Ctx:      "k8s_state.pod_container_terminated_state_reason",
			Priority: prioPodContainerTerminatedStateReason,
		}
		for _, v := range containerTerminatedStateReasons {
			chart.Dims = append(chart.Dims, &module.Dim{
				ID:   "pod_%s_container_%s_state_terminated_reason_" + v,
				Name: v,
			})
		}
		return chart
	}()
)

func (c *Collector) newContainerCharts(ps *podState, cs *containerState) *module.Charts {
	charts := containerChartsTmpl.Copy()
	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, replaceDots(ps.id()), cs.name)
		chart.Labels = c.newContainerChartLabels(ps, cs)
		for _, d := range chart.Dims {
			d.ID = fmt.Sprintf(d.ID, ps.id(), cs.name)
		}
	}
	return charts
}

func (c *Collector) newContainerChartLabels(ps *podState, cs *containerState) []module.Label {
	labels := c.newPodChartLabels(ps)
	labels = append(
		labels, module.Label{Key: labelKeyContainerName, Value: cs.name, Source: module.LabelSourceK8s},
	)
	return labels
}

func (c *Collector) addContainerCharts(ps *podState, cs *containerState) {
	charts := c.newContainerCharts(ps, cs)
	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

var discoveryStatusChart = module.Chart{
	ID:       "discovery_discoverers_state",
	Title:    "Running discoverers state",
	Units:    "state",
	Fam:      "discovery",
	Ctx:      "k8s_state.discovery_discoverers_state",
	Priority: prioDiscoveryDiscovererState,
	Opts:     module.Opts{Hidden: true},
	Dims: module.Dims{
		{ID: "discovery_node_discoverer_state", Name: "node"},
		{ID: "discovery_pod_discoverer_state", Name: "pod"},
	},
}

var reDots = regexp.MustCompile(`\.`)

func replaceDots(v string) string {
	return reDots.ReplaceAllString(v, "-")
}
