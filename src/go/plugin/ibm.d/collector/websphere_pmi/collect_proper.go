// SPDX-License-Identifier: GPL-3.0-or-later

package websphere_pmi

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

// collectProper implements a proper collection based on ACTUAL XML structure
func (w *WebSpherePMI) collectProper(stats *pmiStatsResponse) map[string]int64 {
	mx := make(map[string]int64)
	
	// Track what we've seen this cycle
	w.resetSeenTracking()
	
	// Process each node/server hierarchy
	for _, node := range stats.Nodes {
		for _, server := range node.Servers {
			w.parseServerStats(node.Name, server.Name, server.Stats, mx, []string{node.Name, server.Name})
		}
	}
	
	// Process direct stats (for Liberty)
	if len(stats.Stats) > 0 {
		w.parseServerStats("local", "server", stats.Stats, mx, []string{})
	}
	
	// Clean up absent instances
	w.cleanupAbsentInstances()
	
	return mx
}

// parseServerStats recursively parses all stats based on ACTUAL XML structure
func (w *WebSpherePMI) parseServerStats(nodeName, serverName string, stats []pmiStat, mx map[string]int64, path []string) {
	for _, stat := range stats {
		currentPath := append(path, stat.Name)
		
		// Handle specific stat types based on their name and position
		switch stat.Name {
		case "Transaction Manager":
			w.parseTransactionManager(&stat, nodeName, serverName, mx)
			
		case "JVM Runtime":
			w.parseJVMRuntime(&stat, nodeName, serverName, mx)
			
		case "Thread Pools":
			// Parse child thread pools
			for _, childStat := range stat.SubStats {
				w.parseThreadPool(&childStat, nodeName, serverName, mx)
			}
			
		case "JDBC Connection Pools":
			// Parse as container and its children
			w.parseJDBCContainer(&stat, nodeName, serverName, mx)
			
		case "Web Applications":
			// Parse as container and its children
			w.parseWebApplicationsContainer(&stat, nodeName, serverName, mx)
			
		case "Servlet Session Manager":
			// Parse session manager and its children
			w.parseSessionManagerContainer(&stat, nodeName, serverName, mx)
			
		case "Dynamic Caching":
			// Parse dynamic caching container and its child object caches
			w.parseDynamicCacheContainer(&stat, nodeName, serverName, mx)
			
		case "ORB":
			w.parseORB(&stat, nodeName, serverName, mx)
			
		// Security metrics
		case "Security Authentication":
			w.parseSecurityAuthentication(&stat, nodeName, serverName, mx)
			
		case "Security Authorization":
			w.parseSecurityAuthorization(&stat, nodeName, serverName, mx)
			
		// Individual Thread Pools (appear as separate stats)
		case "AriesThreadPool", "Default", "HAManager.thread.pool", "Message Listener", 
			 "Object Request Broker", "SIBFAPInboundThreadPool", "SIBFAPThreadPool", 
			 "SoapConnectorThreadPool", "TCPChannel DCS", "WMQJCAResourceAdapter", "WebContainer":
			w.parseThreadPool(&stat, nodeName, serverName, mx)
			
		// Individual JDBC Providers
		case "Derby JDBC Provider (XA)":
			// Parse as JDBC provider with datasources
			for _, ds := range stat.SubStats {
				dsInstance := fmt.Sprintf("%s.%s.%s.%s", nodeName, serverName, stat.Name, ds.Name)
				w.parseJDBCDataSource(&ds, dsInstance, nodeName, serverName, stat.Name, ds.Name, mx)
			}
			
		// Object Pools
		case "Object Pool":
			w.parseObjectPool(&stat, nodeName, serverName, mx)
			
		// Object Cache metrics are handled within Dynamic Caching - no standalone case needed
			
		// JCA Connection Pools
		case "J2C Connection Pools":
			// Parse JCA connection pools
			for _, pool := range stat.SubStats {
				w.parseJCAConnectionPool(&pool, nodeName, serverName, mx)
			}
			
		// Enterprise Application stats
		case "Enterprise Applications":
			// Parse enterprise applications
			for _, app := range stat.SubStats {
				w.parseEnterpriseApplication(&app, nodeName, serverName, mx)
			}
			
		// System Data stats
		case "System Data":
			w.parseSystemData(&stat, nodeName, serverName, mx)
			
		// Work Load Management
		case "WLM":
			w.parseWLM(&stat, nodeName, serverName, mx)
			
		// Bean Manager
		case "Bean Manager":
			w.parseBeanManager(&stat, nodeName, serverName, mx)
			
		// Connection Manager
		case "Connection Manager":
			w.parseConnectionManager(&stat, nodeName, serverName, mx)
			
		// JVM specific stats
		case "JVM.GC", "JVM.Memory", "JVM.Thread":
			w.parseJVMSubsystem(&stat, nodeName, serverName, mx)
			
		// Individual EJB modules and beans
		case "EJB":
			w.parseEJBContainer(&stat, nodeName, serverName, mx)
			
		// Message Driven Beans
		case "Message Driven Beans":
			// Parse child MDBs
			for _, mdb := range stat.SubStats {
				w.parseMDB(&mdb, nodeName, serverName, mx)
			}
			
		// Stateful Session Beans
		case "Stateful Session Beans":
			// Parse child SFSBs
			for _, sfsb := range stat.SubStats {
				w.parseStatefulSessionBean(&sfsb, nodeName, serverName, mx)
			}
			
		// Stateless Session Beans
		case "Stateless Session Beans":
			// Parse child SLSBs
			for _, slsb := range stat.SubStats {
				w.parseStatelessSessionBean(&slsb, nodeName, serverName, mx)
			}
			
		// Entity Beans
		case "Entity Beans":
			// Parse child entity beans
			for _, eb := range stat.SubStats {
				w.parseEntityBean(&eb, nodeName, serverName, mx)
			}
			
		// Interceptors and ORB components
		case "Interceptors":
			w.parseInterceptorContainer(&stat, nodeName, serverName, mx)
			
		// Individual Portlets 
		case "Portlets":
			// Process children through normal routing to filter properly
			if len(stat.SubStats) > 0 {
				w.parseServerStats(nodeName, serverName, stat.SubStats, mx, currentPath)
			}
			
		// Portlet Application container
		case "Portlet Application":
			// This is a container - process its children through normal routing
			// Don't force them all to parsePortlet
			if len(stat.SubStats) > 0 {
				w.parseServerStats(nodeName, serverName, stat.SubStats, mx, currentPath)
			}
			
		// Web Service modules
		case "pmiWebServiceModule":
			// This is a container - process its children as web service modules
			for _, child := range stat.SubStats {
				// Route .war files with . delimiter to web service parser
				w.parseWebServiceModule(&child, nodeName, serverName, mx)
			}
			
		// URLs and servlet URL mappings
		case "URLs":
			w.parseURLContainer(&stat, nodeName, serverName, mx)
			
		// HA Manager metrics
		case "HAManager":
			// HAManager container - check for HAManagerMBean child
			for _, child := range stat.SubStats {
				if child.Name == "HAManagerMBean" {
					w.parseHAManager(&child, nodeName, serverName, mx)
				}
			}
			// Also check if HAManager itself has metrics
			if len(stat.BoundedRangeStatistics) > 0 || len(stat.TimeStatistics) > 0 {
				w.parseHAManager(&stat, nodeName, serverName, mx)
			}
			
		case "HAManagerMBean":
			// Direct HAManagerMBean
			w.parseHAManager(&stat, nodeName, serverName, mx)
		
		default:
			// Handle JDBC datasources (start with "jdbc/")
			if strings.HasPrefix(stat.Name, "jdbc/") {
				w.parseJDBCDataSource(&stat, fmt.Sprintf("%s.%s.%s", nodeName, serverName, stat.Name), nodeName, serverName, "JDBC", stat.Name, mx)
			} else if strings.HasPrefix(stat.Name, "jms/") {
				// Handle JMS connection factories
				w.parseJCAConnectionPool(&stat, nodeName, serverName, mx)
			} else if strings.HasPrefix(stat.Name, "Object: ") {
				// Handle individual cache objects
				w.parseCacheObject(&stat, nodeName, serverName, mx)
			} else if strings.HasPrefix(stat.Name, "ObjectPool_") {
				// Handle Object Pools with dynamic names
				w.parseObjectPool(&stat, nodeName, serverName, mx)
			} else if (strings.Contains(stat.Name, "#") || strings.Contains(stat.Name, ".war")) && len(currentPath) > 2 && currentPath[len(currentPath)-2] == "Web Applications" {
				// Check if it's a web application under "Web Applications" container (either # format or .war format)
				w.parseWebApplication(&stat, nodeName, serverName, mx, currentPath)
			} else if len(currentPath) > 3 && currentPath[len(currentPath)-2] == "Servlets" {
				// Handle individual servlet URLs (detected by path structure)
				w.parseServlet(&stat, nodeName, serverName, currentPath[len(currentPath)-3], mx)
			} else if strings.Contains(stat.Name, "Bean") && len(stat.CountStatistics) > 0 {
				// Handle individual EJB beans (typically have bean-specific names)
				w.parseIndividualEJB(&stat, nodeName, serverName, mx, currentPath)
			} else if strings.Contains(stat.Name, "ResourceAdapter") || strings.Contains(stat.Name, "Connection Pool") {
				// Handle JCA connection pools with provider names
				w.parseJCAConnectionPool(&stat, nodeName, serverName, mx)
			} else if strings.Contains(stat.Name, "Interceptor") || strings.Contains(stat.Name, "com.ibm") {
				// Handle individual interceptors
				w.parseIndividualInterceptor(&stat, nodeName, serverName, mx)
			} else if strings.Contains(stat.Name, "Portlet") && !strings.Contains(stat.Name, ".war") && 
				stat.Name != "Portlet Application" && stat.Name != "Details" {
				// Handle individual portlets (but not .war files, containers, or Details)
				w.parsePortlet(&stat, nodeName, serverName, mx)
			} else if strings.HasPrefix(stat.Name, "/") && strings.Contains(stat.Name, "servlet") {
				// Handle servlet URLs 
				w.parseServletURL(&stat, nodeName, serverName, mx)
			} else if strings.Contains(stat.Name, ".war") {
				// Check if this .war file has ServicesLoaded (web service module metrics)
				hasServicesLoaded := false
				hasSessionMetrics := false
				for _, cs := range stat.CountStatistics {
					if cs.Name == "ServicesLoaded" {
						hasServicesLoaded = true
						break
					}
					if cs.Name == "CreateCount" || cs.Name == "InvalidateCount" || cs.Name == "ActiveCount" {
						hasSessionMetrics = true
					}
				}
				if hasServicesLoaded {
					// Route to web service module parser 
					w.parseWebServiceModule(&stat, nodeName, serverName, mx)
				} else if hasSessionMetrics || strings.Contains(stat.Name, "#") {
					// Route to session metrics parser
					instance := fmt.Sprintf("%s.%s.%s", nodeName, serverName, stat.Name)
					w.parseSessionMetrics(&stat, instance, nodeName, serverName, stat.Name, mx)
				} else {
					// Other .war metrics go to generic
					w.parseGenericStat(&stat, nodeName, serverName, mx, currentPath)
				}
			} else if stat.Name == "Object Cache" || stat.Name == "Counters" {
				// Cache-related components with operational metrics
				w.parseCacheComponent(&stat, nodeName, serverName, mx)
			} else if strings.Contains(stat.Name, "ExtensionRegistryStats") {
				// Extension registry component metrics
				w.parseExtensionRegistryStats(&stat, nodeName, serverName, mx)
			} else if strings.Contains(stat.Name, "SIB JMS Resource Adapter") {
				// Service Integration Bus JMS adapter metrics
				w.parseSIBJMSAdapter(&stat, nodeName, serverName, mx)
			} else if stat.Name == "Servlets" && len(currentPath) > 2 && currentPath[len(currentPath)-2] != "Web Applications" {
				// Standalone Servlets component (not under Web Applications)
				w.parseServletsComponent(&stat, nodeName, serverName, mx)
			} else if strings.Contains(stat.Name, "WIM") && (strings.Contains(stat.Name, "User") || strings.Contains(stat.Name, "Group")) {
				// WebSphere Identity Manager (WIM) user/group management metrics
				w.parseWIMComponent(&stat, nodeName, serverName, mx)
			} else if strings.Contains(stat.Name, "WLMTaggedComponentManager") {
				// Workload Management Tagged Component Manager metrics
				w.parseWLMTaggedComponentManager(&stat, nodeName, serverName, mx)
			} else if strings.Contains(stat.Name, "pmiWebServiceService") {
				// PMI Web Service Service metrics
				w.parsePMIWebServiceService(&stat, nodeName, serverName, mx)
			} else if strings.Contains(stat.Name, "TCPChannel") && strings.Contains(stat.Name, "DCS") {
				// TCP Channel DCS (Data Communication Services) metrics
				w.parseTCPChannelDCS(&stat, nodeName, serverName, mx)
			} else if stat.Name == "Details" && len(currentPath) > 1 {
				// Details component metrics (context-sensitive)
				w.parseDetailsComponent(&stat, nodeName, serverName, mx, currentPath)
			} else if strings.Contains(stat.Name, "ISCProductDetails") {
				// IBM Support Center Product Details metrics
				w.parseISCProductDetails(&stat, nodeName, serverName, mx)
			} else {
				// Generic stat parser for truly unhandled stat types
				w.parseGenericStat(&stat, nodeName, serverName, mx, currentPath)
			}
		}
		
		// Always parse child stats recursively
		if len(stat.SubStats) > 0 {
			w.parseServerStats(nodeName, serverName, stat.SubStats, mx, currentPath)
		}
	}
}

