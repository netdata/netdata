// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"strings"
	"testing"
)

func TestLabelSet(t *testing.T) {
	var labels labelSet
	labels.add("namespace", "default")
	labels.add("pod_name", "api,primary")
	labels.add("container_id", "docker://abc")
	labels.add("netdata.cloud/key", "a=b")
	labels.add("netdata.cloud/escaped", "a,b\"c\nline")

	encoded := labels.String()
	if strings.Contains(encoded, "\n") {
		t.Fatalf("label stream contains a newline: %q", encoded)
	}
	parsed := parseLabelSet(encoded)
	tests := map[string]struct {
		got  string
		want string
	}{
		"embedded comma": {
			got:  parsed.valueOrNull("pod_name"),
			want: "api,primary",
		},
		"embedded equals": {
			got:  parsed.valueOrNull("netdata.cloud/key"),
			want: "a=b",
		},
		"escaped value": {
			got:  parsed.valueOrNull("netdata.cloud/escaped"),
			want: "a,b\"c line",
		},
		"missing value": {
			got:  parsed.valueOrNull("missing"),
			want: "null",
		},
		"remove label": {
			got:  parsed.without("container_id").String(),
			want: `namespace="default",pod_name="api,primary",netdata.cloud/key="a=b",netdata.cloud/escaped="a,b\"c line"`,
		},
		"prefix labels": {
			got:  parseLabelSet(`a="1,2",b="x\"y"`).prefixed("k8s_").String(),
			want: `k8s_a="1,2",k8s_b="x\"y"`,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("value = %q, want %q", test.got, test.want)
			}
		})
	}
}
