// SPDX-License-Identifier: GPL-3.0-or-later

package websphere_pmi

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

// Chart templates based on ACTUAL WebSphere PMI metrics

var transactionChartsTmpl = module.Charts{
	{
		ID:       "transaction_manager_%s",
		Title:    "Transaction Manager",
		Units:    "transactions/s",
		Fam:      "transactions",
		Ctx:      "websphere_pmi.transaction_manager",
		Type:     module.Line,
		Priority: prioTransactionManager,
		Dims: module.Dims{
			{ID: "transaction_manager_%s_global_begun", Name: "global_begun", Algo: module.Incremental},
			{ID: "transaction_manager_%s_global_committed", Name: "global_committed", Algo: module.Incremental},
			{ID: "transaction_manager_%s_local_begun", Name: "local_begun", Algo: module.Incremental},
			{ID: "transaction_manager_%s_local_committed", Name: "local_committed", Algo: module.Incremental},
			{ID: "transaction_manager_%s_global_rolled_back", Name: "global_rolled_back", Algo: module.Incremental},
			{ID: "transaction_manager_%s_local_rolled_back", Name: "local_rolled_back", Algo: module.Incremental},
			{ID: "transaction_manager_%s_global_timeout", Name: "global_timeout", Algo: module.Incremental},
			{ID: "transaction_manager_%s_local_timeout", Name: "local_timeout", Algo: module.Incremental},
		},
	},
}

var jvmChartsTmpl = module.Charts{
	{
		ID:       "jvm_memory_%s",
		Title:    "JVM Memory",
		Units:    "bytes",
		Fam:      "jvm",
		Ctx:      "websphere_pmi.jvm_memory",
		Type:     module.Stacked,
		Priority: prioJVMMemory,
		Dims: module.Dims{
			{ID: "jvm_memory_%s_free", Name: "free"},
			{ID: "jvm_memory_%s_used", Name: "used"},
		},
	},
	{
		ID:       "jvm_uptime_%s",
		Title:    "JVM Uptime",
		Units:    "seconds",
		Fam:      "jvm",
		Ctx:      "websphere_pmi.jvm_uptime",
		Type:     module.Line,
		Priority: prioJVMUptime,
		Dims: module.Dims{
			{ID: "jvm_uptime_%s_seconds", Name: "uptime"},
		},
	},
	{
		ID:       "jvm_cpu_%s",
		Title:    "JVM CPU Usage",
		Units:    "percentage",
		Fam:      "jvm",
		Ctx:      "websphere_pmi.jvm_cpu",
		Type:     module.Line,
		Priority: prioJVMCPU,
		Dims: module.Dims{
			{ID: "jvm_cpu_%s_usage", Name: "usage", Div: 100},
		},
	},
}

var threadPoolChartsTmpl = module.Charts{
	{
		ID:       "thread_pool_%s_threads",
		Title:    "Thread Pool Threads",
		Units:    "threads",
		Fam:      "thread_pools",
		Ctx:      "websphere_pmi.thread_pool_threads",
		Type:     module.Line,
		Priority: prioThreadPools,
		Dims: module.Dims{
			{ID: "thread_pool_%s_active", Name: "active"},
			{ID: "thread_pool_%s_size", Name: "size"},
		},
	},
}