// parseTransactionManager handles Transaction Manager metrics
func (w *WebSpherePMI) parseTransactionManager(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureTransactionCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("transaction_manager", instance)
	
	// Process CountStatistics - based on ACTUAL XML
	for _, cs := range stat.CountStatistics {
		if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
			switch cs.Name {
			case "GlobalBegunCount":
				mx[fmt.Sprintf("transaction_manager_%s_global_begun", w.cleanID(instance))] = val
			case "CommittedCount":
				mx[fmt.Sprintf("transaction_manager_%s_global_committed", w.cleanID(instance))] = val
			case "LocalBegunCount":
				mx[fmt.Sprintf("transaction_manager_%s_local_begun", w.cleanID(instance))] = val
			case "LocalCommittedCount":
				mx[fmt.Sprintf("transaction_manager_%s_local_committed", w.cleanID(instance))] = val
			case "RolledbackCount":
				mx[fmt.Sprintf("transaction_manager_%s_global_rolled_back", w.cleanID(instance))] = val
			case "LocalRolledbackCount":
				mx[fmt.Sprintf("transaction_manager_%s_local_rolled_back", w.cleanID(instance))] = val
			case "GlobalTimeoutCount":
				mx[fmt.Sprintf("transaction_manager_%s_global_timeout", w.cleanID(instance))] = val
			case "LocalTimeoutCount":
				mx[fmt.Sprintf("transaction_manager_%s_local_timeout", w.cleanID(instance))] = val
			}
		} else {
			w.Errorf("ISSUE: parseTransactionManager failed to parse CountStatistic %s value '%s': %v", cs.Name, cs.Count, err)
		}
	}
}

