// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_state

import (
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
)

func newKubeState() *kubeState {
	return &kubeState{
		Mutex: &sync.Mutex{},
		nodes: make(map[string]*nodeState),
		pods:  make(map[string]*podState),
	}
}

func newNodeState() *nodeState {
	return &nodeState{
		new:    true,
		labels: make(map[string]string),
	}
}

func newPodState() *podState {
	return &podState{
		new:            true,
		labels:         make(map[string]string),
		initContainers: make(map[string]*containerState),
		containers:     make(map[string]*containerState),
	}
}

func newContainerState() *containerState {
	return &containerState{
		new: true,
	}
}

type kubeState struct {
	*sync.Mutex
	nodes map[string]*nodeState
	pods  map[string]*podState
}

type (
	nodeState struct {
		new     bool
		deleted bool

		name            string
		unSchedulable   bool
		labels          map[string]string
		creationTime    time.Time
		allocatableCPU  int64
		allocatableMem  int64
		allocatablePods int64
		conditions      []corev1.NodeCondition

		stats nodeStateStats
	}
	nodeStateStats struct {
		reqCPU   int64
		limitCPU int64
		reqMem   int64
		limitMem int64
		pods     int64

		podsCondPodReady        int64
		podsCondPodScheduled    int64
		podsCondPodInitialized  int64
		podsCondContainersReady int64

		podsReadinessReady   int64
		podsReadinessUnready int64

		podsPhaseRunning   int64
		podsPhaseFailed    int64
		podsPhaseSucceeded int64
		podsPhasePending   int64

		containers              int64
		initContainers          int64
		initContStateRunning    int64
		initContStateWaiting    int64
		initContStateTerminated int64
		contStateRunning        int64
		contStateWaiting        int64
		contStateTerminated     int64
	}
)

func (ns *nodeState) id() string  { return ns.name }
func (ns *nodeState) resetStats() { ns.stats = nodeStateStats{} }

type (
	podState struct {
		new         bool
		deleted     bool
		unscheduled bool

		name           string
		nodeName       string
		namespace      string
		uid            string
		labels         map[string]string
		controllerKind string
		controllerName string
		qosClass       string
		creationTime   time.Time
		reqCPU         int64
		reqMem         int64
		limitCPU       int64
		limitMem       int64
		// https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#pod-conditions
		condPodScheduled    corev1.ConditionStatus
		condContainersReady corev1.ConditionStatus
		condPodInitialized  corev1.ConditionStatus
		condPodReady        corev1.ConditionStatus
		// https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#pod-phase
		phase        corev1.PodPhase
		statusReason string

		initContainers map[string]*containerState
		containers     map[string]*containerState
	}
)

func (ps podState) id() string { return ps.namespace + "_" + ps.name }

type containerState struct {
	new bool

	name string
	uid  string

	podName   string
	nodeName  string
	namespace string

	ready           bool
	restarts        int64
	stateRunning    bool
	stateWaiting    bool
	stateTerminated bool

	waitingReason    string
	terminatedReason string
}