var jdbcChartsTmpl = module.Charts{
	{
		ID:       "jdbc_%s_connections",
		Title:    "JDBC Connections",
		Units:    "connections/s",
		Fam:      "jdbc",
		Ctx:      "websphere_pmi.jdbc_connections",
		Type:     module.Line,
		Priority: prioJDBCConnections,
		Dims: module.Dims{
			{ID: "jdbc_%s_connections_created", Name: "created", Algo: module.Incremental},
			{ID: "jdbc_%s_connections_closed", Name: "closed", Algo: module.Incremental},
			{ID: "jdbc_%s_connections_allocated", Name: "allocated", Algo: module.Incremental},
			{ID: "jdbc_%s_connections_returned", Name: "returned", Algo: module.Incremental},
		},
	},
	{
		ID:       "jdbc_%s_pool_size",
		Title:    "JDBC Pool Size",
		Units:    "connections",
		Fam:      "jdbc",
		Ctx:      "websphere_pmi.jdbc_pool_size",
		Type:     module.Line,
		Priority: prioJDBCPoolSize,
		Dims: module.Dims{
			{ID: "jdbc_%s_pool_free", Name: "free"},
			{ID: "jdbc_%s_pool_size", Name: "total"},
		},
	},
	{
		ID:       "jdbc_%s_wait_time",
		Title:    "JDBC Wait Time",
		Units:    "milliseconds/s",
		Fam:      "jdbc",
		Ctx:      "websphere_pmi.jdbc_wait_time",
		Type:     module.Line,
		Priority: prioJDBCWaitTime,
		Dims: module.Dims{
			{ID: "jdbc_%s_wait_time_total", Name: "wait_time", Algo: module.Incremental},
		},
	},
	{
		ID:       "jdbc_%s_waiting_threads",
		Title:    "JDBC Waiting Threads",
		Units:    "threads",
		Fam:      "jdbc",
		Ctx:      "websphere_pmi.jdbc_waiting_threads",
		Type:     module.Line,
		Priority: prioJDBCWaitingThreads,
		Dims: module.Dims{
			{ID: "jdbc_%s_waiting_threads", Name: "waiting_threads"},
		},
	},
	{
		ID:       "jdbc_%s_use_time",
		Title:    "JDBC Use Time",
		Units:    "milliseconds/s",
		Fam:      "jdbc",
		Ctx:      "websphere_pmi.jdbc_use_time",
		Type:     module.Line,
		Priority: prioJDBCUseTime,
		Dims: module.Dims{
			{ID: "jdbc_%s_use_time_total", Name: "use_time", Algo: module.Incremental},
		},
	},
}

// Chart templates for specialized parsers

var extensionRegistryChartsTmpl = module.Charts{
	{
		ID:       "extension_registry_%s_metrics",
		Title:    "Extension Registry Metrics",
		Units:    "requests/s",
		Fam:      "extension_registry",
		Ctx:      "websphere_pmi.extension_registry_metrics",
		Type:     module.Line,
		Priority: prioExtensionRegistryMetrics,
		Dims: module.Dims{
			{ID: "extension_registry_%s_requests", Name: "requests", Algo: module.Incremental},
			{ID: "extension_registry_%s_hits", Name: "hits", Algo: module.Incremental},
			{ID: "extension_registry_%s_displacements", Name: "displacements", Algo: module.Incremental},
		},
	},
	{
		ID:       "extension_registry_%s_hit_rate",
		Title:    "Extension Registry Hit Rate",
		Units:    "percentage",
		Fam:      "extension_registry",
		Ctx:      "websphere_pmi.extension_registry_hit_rate",
		Type:     module.Line,
		Priority: prioExtensionRegistryMetrics + 10,
		Dims: module.Dims{
			{ID: "extension_registry_%s_hit_rate", Name: "hit_rate"},
		},
	},
}