// parseJVMRuntime handles JVM Runtime metrics
func (w *WebSpherePMI) parseJVMRuntime(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureJVMCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("jvm_runtime", instance)
	
	// Process CountStatistics - based on ACTUAL XML
	for _, cs := range stat.CountStatistics {
		if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
			switch cs.Name {
			case "FreeMemory":
				mx[fmt.Sprintf("jvm_memory_%s_free", w.cleanID(instance))] = val * 1024 // Convert KB to bytes
			case "UsedMemory":
				mx[fmt.Sprintf("jvm_memory_%s_used", w.cleanID(instance))] = val * 1024 // Convert KB to bytes
			case "UpTime":
				mx[fmt.Sprintf("jvm_uptime_%s_seconds", w.cleanID(instance))] = val / 1000 // Convert ms to seconds
			case "ProcessCpuUsage":
				mx[fmt.Sprintf("jvm_cpu_%s_usage", w.cleanID(instance))] = val
			case "Heap":
				mx[fmt.Sprintf("jvm_heap_%s_size", w.cleanID(instance))] = val * 1024 // Convert KB to bytes
			}
		} else {
			w.Errorf("ISSUE: parseJVMRuntime failed to parse CountStatistic %s value '%s': %v", cs.Name, cs.Count, err)
		}
	}
}

