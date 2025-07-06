// SPDX-License-Identifier: GPL-3.0-or-later

package websphere_pmi

import (
	"fmt"
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
	
	// Post-process thread pool idle calculations
	w.calculateThreadPoolIdleThreads(mx)
	
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
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		switch metric.Name {
		case "GlobalBegunCount":
			mx[fmt.Sprintf("transaction_manager_%s_global_begun", cleanInst)] = metric.Value
		case "CommittedCount":
			mx[fmt.Sprintf("transaction_manager_%s_global_committed", cleanInst)] = metric.Value
		case "LocalBegunCount":
			mx[fmt.Sprintf("transaction_manager_%s_local_begun", cleanInst)] = metric.Value
		case "LocalCommittedCount":
			mx[fmt.Sprintf("transaction_manager_%s_local_committed", cleanInst)] = metric.Value
		case "RolledbackCount":
			mx[fmt.Sprintf("transaction_manager_%s_global_rolled_back", cleanInst)] = metric.Value
		case "LocalRolledbackCount":
			mx[fmt.Sprintf("transaction_manager_%s_local_rolled_back", cleanInst)] = metric.Value
		case "GlobalTimeoutCount":
			mx[fmt.Sprintf("transaction_manager_%s_global_timeout", cleanInst)] = metric.Value
		case "LocalTimeoutCount":
			mx[fmt.Sprintf("transaction_manager_%s_local_timeout", cleanInst)] = metric.Value
		default:
			// For unknown count metrics, use collection helper
			w.collectCountMetric(mx, "transaction_manager", cleanInst, metric)
		}
	}
	
	// Extract ALL TimeStatistics and process with smart processor
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	labels := append([]module.Label{
		{Key: "instance", Value: instance},
		{Key: "node", Value: nodeName},
		{Key: "server", Value: serverName},
	}, w.getVersionLabels()...)
	
	for i, metric := range timeMetrics {
		w.processTimeStatisticWithContext(
			"transaction_manager",
			cleanInst,
			labels,
			metric,
			mx,
			i*10, // Offset priority for each metric
		)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		w.collectRangeMetric(mx, "transaction_manager", cleanInst, metric)
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		w.collectBoundedRangeMetric(mx, "transaction_manager", cleanInst, metric)
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		w.collectAverageMetric(mx, "transaction_manager", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		w.collectDoubleMetric(mx, "transaction_manager", cleanInst, metric)
	}
}

