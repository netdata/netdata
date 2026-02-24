// SPDX-License-Identifier: GPL-3.0-or-later

package puppet

import (
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioJVMHeap = collectorapi.Priority + iota
	prioJVMNonHeap
	prioCPUUsage
	prioFileDescriptors
)

const (
	byteToMiB = 1 << 20
)

var charts = collectorapi.Charts{
	jvmHeapChart.Copy(),
	jvmNonHeapChart.Copy(),
	cpuUsageChart.Copy(),
	fileDescriptorsChart.Copy(),
}

var (
	jvmHeapChart = collectorapi.Chart{
		ID:       "jvm_heap",
		Title:    "JVM Heap",
		Units:    "MiB",
		Fam:      "resources",
		Ctx:      "puppet.jvm_heap",
		Type:     collectorapi.Area,
		Priority: prioJVMHeap,
		Dims: collectorapi.Dims{
			{ID: "jvm_heap_committed", Name: "committed", Div: byteToMiB},
			{ID: "jvm_heap_used", Name: "used", Div: byteToMiB},
		},
		Vars: collectorapi.Vars{
			{ID: "jvm_heap_max"},
			{ID: "jvm_heap_init"},
		},
	}

	jvmNonHeapChart = collectorapi.Chart{
		ID:       "jvm_nonheap",
		Title:    "JVM Non-Heap",
		Units:    "MiB",
		Fam:      "resources",
		Ctx:      "puppet.jvm_nonheap",
		Type:     collectorapi.Area,
		Priority: prioJVMNonHeap,
		Dims: collectorapi.Dims{
			{ID: "jvm_nonheap_committed", Name: "committed", Div: byteToMiB},
			{ID: "jvm_nonheap_used", Name: "used", Div: byteToMiB},
		},
		Vars: collectorapi.Vars{
			{ID: "jvm_nonheap_max"},
			{ID: "jvm_nonheap_init"},
		},
	}

	cpuUsageChart = collectorapi.Chart{
		ID:       "cpu",
		Title:    "CPU usage",
		Units:    "percentage",
		Fam:      "resources",
		Ctx:      "puppet.cpu",
		Type:     collectorapi.Stacked,
		Priority: prioCPUUsage,
		Dims: collectorapi.Dims{
			{ID: "cpu_usage", Name: "execution", Div: 1000},
			{ID: "gc_cpu_usage", Name: "GC", Div: 1000},
		},
	}

	fileDescriptorsChart = collectorapi.Chart{
		ID:       "fd_open",
		Title:    "File Descriptors",
		Units:    "descriptors",
		Fam:      "resources",
		Ctx:      "puppet.fdopen",
		Type:     collectorapi.Line,
		Priority: prioFileDescriptors,
		Dims: collectorapi.Dims{
			{ID: "fd_used", Name: "used"},
		},
		Vars: collectorapi.Vars{
			{ID: "fd_max"},
		},
	}
)