var sibJMSAdapterChartsTmpl = module.Charts{
	{
		ID:       "sib_jms_%s_connections",
		Title:    "SIB JMS Adapter Connections",
		Units:    "connections/s",
		Fam:      "sib_jms_adapter",
		Ctx:      "websphere_pmi.sib_jms_connections",
		Type:     module.Line,
		Priority: prioSIBJMSAdapterMetrics,
		Dims: module.Dims{
			{ID: "sib_jms_%s_create_count", Name: "created", Algo: module.Incremental},
			{ID: "sib_jms_%s_close_count", Name: "closed", Algo: module.Incremental},
			{ID: "sib_jms_%s_allocate_count", Name: "allocated", Algo: module.Incremental},
			{ID: "sib_jms_%s_freed_count", Name: "freed", Algo: module.Incremental},
		},
	},
	{
		ID:       "sib_jms_%s_faults",
		Title:    "SIB JMS Adapter Faults",
		Units:    "faults/s",
		Fam:      "sib_jms_adapter",
		Ctx:      "websphere_pmi.sib_jms_faults",
		Type:     module.Line,
		Priority: prioSIBJMSAdapterMetrics + 10,
		Dims: module.Dims{
			{ID: "sib_jms_%s_fault_count", Name: "faults", Algo: module.Incremental},
		},
	},
	{
		ID:       "sib_jms_%s_managed",
		Title:    "SIB JMS Adapter Managed Resources",
		Units:    "resources",
		Fam:      "sib_jms_adapter",
		Ctx:      "websphere_pmi.sib_jms_managed",
		Type:     module.Line,
		Priority: prioSIBJMSAdapterMetrics + 20,
		Dims: module.Dims{
			{ID: "sib_jms_%s_managed_connections", Name: "managed_connections"},
			{ID: "sib_jms_%s_connection_handles", Name: "connection_handles"},
		},
	},
}

var servletsComponentChartsTmpl = module.Charts{
	{
		ID:       "servlets_component_%s_requests",
		Title:    "Servlets Component Requests",
		Units:    "requests/s",
		Fam:      "servlets_component",
		Ctx:      "websphere_pmi.servlets_component_requests",
		Type:     module.Line,
		Priority: prioServletsComponentMetrics,
		Dims: module.Dims{
			{ID: "servlets_component_%s_requests", Name: "requests", Algo: module.Incremental},
			{ID: "servlets_component_%s_errors", Name: "errors", Algo: module.Incremental},
		},
	},
	{
		ID:       "servlets_component_%s_timing",
		Title:    "Servlets Component Timing",
		Units:    "milliseconds/s",
		Fam:      "servlets_component",
		Ctx:      "websphere_pmi.servlets_component_timing",
		Type:     module.Line,
		Priority: prioServletsComponentMetrics + 10,
		Dims: module.Dims{
			{ID: "servlets_component_%s_service_time", Name: "service_time", Algo: module.Incremental},
			{ID: "servlets_component_%s_async_response_time", Name: "async_response_time", Algo: module.Incremental},
		},
	},
	{
		ID:       "servlets_component_%s_concurrent",
		Title:    "Servlets Component Concurrent Requests",
		Units:    "requests",
		Fam:      "servlets_component",
		Ctx:      "websphere_pmi.servlets_component_concurrent",
		Type:     module.Line,
		Priority: prioServletsComponentMetrics + 20,
		Dims: module.Dims{
			{ID: "servlets_component_%s_concurrent_requests", Name: "concurrent_requests"},
		},
	},
}

var wimComponentChartsTmpl = module.Charts{
	{
		ID:       "wim_%s_portlet_requests",
		Title:    "WIM Component Portlet Requests",
		Units:    "requests/s",
		Fam:      "wim_component",
		Ctx:      "websphere_pmi.wim_portlet_requests",
		Type:     module.Line,
		Priority: prioWIMComponentMetrics,
		Dims: module.Dims{
			{ID: "wim_%s_portlet_requests", Name: "requests", Algo: module.Incremental},
			{ID: "wim_%s_portlet_errors", Name: "errors", Algo: module.Incremental},
		},
	},
	{
		ID:       "wim_%s_portlet_timing",
		Title:    "WIM Component Portlet Timing",
		Units:    "milliseconds/s",
		Fam:      "wim_component",
		Ctx:      "websphere_pmi.wim_portlet_timing",
		Type:     module.Line,
		Priority: prioWIMComponentMetrics + 10,
		Dims: module.Dims{
			{ID: "wim_%s_render_time", Name: "render_time", Algo: module.Incremental},
			{ID: "wim_%s_action_time", Name: "action_time", Algo: module.Incremental},
			{ID: "wim_%s_process_event_time", Name: "process_event_time", Algo: module.Incremental},
			{ID: "wim_%s_serve_resource_time", Name: "serve_resource_time", Algo: module.Incremental},
		},
	},
}