// parseJVMRuntime handles JVM Runtime metrics
func (w *WebSpherePMI) parseJVMRuntime(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureJVMCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("jvm_runtime", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		switch metric.Name {
		case "FreeMemory":
			mx[fmt.Sprintf("jvm_memory_%s_free", cleanInst)] = metric.Value * 1024 // Convert KB to bytes
		case "UsedMemory":
			mx[fmt.Sprintf("jvm_memory_%s_used", cleanInst)] = metric.Value * 1024 // Convert KB to bytes
		case "UpTime":
			mx[fmt.Sprintf("jvm_uptime_%s_seconds", cleanInst)] = metric.Value / 1000 // Convert ms to seconds
		case "ProcessCpuUsage":
			mx[fmt.Sprintf("jvm_cpu_%s_usage", cleanInst)] = metric.Value
		case "Heap":
			mx[fmt.Sprintf("jvm_heap_%s_size", cleanInst)] = metric.Value * 1024 // Convert KB to bytes
		default:
			// For unknown count metrics, use collection helper
			w.collectCountMetric(mx, "jvm_runtime", cleanInst, metric)
		}
	}
	
	// Extract ALL TimeStatistics and process with smart processor
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	labels := append([]module.Label{
		{Key: "instance", Value: instance},
		{Key: "node", Value: nodeName},
		{Key: "server", Value: serverName},
	}, w.getVersionLabels()...)
	
	for i, metric := range timeMetrics {
		w.processTimeStatisticWithContext(
			"jvm_runtime",
			cleanInst,
			labels,
			metric,
			mx,
			100+i*10, // Offset priority after main charts
		)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		// Use collection helper for all range metrics
		w.collectRangeMetric(mx, "jvm_runtime", cleanInst, metric)
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		// Use collection helper for all bounded range metrics
		w.collectBoundedRangeMetric(mx, "jvm_runtime", cleanInst, metric)
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		// Use collection helper for all average metrics
		w.collectAverageMetric(mx, "jvm_runtime", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		// Use collection helper for all double metrics
		w.collectDoubleMetric(mx, "jvm_runtime", cleanInst, metric)
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
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		// Check if MaxPoolSize exists and add chart if needed
		if metric.Name == "MaxPoolSize" {
			w.ensureThreadPoolMaxPoolSizeChart(instance, nodeName, serverName, poolName)
		}
		// Use collection helper for all count metrics
		w.collectCountMetric(mx, "thread_pool", cleanInst, metric)
	}
	
	// Extract ALL TimeStatistics and process with smart processor
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	labels := append([]module.Label{
		{Key: "instance", Value: instance},
		{Key: "node", Value: nodeName},
		{Key: "server", Value: serverName},
		{Key: "pool", Value: poolName},
	}, w.getVersionLabels()...)
	
	for i, metric := range timeMetrics {
		w.processTimeStatisticWithContext(
			"thread_pool",
			cleanInst,
			labels,
			metric,
			mx,
			100+i*10, // Offset priority after main charts
		)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		// Use collection helper for all range metrics
		w.collectRangeMetric(mx, "thread_pool", cleanInst, metric)
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		switch metric.Name {
		case "ActiveCount":
			mx[fmt.Sprintf("thread_pool_%s_active", cleanInst)] = metric.Value
		case "PoolSize":
			// Store the size temporarily for idle calculation
			poolSize := metric.Value
			sizeKey := fmt.Sprintf("thread_pool_%s_size", cleanInst)
			mx[sizeKey] = poolSize
			
			// Calculate idle threads immediately if we have active count
			activeKey := fmt.Sprintf("thread_pool_%s_active", cleanInst)
			if activeCount, exists := mx[activeKey]; exists {
				mx[fmt.Sprintf("thread_pool_%s_idle", cleanInst)] = poolSize - activeCount
			}
		default:
			// For unknown bounded range metrics, use collection helper
			w.collectBoundedRangeMetric(mx, "thread_pool", cleanInst, metric)
		}
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		// Use collection helper for all average metrics
		w.collectAverageMetric(mx, "thread_pool", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		// Use collection helper for all double metrics
		w.collectDoubleMetric(mx, "thread_pool", cleanInst, metric)
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
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		switch metric.Name {
		case "CreateCount":
			mx[fmt.Sprintf("jdbc_%s_connections_created", cleanInst)] = metric.Value
		case "CloseCount":
			mx[fmt.Sprintf("jdbc_%s_connections_closed", cleanInst)] = metric.Value
		case "AllocateCount":
			mx[fmt.Sprintf("jdbc_%s_connections_allocated", cleanInst)] = metric.Value
		case "ReturnCount":
			mx[fmt.Sprintf("jdbc_%s_connections_returned", cleanInst)] = metric.Value
		default:
			// For unknown count metrics, use collection helper
			w.collectCountMetric(mx, "jdbc", cleanInst, metric)
		}
	}
	
	// Extract ALL TimeStatistics and process with smart processor
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	labels := append([]module.Label{
		{Key: "instance", Value: instance},
		{Key: "node", Value: nodeName},
		{Key: "server", Value: serverName},
		{Key: "datasource", Value: dsName},
	}, w.getVersionLabels()...)
	
	for i, metric := range timeMetrics {
		// For WaitTime and UseTime, still collect the total for backward compatibility
		switch metric.Name {
		case "WaitTime":
			mx[fmt.Sprintf("jdbc_%s_wait_time_total", cleanInst)] = metric.Total
		case "UseTime":
			mx[fmt.Sprintf("jdbc_%s_use_time_total", cleanInst)] = metric.Total
		}
		
		// Process all TimeStatistics with the smart processor
		w.processTimeStatisticWithContext(
			"jdbc",
			cleanInst,
			labels,
			metric,
			mx,
			100+i*10, // Offset priority after main charts
		)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		switch metric.Name {
		case "WaitingThreadCount":
			mx[fmt.Sprintf("jdbc_%s_waiting_threads", cleanInst)] = metric.Current
		default:
			// For unknown range metrics, use collection helper
			w.collectRangeMetric(mx, "jdbc", cleanInst, metric)
		}
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		switch metric.Name {
		case "FreePoolSize":
			mx[fmt.Sprintf("jdbc_%s_pool_free", cleanInst)] = metric.Value
		case "PoolSize":
			mx[fmt.Sprintf("jdbc_%s_pool_size", cleanInst)] = metric.Value
		default:
			// For unknown bounded range metrics, use collection helper
			w.collectBoundedRangeMetric(mx, "jdbc", cleanInst, metric)
		}
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		// Use collection helper for all average metrics
		w.collectAverageMetric(mx, "jdbc", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		// Use collection helper for all double metrics
		w.collectDoubleMetric(mx, "jdbc", cleanInst, metric)
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

func (w *WebSpherePMI) ensureThreadPoolMaxPoolSizeChart(instance, nodeName, serverName, poolName string) {
	chartKey := fmt.Sprintf("thread_pool_max_pool_size_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		// Create MaxPoolSize chart (v9.0.5+ only)
		chart := threadPoolMaxPoolSizeChartTmpl.Copy()
		chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
		chart.Labels = []module.Label{
			{Key: "node", Value: nodeName},
			{Key: "server", Value: serverName},
			{Key: "pool", Value: poolName},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
		}
		
		if err := w.charts.Add(chart); err != nil {
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
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		switch metric.Name {
		case "ObjectsCreatedCount":
			mx[fmt.Sprintf("object_pool_%s_created", cleanInst)] = metric.Value
		default:
			// For unknown count metrics, use collection helper
			w.collectCountMetric(mx, "object_pool", cleanInst, metric)
		}
	}
	
	// Extract ALL TimeStatistics and process with smart processor
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	labels := append([]module.Label{
		{Key: "instance", Value: instance},
		{Key: "node", Value: nodeName},
		{Key: "server", Value: serverName},
		{Key: "pool", Value: poolName},
	}, w.getVersionLabels()...)
	
	for i, metric := range timeMetrics {
		w.processTimeStatisticWithContext(
			"object_pool",
			cleanInst,
			labels,
			metric,
			mx,
			100+i*10, // Offset priority after main charts
		)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		// Use collection helper for all range metrics
		w.collectRangeMetric(mx, "object_pool", cleanInst, metric)
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		switch metric.Name {
		case "ObjectsAllocatedCount":
			mx[fmt.Sprintf("object_pool_%s_allocated", cleanInst)] = metric.Value
		case "ObjectsReturnedCount":
			mx[fmt.Sprintf("object_pool_%s_returned", cleanInst)] = metric.Value
		case "IdleObjectsSize":
			mx[fmt.Sprintf("object_pool_%s_idle", cleanInst)] = metric.Value
		default:
			// For unknown bounded range metrics, use collection helper
			w.collectBoundedRangeMetric(mx, "object_pool", cleanInst, metric)
		}
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		// Use collection helper for all average metrics
		w.collectAverageMetric(mx, "object_pool", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		// Use collection helper for all double metrics
		w.collectDoubleMetric(mx, "object_pool", cleanInst, metric)
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

// calculateThreadPoolIdleThreads calculates idle threads for all thread pools
func (w *WebSpherePMI) calculateThreadPoolIdleThreads(mx map[string]int64) {
	// Find all thread pool instances
	threadPools := make(map[string]bool)
	for key := range mx {
		if strings.HasPrefix(key, "thread_pool_") && strings.HasSuffix(key, "_size") {
			// Extract instance name
			instance := strings.TrimPrefix(key, "thread_pool_")
			instance = strings.TrimSuffix(instance, "_size")
			threadPools[instance] = true
		}
	}
	
	// Calculate idle for each thread pool
	for instance := range threadPools {
		sizeKey := fmt.Sprintf("thread_pool_%s_size", instance)
		activeKey := fmt.Sprintf("thread_pool_%s_active", instance)
		idleKey := fmt.Sprintf("thread_pool_%s_idle", instance)
		
		// Only calculate if we have both size and active
		if size, hasSize := mx[sizeKey]; hasSize {
			if active, hasActive := mx[activeKey]; hasActive {
				mx[idleKey] = size - active
			}
			// If we don't have active count, we can't calculate idle
			// Don't set any values - let Netdata show the gap
			
			// Remove the size metric as it's no longer needed
			delete(mx, sizeKey)
		}
	}
}

// resetSeenTracking resets the seen instances map for this collection cycle
func (w *WebSpherePMI) resetSeenTracking() {
	w.seenInstances = make(map[string]bool)
}

// cleanupAbsentInstances removes charts for instances that are no longer seen
func (w *WebSpherePMI) cleanupAbsentInstances() {
	// Check which instances were collected but not seen
	for key, collected := range w.collectedInstances {
		if collected && !w.seenInstances[key] {
			// Instance disappeared - mark its charts as obsolete
			w.removeInstanceCharts(key)
			delete(w.collectedInstances, key)
		}
	}
}

// removeInstanceCharts marks all charts for a specific instance as obsolete
func (w *WebSpherePMI) removeInstanceCharts(instanceKey string) {
	// Parse the instance key to determine what type of charts to remove
	parts := strings.Split(instanceKey, "_")
	if len(parts) < 2 {
		return
	}
	
	category := parts[0]
	instance := strings.Join(parts[1:], "_")
	
	// Mark charts as obsolete based on category
	switch category {
	case "threadpool":
		w.removeThreadPoolCharts(instance)
	case "jdbc":
		w.removeJDBCCharts(instance)
	case "jca":
		w.removeJCACharts(instance)
	case "servlet":
		w.removeServletCharts(instance)
	case "session":
		w.removeSessionCharts(instance)
	case "portlet":
		w.removePortletCharts(instance)
	case "cache":
		w.removeCacheCharts(instance)
	case "interceptor":
		w.removeInterceptorCharts(instance)
	case "objectpool":
		w.removeObjectPoolCharts(instance)
	}
}

// removeThreadPoolCharts removes charts for a specific thread pool
func (w *WebSpherePMI) removeThreadPoolCharts(cleanName string) {
	prefix := fmt.Sprintf("thread_pool_%s_", cleanName)
	w.markChartsObsolete(prefix)
}

// removeJDBCCharts removes charts for a specific JDBC datasource
func (w *WebSpherePMI) removeJDBCCharts(cleanName string) {
	prefix := fmt.Sprintf("jdbc_%s_", cleanName)
	w.markChartsObsolete(prefix)
}

// removeJCACharts removes charts for a specific JCA connection factory
func (w *WebSpherePMI) removeJCACharts(cleanName string) {
	prefix := fmt.Sprintf("jca_%s_", cleanName)
	w.markChartsObsolete(prefix)
}

// removeServletCharts removes charts for a specific servlet
func (w *WebSpherePMI) removeServletCharts(cleanName string) {
	prefix := fmt.Sprintf("servlet_%s_", cleanName)
	w.markChartsObsolete(prefix)
}

// removeSessionCharts removes charts for a specific session manager
func (w *WebSpherePMI) removeSessionCharts(cleanName string) {
	prefix := fmt.Sprintf("session_%s_", cleanName)
	w.markChartsObsolete(prefix)
}

// removePortletCharts removes charts for a specific portlet
func (w *WebSpherePMI) removePortletCharts(cleanName string) {
	prefix := fmt.Sprintf("portlet_%s_", cleanName)
	w.markChartsObsolete(prefix)
}

// removeCacheCharts removes charts for a specific cache instance
func (w *WebSpherePMI) removeCacheCharts(cleanName string) {
	prefix := fmt.Sprintf("cache_%s_", cleanName)
	w.markChartsObsolete(prefix)
}

// removeInterceptorCharts removes charts for a specific interceptor
func (w *WebSpherePMI) removeInterceptorCharts(cleanName string) {
	prefix := fmt.Sprintf("interceptor_%s_", cleanName)
	w.markChartsObsolete(prefix)
}

// removeObjectPoolCharts removes charts for a specific object pool
func (w *WebSpherePMI) removeObjectPoolCharts(cleanName string) {
	prefix := fmt.Sprintf("object_pool_%s_", cleanName)
	w.markChartsObsolete(prefix)
}

// markChartsObsolete marks all charts with the given prefix as obsolete
func (w *WebSpherePMI) markChartsObsolete(prefix string) {
	for _, chart := range *w.charts {
		if strings.HasPrefix(chart.ID, prefix) && !chart.Obsolete {
			chart.Obsolete = true
			chart.MarkNotCreated() // Reset created flag to trigger CHART command
			w.Debugf("Marked chart %s as obsolete", chart.ID)
		}
	}
}

