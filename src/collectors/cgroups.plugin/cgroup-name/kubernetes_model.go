// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"encoding/json"
	"strings"
)

type podsDocument struct {
	Items []podItem `json:"items"`
}

type podItem struct {
	Metadata struct {
		Namespace       *string         `json:"namespace"`
		Name            *string         `json:"name"`
		UID             *string         `json:"uid"`
		Annotations     json.RawMessage `json:"annotations"`
		OwnerReferences []struct {
			Controller *bool   `json:"controller"`
			Kind       *string `json:"kind"`
			Name       *string `json:"name"`
		} `json:"ownerReferences"`
	} `json:"metadata"`
	Spec struct {
		NodeName *string `json:"nodeName"`
	} `json:"spec"`
	Status struct {
		ContainerStatuses []struct {
			Name        *string `json:"name"`
			ContainerID *string `json:"containerID"`
		} `json:"containerStatuses"`
	} `json:"status"`
}

var containerRuntimePrefix = strings.NewReplacer(
	"docker://", "",
	"cri-o://", "",
	"containerd://", "",
)

func podsToContainerLabelSets(raw string) ([]labelSet, error) {
	var document podsDocument
	if err := json.Unmarshal([]byte(raw), &document); err != nil {
		return nil, err
	}

	var containers []labelSet
	for _, item := range document.Items {
		var base labelSet
		addPresentLabel(&base, "namespace", item.Metadata.Namespace)
		addPresentLabel(&base, "pod_name", item.Metadata.Name)
		addPresentLabel(&base, "pod_uid", item.Metadata.UID)

		annotations, err := orderedStringEntries(item.Metadata.Annotations)
		if err != nil {
			return nil, err
		}
		for _, annotation := range annotations {
			if strings.HasPrefix(annotation.key, "netdata.cloud/") {
				base.add(annotation.key, annotation.value)
			}
		}
		for _, owner := range item.Metadata.OwnerReferences {
			if owner.Controller != nil && *owner.Controller {
				base.add("controller_kind", pointerString(owner.Kind))
				base.add("controller_name", pointerString(owner.Name))
				break
			}
		}
		base.add("node_name", pointerString(item.Spec.NodeName))

		for _, status := range item.Status.ContainerStatuses {
			labels := labelSet{items: append([]label(nil), base.items...)}
			addPresentLabel(&labels, "container_name", status.Name)
			if status.ContainerID != nil {
				labels.add("container_id", containerRuntimePrefix.Replace(*status.ContainerID))
			}
			containers = append(containers, labels)
		}
	}
	return containers, nil
}

func addPresentLabel(labels *labelSet, name string, value *string) {
	if value != nil {
		labels.add(name, *value)
	}
}

func jsonMetadataUID(raw string) (string, error) {
	var document struct {
		Metadata struct {
			UID *string `json:"uid"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal([]byte(raw), &document); err != nil {
		return "", err
	}
	if document.Metadata.UID == nil {
		return "null", nil
	}
	return *document.Metadata.UID, nil
}

func pointerString(value *string) string {
	if value == nil {
		return "null"
	}
	return *value
}
