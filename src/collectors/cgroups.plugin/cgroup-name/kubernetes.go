// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	reK8sQOS     = regexp.MustCompile(`.+(besteffort|burstable|guaranteed)$`)
	reK8sCRIID   = regexp.MustCompile(`.+pod[a-f0-9_-]+_(docker|crio|cri-containerd)-([a-f0-9]+)$`)
	reK8sPlainID = regexp.MustCompile(`.+pod[a-f0-9-]+_([a-f0-9]+)$`)
	reK8sPodUID  = regexp.MustCompile(`.+pod([a-f0-9_-]+)$`)
	reK8sQOSAny  = regexp.MustCompile(`.+(besteffort|burstable)`)
	reNullName   = regexp.MustCompile(`_null(_|$)`)
)

type kubePodOutcome int

const (
	kubePodEnableFallback kubePodOutcome = iota
	kubePodSuccess
	kubePodRetryFallback
	kubePodDisableFallback
)

type kubePodResolution struct {
	name    string
	labels  labelSet
	outcome kubePodOutcome
}

type kubernetesCgroupIdentity struct {
	baseName    string
	podUID      string
	containerID string
	qosClass    string
}

func isKubernetesCgroup(cgroup string) bool {
	return strings.Contains(cgroup, "kubepods")
}

func (r *resolver) resolveKubernetes(ctx context.Context, cgroupPath, id string) resolution {
	result := r.resolveKubePod(ctx, cgroupPath, id)
	switch result.outcome {
	case kubePodSuccess:
		name := "k8s_" + result.name
		if labels := result.labels.String(); labels != "" {
			r.infof("k8s_get_name: cgroup '%s' has chart name '%s', labels '%s'", id, name, labels)
		} else {
			r.infof("k8s_get_name: cgroup '%s' has chart name '%s'", id, name)
		}
		return resolution{name: name, labels: result.labels, exitCode: exitSuccess}
	case kubePodRetryFallback:
		if r.budgetExpired() {
			return r.deadlineRetry()
		}
		name := "k8s_" + id
		r.warningf("k8s_get_name: cannot find the name of cgroup with id '%s'. Setting name to %s and asking for retry.", id, name)
		return resolution{name: name, exitCode: exitRetry}
	case kubePodDisableFallback:
		name := "k8s_" + id
		r.warningf("k8s_get_name: cannot find the name of cgroup with id '%s'. Setting name to %s and disabling it.", id, name)
		return resolution{name: name, exitCode: exitDisable}
	default:
		if r.budgetExpired() {
			return r.deadlineRetry()
		}
		name := "k8s_" + id
		r.warningf("k8s_get_name: cannot find the name of cgroup with id '%s'. Setting name to %s and enabling it.", id, name)
		return resolution{name: name, exitCode: exitSuccess}
	}
}

func (r *resolver) resolveKubePod(ctx context.Context, cgroupPath, id string) kubePodResolution {
	const functionName = "k8s_get_kubepod_name"
	if !isKubernetesCgroup(id) {
		r.warningf("%s: '%s' is not kubepod cgroup.", functionName, id)
		return kubePodResolution{outcome: kubePodEnableFallback}
	}

	identity := parseKubernetesCgroup(id)
	if identity.baseName != "" {
		return kubePodResolution{name: identity.baseName, outcome: kubePodSuccess}
	}
	if identity.podUID == "" && identity.containerID == "" {
		r.warningf("%s: can't extract pod_uid or container_id from the cgroup '%s'.", functionName, id)
		return kubePodResolution{outcome: kubePodDisableFallback}
	}
	if identity.podUID != "" {
		r.infof("%s: cgroup '%s' is a pod(uid:%s)", functionName, id, identity.podUID)
	}
	if identity.containerID != "" {
		r.infof("%s: cgroup '%s' is a container(id:%s)", functionName, id, identity.containerID)
		if r.k8sIsPauseContainer(cgroupPath) {
			return kubePodResolution{outcome: kubePodDisableFallback}
		}
	}

	metadata, outcome := r.loadKubernetesMetadata(ctx, functionName, identity.containerID)
	if outcome != kubePodSuccess {
		return kubePodResolution{outcome: outcome}
	}
	if identity.containerID != "" {
		return r.resolveKubernetesContainer(functionName, id, identity, metadata)
	}
	return r.resolveKubernetesPod(functionName, id, identity, metadata)
}

func parseKubernetesCgroup(id string) kubernetesCgroupIdentity {
	cleanID := strings.ReplaceAll(id, ".slice", "")
	cleanID = strings.ReplaceAll(cleanID, ".scope", "")

	identity := kubernetesCgroupIdentity{qosClass: "guaranteed"}
	if match := reK8sQOSAny.FindStringSubmatch(cleanID); len(match) > 1 {
		identity.qosClass = match[1]
	}
	switch {
	case cleanID == "kubepods":
		identity.baseName = cleanID
	case reK8sQOS.MatchString(cleanID):
		identity.baseName = strings.ReplaceAll(cleanID, "-", "_")
		if suffix, ok := strings.CutPrefix(identity.baseName, "kubepods_kubepods"); ok {
			identity.baseName = "kubepods" + suffix
		}
	case reK8sCRIID.MatchString(cleanID):
		identity.containerID = reK8sCRIID.FindStringSubmatch(cleanID)[2]
	case reK8sPlainID.MatchString(cleanID):
		identity.containerID = reK8sPlainID.FindStringSubmatch(cleanID)[1]
	case reK8sPodUID.MatchString(cleanID):
		identity.podUID = strings.ReplaceAll(reK8sPodUID.FindStringSubmatch(cleanID)[1], "_", "-")
	}
	return identity
}