// parseThreadPool handles individual thread pool metrics
func (w *WebSpherePMI) parseThreadPool(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	poolName := stat.Name
	instance := fmt.Sprintf("%s.%s.%s", nodeName, serverName, poolName)
	
	// Create charts if not exists
	w.ensureThreadPoolCharts(instance, nodeName, serverName, poolName)
	
	// Mark as seen
	w.markInstanceSeen("thread_pool", instance)
	
	// Process BoundedRangeStatistics
	for _, brs := range stat.BoundedRangeStatistics {
		if val, err := strconv.ParseInt(brs.Current, 10, 64); err == nil {
			switch brs.Name {
			case "ActiveCount":
				mx[fmt.Sprintf("thread_pool_%s_active", w.cleanID(instance))] = val
			case "PoolSize":
				mx[fmt.Sprintf("thread_pool_%s_size", w.cleanID(instance))] = val
			}
		} else {
			w.Errorf("ISSUE: parseThreadPool failed to parse BoundedRangeStatistic %s current value '%s': %v", brs.Name, brs.Current, err)
		}
	}
}

// parseJDBCContainer handles JDBC container and children
func (w *WebSpherePMI) parseJDBCContainer(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	// Process child JDBC providers
	for _, provider := range stat.SubStats {
		// Each provider has datasources as children
		for _, ds := range provider.SubStats {
			dsInstance := fmt.Sprintf("%s.%s.%s.%s", nodeName, serverName, provider.Name, ds.Name)
			w.parseJDBCDataSource(&ds, dsInstance, nodeName, serverName, provider.Name, ds.Name, mx)
		}
	}
}