var wlmTaggedComponentChartsTmpl = module.Charts{
	{
		ID:       "wlm_tagged_%s_processing_time",
		Title:    "WLM Tagged Component Processing Time",
		Units:    "milliseconds/s",
		Fam:      "wlm_tagged_component",
		Ctx:      "websphere_pmi.wlm_tagged_processing_time",
		Type:     module.Line,
		Priority: prioWLMTaggedComponentMetrics,
		Dims: module.Dims{
			{ID: "wlm_tagged_%s_processing_time", Name: "processing_time", Algo: module.Incremental},
		},
	},
}

var pmiWebServiceServiceChartsTmpl = module.Charts{
	{
		ID:       "pmi_webservice_%s_requests",
		Title:    "PMI Web Service Service Requests",
		Units:    "requests/s",
		Fam:      "pmi_webservice_service",
		Ctx:      "websphere_pmi.pmi_webservice_requests",
		Type:     module.Line,
		Priority: prioPMIWebServiceServiceMetrics,
		Dims: module.Dims{
			{ID: "pmi_webservice_%s_requests_received", Name: "received", Algo: module.Incremental},
			{ID: "pmi_webservice_%s_requests_dispatched", Name: "dispatched", Algo: module.Incremental},
			{ID: "pmi_webservice_%s_requests_successful", Name: "successful", Algo: module.Incremental},
		},
	},
	{
		ID:       "pmi_webservice_%s_timing",
		Title:    "PMI Web Service Service Timing",
		Units:    "milliseconds/s",
		Fam:      "pmi_webservice_service",
		Ctx:      "websphere_pmi.pmi_webservice_timing",
		Type:     module.Line,
		Priority: prioPMIWebServiceServiceMetrics + 10,
		Dims: module.Dims{
			{ID: "pmi_webservice_%s_response_time", Name: "response_time", Algo: module.Incremental},
			{ID: "pmi_webservice_%s_request_response_time", Name: "request_response_time", Algo: module.Incremental},
		},
	},
}

var tcpChannelDCSChartsTmpl = module.Charts{
	{
		ID:       "tcp_dcs_%s_lifecycle",
		Title:    "TCP Channel DCS Lifecycle",
		Units:    "operations/s",
		Fam:      "tcp_channel_dcs",
		Ctx:      "websphere_pmi.tcp_dcs_lifecycle",
		Type:     module.Line,
		Priority: prioTCPChannelDCSMetrics,
		Dims: module.Dims{
			{ID: "tcp_dcs_%s_create_count", Name: "created", Algo: module.Incremental},
			{ID: "tcp_dcs_%s_destroy_count", Name: "destroyed", Algo: module.Incremental},
		},
	},
	{
		ID:       "tcp_dcs_%s_threads",
		Title:    "TCP Channel DCS Thread Management",
		Units:    "threads/s",
		Fam:      "tcp_channel_dcs",
		Ctx:      "websphere_pmi.tcp_dcs_threads",
		Type:     module.Line,
		Priority: prioTCPChannelDCSMetrics + 10,
		Dims: module.Dims{
			{ID: "tcp_dcs_%s_thread_hung_count", Name: "hung_declared", Algo: module.Incremental},
			{ID: "tcp_dcs_%s_thread_hang_cleared", Name: "hang_cleared", Algo: module.Incremental},
		},
	},
	{
		ID:       "tcp_dcs_%s_pool_status",
		Title:    "TCP Channel DCS Pool Status",
		Units:    "threads",
		Fam:      "tcp_channel_dcs",
		Ctx:      "websphere_pmi.tcp_dcs_pool_status",
		Type:     module.Line,
		Priority: prioTCPChannelDCSMetrics + 20,
		Dims: module.Dims{
			{ID: "tcp_dcs_%s_active_count", Name: "active"},
			{ID: "tcp_dcs_%s_pool_size", Name: "pool_size"},
			{ID: "tcp_dcs_%s_concurrent_hung_threads", Name: "concurrent_hung"},
		},
	},
	{
		ID:       "tcp_dcs_%s_utilization",
		Title:    "TCP Channel DCS Pool Utilization",
		Units:    "percentage",
		Fam:      "tcp_channel_dcs",
		Ctx:      "websphere_pmi.tcp_dcs_utilization",
		Type:     module.Line,
		Priority: prioTCPChannelDCSMetrics + 30,
		Dims: module.Dims{
			{ID: "tcp_dcs_%s_percent_used", Name: "used"},
			{ID: "tcp_dcs_%s_percent_maxed", Name: "maxed"},
		},
	},
	{
		ID:       "tcp_dcs_%s_timing",
		Title:    "TCP Channel DCS Active Time",
		Units:    "milliseconds/s",
		Fam:      "tcp_channel_dcs",
		Ctx:      "websphere_pmi.tcp_dcs_timing",
		Type:     module.Line,
		Priority: prioTCPChannelDCSMetrics + 40,
		Dims: module.Dims{
			{ID: "tcp_dcs_%s_active_time", Name: "active_time", Algo: module.Incremental},
		},
	},
}

