// SPDX-License-Identifier: GPL-3.0-or-later

package main

import "testing"

func TestPodsToContainerLabelSets(t *testing.T) {
	pods := `{"items":[{"metadata":{"namespace":"default","name":"api-123","uid":"pod-uid","annotations":{"netdata.cloud/service":"payments"},"ownerReferences":[{"controller":true,"kind":"ReplicaSet","name":"api-123"}]},"spec":{"nodeName":"node-a"},"status":{"containerStatuses":[{"name":"app","containerID":"containerd://abcdef"}]}}]}`
	containers, err := podsToContainerLabelSets(pods)
	if err != nil {
		t.Fatal(err)
	}
	if len(containers) != 1 {
		t.Fatalf("containers = %d, want 1", len(containers))
	}
	want := `namespace="default",pod_name="api-123",pod_uid="pod-uid",netdata.cloud/service="payments",controller_kind="ReplicaSet",controller_name="api-123",node_name="node-a",container_name="app",container_id="abcdef"`
	if got := containers[0].String(); got != want {
		t.Fatalf("container labels:\nwant %q\n got %q", want, got)
	}
}