// parseJDBCDataSource handles individual JDBC datasource
func (w *WebSpherePMI) parseJDBCDataSource(stat *pmiStat, instance, nodeName, serverName, providerName, dsName string, mx map[string]int64) {
	// Create charts if not exists
	w.ensureJDBCCharts(instance, nodeName, serverName, providerName, dsName)
	
	// Mark as seen
	w.markInstanceSeen("jdbc_datasource", instance)
	
	// Process CountStatistics
	for _, cs := range stat.CountStatistics {
		if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
			switch cs.Name {
			case "CreateCount":
				mx[fmt.Sprintf("jdbc_%s_connections_created", w.cleanID(instance))] = val
			case "CloseCount":
				mx[fmt.Sprintf("jdbc_%s_connections_closed", w.cleanID(instance))] = val
			case "AllocateCount":
				mx[fmt.Sprintf("jdbc_%s_connections_allocated", w.cleanID(instance))] = val
			case "ReturnCount":
				mx[fmt.Sprintf("jdbc_%s_connections_returned", w.cleanID(instance))] = val
			}
		} else {
			w.Errorf("ISSUE: parseJDBCDataSource failed to parse CountStatistic %s value '%s': %v", cs.Name, cs.Count, err)
		}
	}
	
	// Process RangeStatistics
	for _, rs := range stat.RangeStatistics {
		if val, err := strconv.ParseInt(rs.Current, 10, 64); err == nil {
			switch rs.Name {
			case "WaitingThreadCount":
				mx[fmt.Sprintf("jdbc_%s_waiting_threads", w.cleanID(instance))] = val
			}
		} else {
			w.Errorf("ISSUE: parseJDBCDataSource failed to parse RangeStatistic %s current value '%s': %v", rs.Name, rs.Current, err)
		}
	}
	
	// Process BoundedRangeStatistics
	for _, brs := range stat.BoundedRangeStatistics {
		if val, err := strconv.ParseInt(brs.Current, 10, 64); err == nil {
			switch brs.Name {
			case "FreePoolSize":
				mx[fmt.Sprintf("jdbc_%s_pool_free", w.cleanID(instance))] = val
			case "PoolSize":
				mx[fmt.Sprintf("jdbc_%s_pool_size", w.cleanID(instance))] = val
			}
		} else {
			w.Errorf("ISSUE: parseJDBCDataSource failed to parse BoundedRangeStatistic %s current value '%s': %v", brs.Name, brs.Current, err)
		}
	}
	
	// Process TimeStatistics
	for _, ts := range stat.TimeStatistics {
		switch ts.Name {
		case "WaitTime":
			total := ts.TotalTime
			if total == "" {
				total = ts.Total
			}
			if val, err := strconv.ParseInt(total, 10, 64); err == nil {
				mx[fmt.Sprintf("jdbc_%s_wait_time_total", w.cleanID(instance))] = val
			} else {
				w.Errorf("ISSUE: parseJDBCDataSource failed to parse TimeStatistic %s total value '%s': %v", ts.Name, total, err)
			}
		case "UseTime":
			total := ts.TotalTime
			if total == "" {
				total = ts.Total
			}
			if val, err := strconv.ParseInt(total, 10, 64); err == nil {
				mx[fmt.Sprintf("jdbc_%s_use_time_total", w.cleanID(instance))] = val
			} else {
				w.Errorf("ISSUE: parseJDBCDataSource failed to parse TimeStatistic %s total value '%s': %v", ts.Name, total, err)
			}
		}
	}
}

