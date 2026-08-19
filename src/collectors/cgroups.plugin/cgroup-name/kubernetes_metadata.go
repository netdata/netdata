// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"time"
)

const unknownKubernetesClusterName = "unknown"

type kubernetesMetadata struct {
	clusterName        string
	systemUID          string
	containerLabels    labelSet
	hasContainerLabels bool
	containers         []labelSet
}

func (r *resolver) loadKubernetesMetadata(ctx context.Context, functionName, containerID string) (kubernetesMetadata, kubePodOutcome) {
	cache, err := newKubernetesCache(r.config.kubernetesCacheDir)
	if err != nil {
		r.warningf("cannot use Kubernetes metadata cache %q: %v", r.config.kubernetesCacheDir, err)
		cache = kubernetesCache{}
	}

	metadata := kubernetesMetadata{
		systemUID:   cache.systemUID(),
		clusterName: cache.clusterName(),
	}
	if cache.clusterNameNeedsRefresh(metadata.clusterName, time.Now()) {
		if value, ok := r.k8sGCPGetClusterName(ctx); ok {
			metadata.clusterName = value
		} else {
			metadata.clusterName = unknownKubernetesClusterName
		}
		if err := cache.writeClusterName(metadata.clusterName); err != nil {
			r.warningf("cannot update Kubernetes cluster-name cache: %v", err)
		}
	}
	if containerID != "" && cache.complete() {
		if labels, ok := cache.lookupContainer(ctx, containerID); ok {
			metadata.containerLabels = labels
			metadata.hasContainerLabels = true
			return metadata, kubePodSuccess
		}
	}

	fetched := r.k8sFetchPods(ctx, functionName, metadata.systemUID)
	if fetched.outcome != kubePodSuccess {
		return kubernetesMetadata{}, fetched.outcome
	}
	if fetched.kubeSystemNamespace != "" {
		uid, err := jsonMetadataUID(fetched.kubeSystemNamespace)
		if err != nil {
			r.warningf("%s: error on 'jq' parse kube_system_ns: %s.", functionName, err.Error())
		} else {
			metadata.systemUID = uid
		}
	}

	containers, err := podsToContainerLabelSets(fetched.pods)
	if err != nil {
		r.warningf("%s: error on 'jq' parse pods: %s.", functionName, err.Error())
		return kubernetesMetadata{}, kubePodEnableFallback
	}
	metadata.containers = containers

	if fetched.kubeSystemNamespace != "" && metadata.systemUID != "" {
		if err := cache.writeSystemUID(metadata.systemUID); err != nil {
			r.warningf("cannot update Kubernetes system-UID cache: %v", err)
		}
	}
	if err := cache.writeContainers(metadata.containers); err != nil {
		r.warningf("cannot update Kubernetes container cache: %v", err)
	}
	return metadata, kubePodSuccess
}
