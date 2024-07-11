// SPDX-License-Identifier: GPL-3.0-or-later

package puppet

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioJVMHeap = module.Priority + iota
	prioJVMNonHeap
	prioCPUUsage
	prioFileDescriptors
)

const (
	byteToMiB = 1 << 20
)

var charts = module.Charts{
	jvmHeapChart.Copy(),
	jvmNonHeapChart.Copy(),
	cpuUsageChart.Copy(),
	fileDescriptorsChart.Copy(),
}

var (
	jvmHeapChart = module.Chart{
		ID:       "jvm_heap",
		Title:    "JVM Heap",
		Units:    "MiB",
		Fam:      "resources",
		Ctx:      "puppet.jvm_heap",
		Type:     module.Area,
		Priority: prioJVMHeap,
		Dims: module.Dims{
			{ID: "jvm_heap_committed", Name: "committed", Div: byteToMiB},
			{ID: "jvm_heap_used", Name: "used", Div: byteToMiB},
		},
		Vars: module.Vars{
			{ID: "jvm_heap_max"},
			{ID: "jvm_heap_init"},
		},
	}

	jvmNonHeapChart = module.Chart{
		ID:       "jvm_nonheap",
		Title:    "JVM Non-Heap",
		Units:    "MiB",
		Fam:      "resources",
		Ctx:      "puppet.jvm_nonheap",
		Type:     module.Area,
		Priority: prioJVMNonHeap,
		Dims: module.Dims{
			{ID: "jvm_nonheap_committed", Name: "committed", Div: byteToMiB},
			{ID: "jvm_nonheap_used", Name: "used", Div: byteToMiB},
		},
		Vars: module.Vars{
			{ID: "jvm_nonheap_max"},
			{ID: "jvm_nonheap_init"},
		},
	}

	cpuUsageChart = module.Chart{
		ID:       "cpu",
		Title:    "CPU usage",
		Units:    "percentage",
		Fam:      "resources",
		Ctx:      "puppet.cpu",
		Type:     module.Stacked,
		Priority: prioCPUUsage,
		Dims: module.Dims{
			{ID: "cpu_usage", Name: "execution", Div: 1000},
			{ID: "gc_cpu_usage", Name: "GC", Div: 1000},
		},
	}

	fileDescriptorsChart = module.Chart{
		ID:       "fd_open",
		Title:    "File Descriptors",
		Units:    "descriptors",
		Fam:      "resources",
		Ctx:      "puppet.fdopen",
		Type:     module.Line,
		Priority: prioFileDescriptors,
		Dims: module.Dims{
			{ID: "fd_used", Name: "used"},
		},
		Vars: module.Vars{
			{ID: "fd_max"},
		},
	}
)