func (r *resolver) resolveKubernetesContainer(functionName, id string, identity kubernetesCgroupIdentity, metadata kubernetesMetadata) kubePodResolution {
	labels := metadata.containerLabels
	if !metadata.hasContainerLabels {
		var ok bool
		labels, ok = findLabelSet(metadata.containers, identity.containerID)
		if !ok {
			return kubePodResolution{outcome: kubePodRetryFallback}
		}
	}
	containerName := labels.valueOrNull("container_name")
	podName := labels.valueOrNull("pod_name")
	if strings.HasPrefix(podName, "virt-launcher-") {
		switch containerName {
		case "volumerootdisk", "guest-console-log":
			r.infof("%s: skipping kubevirt helper container '%s' in pod '%s'", functionName, containerName, podName)
			return kubePodResolution{outcome: kubePodDisableFallback}
		}
	}
	labels.add("kind", "container")
	labels.add("qos_class", identity.qosClass)
	labels = addClusterLabels(labels, metadata.systemUID, metadata.clusterName)
	name := "cntr_" + labels.valueOrNull("namespace") + "_" + labels.valueOrNull("pod_name") + "_" + labels.valueOrNull("container_name")
	labels = labels.without("container_id").without("pod_uid").prefixed("k8s_")
	return r.validateKubernetesName(functionName, id, name, labels)
}

func (r *resolver) resolveKubernetesPod(functionName, id string, identity kubernetesCgroupIdentity, metadata kubernetesMetadata) kubePodResolution {
	labels, ok := findLabelSet(metadata.containers, identity.podUID)
	if !ok {
		return kubePodResolution{outcome: kubePodRetryFallback}
	}
	labels = labelsBeforeContainerFields(labels)
	labels.add("kind", "pod")
	labels.add("qos_class", identity.qosClass)
	labels = addClusterLabels(labels, metadata.systemUID, metadata.clusterName)
	name := "pod_" + labels.valueOrNull("namespace") + "_" + labels.valueOrNull("pod_name")
	labels = labels.without("pod_uid").prefixed("k8s_")
	return r.validateKubernetesName(functionName, id, name, labels)
}

func (r *resolver) validateKubernetesName(functionName, id, name string, labels labelSet) kubePodResolution {
	if reNullName.MatchString(name) {
		r.warningf("%s: invalid name: %s (cgroup '%s')", functionName, name, id)
		if r.config.kubernetes.useKubelet {
			return kubePodResolution{name: name, labels: labels, outcome: kubePodRetryFallback}
		}
		return kubePodResolution{name: name, labels: labels, outcome: kubePodEnableFallback}
	}
	if name == "" {
		return kubePodResolution{outcome: kubePodEnableFallback}
	}
	return kubePodResolution{name: name, labels: labels, outcome: kubePodSuccess}
}

func (r *resolver) k8sIsPauseContainer(cgroupPath string) bool {
	base := hostPath(r.config.hostPrefix, "/sys/fs/cgroup")
	file := filepath.Join(base, cgroupPath, "cgroup.procs")
	if isDir(filepath.Join(base, "cpuacct")) {
		file = filepath.Join(base, "cpuacct", cgroupPath, "cgroup.procs")
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return false
	}
	// /proc intentionally uses the running namespace, not NETDATA_HOST_PREFIX:
	// containerized deployments run the plugin in the host PID namespace.
	return singleCgroupProcessIsPause(data, func(pid string) ([]byte, error) {
		return os.ReadFile(filepath.Join("/proc", pid, "comm"))
	})
}

func singleCgroupProcessIsPause(processes []byte, readComm func(string) ([]byte, error)) bool {
	fields := strings.Fields(string(processes))
	if len(fields) != 1 {
		return false
	}
	comm, err := readComm(fields[0])
	return err == nil && strings.TrimRight(string(comm), "\n") == "pause"
}

func addClusterLabels(labels labelSet, systemUID, clusterName string) labelSet {
	if systemUID != "" && systemUID != "null" {
		labels.add("cluster_id", systemUID)
	}
	if clusterName != "" && clusterName != "unknown" {
		labels.add("cluster_name", clusterName)
	}
	return labels
}

func labelsBeforeContainerFields(labels labelSet) labelSet {
	var out labelSet
	for _, item := range labels.items {
		if strings.HasPrefix(item.name, "container_") {
			break
		}
		out.items = append(out.items, item)
	}
	return out
}
