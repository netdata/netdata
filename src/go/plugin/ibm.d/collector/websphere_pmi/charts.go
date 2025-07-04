// SPDX-License-Identifier: GPL-3.0-or-later

package websphere_pmi

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

// Base charts for WebSphere PMI - all charts are created dynamically
// This empty chart set is required by the framework
var baseCharts = module.Charts{
	// All charts are created dynamically by ensureChartExists()
	// based on the actual metrics discovered in the PMI data
}

// Chart creation priorities for consistent ordering
const (
	// Server metrics
	prioServerExtensions = 70000
	prioServerUptime     = 70001
	prioServerHealth     = 70002

	// JVM metrics
	prioJVMHeap    = 70100
	prioJVMNonHeap = 70101
	prioJVMGC      = 70200
	prioJVMThreads = 70300
	prioJVMClasses = 70301
	prioJVMCPU     = 70302

	// Security metrics
	prioSecurityAuth     = 71000
	prioSecurityAuthz    = 71001
	prioSecuritySubjects = 71002

	// System metrics
	prioSystemTransactions = 70900
	prioSystemWorkload     = 70901

	// Connection pools
	prioJDBCPools = 70600
	prioJCAPools  = 70700

	// Threading
	prioThreadPools = 70800
	prioHAManager   = 70801

	// Web container
	prioWebServlets = 70400
	prioWebSessions = 70500
	prioWebPortlets = 70501
	prioWebApps     = 70502

	// Caching
	prioDynaCache = 71300

	// Messaging
	prioJMSDestinations = 71100
	prioSIBMessaging    = 71200
	prioWebServices     = 71201
	prioORBInterceptors = 71202

	// Object pools
	prioObjectPoolObjects    = 70801
	prioObjectPoolLifecycle  = 70802
	prioObjectCacheObjects   = 70803
	prioObjectCacheHits      = 70804
	
	// Security priorities
	prioSecurityAuthEvents   = 71003
	prioSecurityAuthTiming   = 71004
	prioSecurityAuthzEvents  = 71005
	prioSecurityAuthzTiming  = 71006

	// Monitoring (catch-all)
	prioMonitoringOther = 79000
)

// Note: All charts are now created dynamically using ensureChartExists() in collect_dynamic.go
// This provides proper NIDL compliance with one instance type per chart context
