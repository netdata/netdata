// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"fmt"
)

type kubernetesMetadata struct {
	clusterName        string
	systemUID          string
	containerLabels    labelSet
	hasContainerLabels bool
	containers         []labelSet
}

func (r *resolver) loadKubernetesMetadata(ctx context.Context, functionName, containerID string) (kubernetesMetadata, kubePodOutcome) {
	cache := newKubernetesCache(r.config.tmpDir)
	cache.repairModes()

	var metadata kubernetesMetadata
	if containerID != "" && cache.complete() {
		if labels, ok := cache.lookup(containerID); ok {
			metadata.containerLabels = labels
			metadata.hasContainerLabels = true
			metadata.systemUID = cache.systemUID()
			metadata.clusterName = cache.clusterName()
			return metadata, kubePodSuccess
		}
	}

	metadata.systemUID = cache.systemUID()
	metadata.clusterName = cache.clusterName()
	if metadata.clusterName == "" {
		if value, ok := r.k8sGCPGetClusterName(ctx); ok {
			metadata.clusterName = value
		} else {
			metadata.clusterName = "unknown"
		}
	}

	fetched := r.k8sFetchPods(ctx, functionName, metadata.systemUID)
	if fetched.outcome != kubePodSuccess {
		return kubernetesMetadata{}, fetched.outcome
	}
	if fetched.kubeSystemNamespace != "" {
		uid, err := jsonMetadataUID(fetched.kubeSystemNamespace)
		if err != nil {
			r.warning(fmt.Sprintf("%s: error on 'jq' parse kube_system_ns: %s.", functionName, err.Error()))
		} else {
			metadata.systemUID = uid
		}
	}

	containers, err := podsToContainerLabelSets(fetched.pods)
	if err != nil {
		r.warning(fmt.Sprintf("%s: error on 'jq' parse pods: %s.", functionName, err.Error()))
		return kubernetesMetadata{}, kubePodEnableFallback
	}
	metadata.containers = containers

	cache.writeClusterName(metadata.clusterName)
	if fetched.kubeSystemNamespace != "" && metadata.systemUID != "" {
		cache.writeSystemUID(metadata.systemUID)
	}
	cache.writeContainers(metadata.containers)
	return metadata, kubePodSuccess
}
