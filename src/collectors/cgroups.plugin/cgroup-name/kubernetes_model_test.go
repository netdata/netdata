// SPDX-License-Identifier: GPL-3.0-or-later

package main

import "testing"

func TestPodsToContainerLabelSets(t *testing.T) {
	tests := map[string]struct {
		pods string
		want string
	}{
		"complete metadata": {
			pods: `{"items":[{"metadata":{"namespace":"default","name":"api-123","uid":"pod-uid","annotations":{"netdata.cloud/service":"payments"},"ownerReferences":[{"controller":true,"kind":"ReplicaSet","name":"api-123"}]},"spec":{"nodeName":"node-a"},"status":{"containerStatuses":[{"name":"app","containerID":"containerd://abcdef"}]}}]}`,
			want: `namespace="default",pod_name="api-123",pod_uid="pod-uid",netdata.cloud/service="payments",controller_kind="ReplicaSet",controller_name="api-123",node_name="node-a",container_name="app",container_id="abcdef"`,
		},
		"missing required field stays absent": {
			pods: `{"items":[{"metadata":{"name":"api-123","uid":"pod-uid"},"spec":{"nodeName":"node-a"},"status":{"containerStatuses":[{"name":"app","containerID":"containerd://abcdef"}]}}]}`,
			want: `pod_name="api-123",pod_uid="pod-uid",node_name="node-a",container_name="app",container_id="abcdef"`,
		},
		"literal null remains a value": {
			pods: `{"items":[{"metadata":{"namespace":"null","name":"api-123","uid":"pod-uid"},"spec":{"nodeName":"node-a"},"status":{"containerStatuses":[{"name":"app","containerID":"containerd://abcdef"}]}}]}`,
			want: `namespace="null",pod_name="api-123",pod_uid="pod-uid",node_name="node-a",container_name="app",container_id="abcdef"`,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			containers, err := podsToContainerLabelSets(test.pods)
			if err != nil {
				t.Fatal(err)
			}
			if len(containers) != 1 {
				t.Fatalf("containers = %d, want 1", len(containers))
			}
			if got := containers[0].String(); got != test.want {
				t.Fatalf("container labels:\nwant %q\n got %q", test.want, got)
			}
		})
	}
}
