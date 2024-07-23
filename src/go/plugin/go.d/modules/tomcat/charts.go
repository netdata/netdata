// SPDX-License-Identifier: GPL-3.0-or-later

package tomcat

import "github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

const (
	prioAccesses = module.Priority + iota
	prioBandwidth
	prioProcessingTime
	prioThreads
	prioJVM
	prioJVMEden
	prioJVMSurvivor
	prioJVMTenured
)

const (
	byteToMiB = 1 << 20
)

var charts = module.Charts{
	chartAccesses.Copy(),
	chartBandwidth.Copy(),
	chartProcessingTime.Copy(),
	chartThreads.Copy(),
	chartJVM.Copy(),
	chartJVMEden.Copy(),
	chartJVMSurvivor.Copy(),
	chartJVMTenured.Copy(),
}

var (
	chartAccesses = module.Chart{
		ID:       "accesses",
		Title:    "Requests",
		Units:    "requests/s",
		Fam:      "statistics",
		Ctx:      "tomcat.accesses",
		Type:     module.Area,
		Priority: prioAccesses,
		Dims: module.Dims{
			{ID: "request_count", Name: "accesses", Algo: module.Incremental},
			{ID: "error_count", Name: "errors"},
		},
	}
	chartBandwidth = module.Chart{
		ID:       "bandwidth",
		Title:    "Bandwidth",
		Units:    "KiB/s",
		Fam:      "statistics",
		Ctx:      "tomcat.bandwidth",
		Type:     module.Area,
		Priority: prioBandwidth,
		Dims: module.Dims{
			{ID: "bytes_sent", Name: "sent", Div: 1024, Algo: module.Incremental},
			{ID: "bytes_received", Name: "received", Div: 1024, Algo: module.Incremental},
		},
	}
	chartProcessingTime = module.Chart{
		ID:       "processing_time",
		Title:    "Processing time",
		Units:    "seconds",
		Fam:      "statistics",
		Ctx:      "tomcat.processing_time",
		Type:     module.Area,
		Priority: prioProcessingTime,
		Dims: module.Dims{
			{ID: "processing_time", Name: "processing time", Algo: module.Incremental},
		},
	}
	chartThreads = module.Chart{
		ID:       "threads",
		Title:    "Threads",
		Units:    "current threads",
		Fam:      "statistics",
		Ctx:      "tomcat.threads",
		Type:     module.Area,
		Priority: prioThreads,
		Dims: module.Dims{
			{ID: "current_thread_count", Name: "current"},
			{ID: "busy_thread_count", Name: "busy"},
		},
	}
	chartJVM = module.Chart{
		ID:       "jvm",
		Title:    "JVM Memory Pool Usage",
		Units:    "MiB",
		Fam:      "memory",
		Ctx:      "tomcat.jvm",
		Type:     module.Stacked,
		Priority: prioJVM,
		Dims: module.Dims{
			{ID: "current_thread_count", Name: "current"},
			{ID: "busy_thread_count", Name: "busy"},
		},
	}
	chartJVMEden = module.Chart{
		ID:       "jvm_eden",
		Title:    "Eden Memory Usage",
		Units:    "MiB",
		Fam:      "memory",
		Ctx:      "tomcat.jvm_eden",
		Type:     module.Area,
		Priority: prioJVMEden,
		Dims: module.Dims{
			{ID: "eden_used", Name: "used", Div: byteToMiB},
			{ID: "eden_committed", Name: "committed", Div: byteToMiB},
			{ID: "eden_max", Name: "max", Div: byteToMiB},
		},
	}
	chartJVMSurvivor = module.Chart{
		ID:       "jvm_survivor",
		Title:    "Survivor Memory Usage",
		Units:    "MiB",
		Fam:      "memory",
		Ctx:      "tomcat.jvm_survivor",
		Type:     module.Area,
		Priority: prioJVMSurvivor,
		Dims: module.Dims{
			{ID: "survivor_used", Name: "used", Div: byteToMiB},
			{ID: "survivor_committed", Name: "committed", Div: byteToMiB},
			{ID: "survivor_max", Name: "max", Div: byteToMiB},
		},
	}
	chartJVMTenured = module.Chart{
		ID:       "jvm_tenured",
		Title:    "Tenured Memory Usage",
		Units:    "MiB",
		Fam:      "memory",
		Ctx:      "tomcat.jvm_tenured",
		Type:     module.Area,
		Priority: prioJVMTenured,
		Dims: module.Dims{
			{ID: "tenured_used", Name: "used", Div: byteToMiB},
			{ID: "tenured_committed", Name: "committed", Div: byteToMiB},
			{ID: "tenured_max", Name: "max", Div: byteToMiB},
		},
	}
)