var detailsComponentChartsTmpl = module.Charts{
	{
		ID:       "details_%s_portlets",
		Title:    "Details Component Loaded Portlets",
		Units:    "portlets",
		Fam:      "details_component",
		Ctx:      "websphere_pmi.details_portlets",
		Type:     module.Line,
		Priority: prioDetailsComponentMetrics,
		Dims: module.Dims{
			{ID: "details_%s_loaded_portlets", Name: "loaded_portlets"},
		},
	},
}

var iscProductDetailsChartsTmpl = module.Charts{
	{
		ID:       "isc_product_%s_portlet_requests",
		Title:    "ISC Product Details Portlet Requests",
		Units:    "requests/s",
		Fam:      "isc_product_details",
		Ctx:      "websphere_pmi.isc_product_portlet_requests",
		Type:     module.Line,
		Priority: prioISCProductDetailsMetrics,
		Dims: module.Dims{
			{ID: "isc_product_%s_portlet_requests", Name: "requests", Algo: module.Incremental},
			{ID: "isc_product_%s_portlet_errors", Name: "errors", Algo: module.Incremental},
		},
	},
	{
		ID:       "isc_product_%s_portlet_timing",
		Title:    "ISC Product Details Portlet Timing",
		Units:    "milliseconds/s",
		Fam:      "isc_product_details",
		Ctx:      "websphere_pmi.isc_product_portlet_timing",
		Type:     module.Line,
		Priority: prioISCProductDetailsMetrics + 10,
		Dims: module.Dims{
			{ID: "isc_product_%s_render_time", Name: "render_time", Algo: module.Incremental},
			{ID: "isc_product_%s_action_time", Name: "action_time", Algo: module.Incremental},
			{ID: "isc_product_%s_process_event_time", Name: "process_event_time", Algo: module.Incremental},
			{ID: "isc_product_%s_serve_resource_time", Name: "serve_resource_time", Algo: module.Incremental},
		},
	},
}

// Priority constants
const (
	prioTransactionManager        = 2000
	prioJVMMemory                = 2100
	prioJVMUptime                = 2110
	prioJDBCConnections          = 2300
	prioJDBCPoolSize             = 2310
	prioJDBCWaitTime             = 2320
	prioJDBCWaitingThreads       = 2330
	prioJDBCUseTime              = 2340
	
	// Specialized parser priorities
	prioExtensionRegistryMetrics      = 2700
	prioSIBJMSAdapterMetrics         = 2800
	prioServletsComponentMetrics     = 2900
	prioWIMComponentMetrics          = 3000
	prioWLMTaggedComponentMetrics    = 3100
	prioPMIWebServiceServiceMetrics  = 3200
	prioTCPChannelDCSMetrics        = 3300
	prioDetailsComponentMetrics      = 3400
	prioISCProductDetailsMetrics     = 3500
)