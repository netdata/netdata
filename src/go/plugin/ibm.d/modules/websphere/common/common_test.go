package common

import "testing"

func TestIdentityLabels(t *testing.T) {
	id := Identity{Cluster: "prod", Cell: "appcell", Node: "node1", Server: "server1", Edition: "traditional", Version: "9.0.5"}
	labels := id.Labels()
	if labels["cluster"] != "prod" || labels["websphere_version"] != "9.0.5" {
		t.Fatalf("unexpected labels: %#v", labels)
	}
}

func TestNormaliseName(t *testing.T) {
	cases := map[string]string{
		"Web Container": "web_container",
		"$$$":           "unknown",
		"http-server":   "http_server",
	}
	for input, expect := range cases {
		if got := NormaliseName(input); got != expect {
			t.Fatalf("normalise %q: got %q expect %q", input, got, expect)
		}
	}
}

func TestInstanceKey(t *testing.T) {
	if key := InstanceKey("Node01", "Server1", "Thread Pool"); key != "node01.server1.thread_pool" {
		t.Fatalf("unexpected key: %s", key)
	}
}

func TestFormatPercent(t *testing.T) {
	if val := FormatPercent(12.345); val != 12345 {
		t.Fatalf("unexpected percent value: %d", val)
	}
}

func TestBuildMetricName(t *testing.T) {
	if name := BuildMetricName("JVM", "Heap Usage"); name != "jvm.heap_usage" {
		t.Fatalf("unexpected metric name: %s", name)
	}
}