// Helper functions for chart creation
func (w *WebSpherePMI) ensureTransactionCharts(instance, nodeName, serverName string) {
	chartKey := fmt.Sprintf("transaction_manager_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		// Create transaction charts
		charts := transactionChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
			}
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

func (w *WebSpherePMI) ensureJVMCharts(instance, nodeName, serverName string) {
	chartKey := fmt.Sprintf("jvm_runtime_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		// Create JVM charts
		charts := jvmChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
			}
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

func (w *WebSpherePMI) ensureThreadPoolCharts(instance, nodeName, serverName, poolName string) {
	chartKey := fmt.Sprintf("thread_pool_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		// Create thread pool charts
		charts := threadPoolChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
				{Key: "pool", Value: poolName},
			}
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

func (w *WebSpherePMI) ensureJDBCCharts(instance, nodeName, serverName, providerName, dsName string) {
	chartKey := fmt.Sprintf("jdbc_datasource_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		// Create JDBC charts
		charts := jdbcChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
				{Key: "provider", Value: providerName},
				{Key: "datasource", Value: dsName},
			}
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

// Additional parsing functions would follow the same pattern...
// parseWebApplicationsContainer, parseWebApplication, parseSessionManagerContainer, etc.

// parseObjectPool handles Object Pool metrics
func (w *WebSpherePMI) parseObjectPool(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	poolName := stat.Name
	instance := fmt.Sprintf("%s.%s.%s", nodeName, serverName, poolName)
	
	// Create charts if not exists
	w.ensureObjectPoolCharts(instance, nodeName, serverName, poolName)
	w.markInstanceSeen("object_pool", instance)
	
	// Process CountStatistics (like ObjectsCreatedCount)
	for _, cs := range stat.CountStatistics {
		if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
			switch cs.Name {
			case "ObjectsCreatedCount":
				mx[fmt.Sprintf("object_pool_%s_created", w.cleanID(instance))] = val
			}
		} else {
			w.Errorf("ISSUE: parseObjectPool failed to parse CountStatistic %s value '%s': %v", cs.Name, cs.Count, err)
		}
	}
	
	// Process BoundedRangeStatistics (like ObjectsAllocatedCount, ObjectsReturnedCount, IdleObjectsSize)
	// NOTE: XML 'value' attribute maps to struct 'Current' field
	for _, brs := range stat.BoundedRangeStatistics {
		if val, err := strconv.ParseInt(brs.Current, 10, 64); err == nil {
			switch brs.Name {
			case "ObjectsAllocatedCount":
				mx[fmt.Sprintf("object_pool_%s_allocated", w.cleanID(instance))] = val
			case "ObjectsReturnedCount":
				mx[fmt.Sprintf("object_pool_%s_returned", w.cleanID(instance))] = val
			case "IdleObjectsSize":
				mx[fmt.Sprintf("object_pool_%s_idle", w.cleanID(instance))] = val
			}
		} else {
			w.Errorf("ISSUE: parseObjectPool failed to parse BoundedRangeStatistic %s current value '%s': %v", brs.Name, brs.Current, err)
		}
	}
}

// parseObjectCache handles Object Cache metrics (different from Dynamic Caching)
func (w *WebSpherePMI) parseObjectCache(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	// Extract cache name from stat name like "Object: ws/com.ibm.workplace/ExtensionRegistryCache"
	// Note: cacheName extracted but using unified instance name for aggregation
	_ = strings.TrimPrefix(stat.Name, "Object: ")
	instance := fmt.Sprintf("%s.%s.Object_Cache", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureObjectCacheCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("object_cache", instance)
	
	// Process child stats (like "Object Cache" -> "Counters" or direct metrics)
	for _, child := range stat.SubStats {
		if child.Name == "Object Cache" {
			// Process direct object cache metrics at this level
			for _, cs := range child.CountStatistics {
				if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
					switch cs.Name {
					case "InMemoryCacheEntryCount":
						mx[fmt.Sprintf("object_cache_%s_objects", w.cleanID(instance))] = val
					case "MaxInMemoryCacheEntryCount":
						mx[fmt.Sprintf("object_cache_%s_max_objects", w.cleanID(instance))] = val
					}
				} else {
					w.Errorf("ISSUE: parseObjectCache failed to parse CountStatistic %s value '%s': %v", cs.Name, cs.Count, err)
				}
			}
			
			// Look for "Counters" sub-stat
			for _, counters := range child.SubStats {
				if counters.Name == "Counters" {
					// Process counter metrics
					for _, cs := range counters.CountStatistics {
						if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
							switch cs.Name {
							case "HitsInMemoryCount":
								mx[fmt.Sprintf("object_cache_%s_memory_hits", w.cleanID(instance))] = val
							case "HitsOnDiskCount":
								mx[fmt.Sprintf("object_cache_%s_disk_hits", w.cleanID(instance))] = val
							case "MissCount":
								mx[fmt.Sprintf("object_cache_%s_misses", w.cleanID(instance))] = val
							case "InMemoryAndDiskCacheEntryCount":
								mx[fmt.Sprintf("object_cache_%s_total_entries", w.cleanID(instance))] = val
							}
						} else {
							w.Errorf("ISSUE: parseObjectCache failed to parse CountStatistic %s value '%s': %v", cs.Name, cs.Count, err)
						}
					}
				}
			}
		}
	}
}

// Helper to clean IDs for use in metrics
func (w *WebSpherePMI) cleanID(s string) string {
	// Replace special characters with underscores
	r := strings.NewReplacer(
		".", "_",
		" ", "_",
		"-", "_",
		"#", "_",
		"/", "_",
		":", "_",
		"(", "_",
		")", "_",
		"[", "_",
		"]", "_",
	)
	return r.Replace(s)
}

// markInstanceSeen marks an instance as seen this collection cycle
func (w *WebSpherePMI) markInstanceSeen(category, instance string) {
	key := fmt.Sprintf("%s_%s", category, instance)
	w.seenInstances[key] = true
}

