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
	if got := parsed.valueOrNull("pod_name"); got != "api,primary" {
		t.Fatalf("pod_name with embedded comma = %q", got)
	}
	if got := parsed.valueOrNull("netdata.cloud/key"); got != "a=b" {
		t.Fatalf("label with embedded equals = %q", got)
	}
	if got := parsed.valueOrNull("netdata.cloud/escaped"); got != "a,b\"c line" {
		t.Fatalf("escaped label value = %q", got)
	}
	if got := parsed.valueOrNull("missing"); got != "null" {
		t.Fatalf("missing label = %q", got)
	}
	if got := parsed.without("container_id").String(); strings.Contains(got, "container_id") {
		t.Fatalf("without left container_id in %q", got)
	}
	if got := parseLabelSet(`a="1,2",b="x\"y"`).prefixed("k8s_").String(); got != `k8s_a="1,2",k8s_b="x\"y"` {
		t.Fatalf("prefixed labels = %q", got)
	}
}
