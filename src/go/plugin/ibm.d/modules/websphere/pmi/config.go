package pmi

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"
)

// Config defines the user-facing configuration for the WebSphere PMI module.
type Config struct {
	framework.Config `yaml:",inline" json:",inline"`
	web.HTTPConfig   `yaml:",inline" json:",inline"`

	// Vnode allows binding the collector to a virtual node.
	Vnode string `yaml:"vnode,omitempty" json:"vnode"`

	// PMIStatsType selects which PMI statistics tier to request (basic, extended, all).
	PMIStatsType string `yaml:"pmi_stats_type,omitempty" json:"pmi_stats_type"`

	// PMIRefreshRate overrides the PMI servlet refresh interval in seconds.
	PMIRefreshRate int `yaml:"pmi_refresh_rate,omitempty" json:"pmi_refresh_rate"`

	// PMICustomStatsPaths allows explicitly requesting additional PMI XML paths.
	PMICustomStatsPaths []string `yaml:"pmi_custom_stats_paths,omitempty" json:"pmi_custom_stats_paths"`

	// ClusterName appends a cluster label to every timeseries.
	ClusterName string `yaml:"cluster_name,omitempty" json:"cluster_name"`

	// CellName appends the cell label to every timeseries.
	CellName string `yaml:"cell_name,omitempty" json:"cell_name"`

	// NodeName appends the node label to every timeseries.
	NodeName string `yaml:"node_name,omitempty" json:"node_name"`

	// ServerType annotates metrics with the WebSphere server type (e.g. app_server, dmgr).
	ServerType string `yaml:"server_type,omitempty" json:"server_type"`

	// CustomLabels allows arbitrary additional static labels.
	CustomLabels map[string]string `yaml:"custom_labels,omitempty" json:"custom_labels"`

	// CollectJVMMetrics toggles JVM runtime metrics.
	CollectJVMMetrics *bool `yaml:"collect_jvm_metrics,omitempty" json:"collect_jvm_metrics"`

	// CollectThreadPoolMetrics toggles thread pool metrics.
	CollectThreadPoolMetrics *bool `yaml:"collect_threadpool_metrics,omitempty" json:"collect_threadpool_metrics"`

	// CollectJDBCMetrics toggles JDBC pool metrics.
	CollectJDBCMetrics *bool `yaml:"collect_jdbc_metrics,omitempty" json:"collect_jdbc_metrics"`

	// CollectJCAMetrics toggles JCA resource adapter metrics.
	CollectJCAMetrics *bool `yaml:"collect_jca_metrics,omitempty" json:"collect_jca_metrics"`

	// CollectJMSMetrics toggles JMS destination metrics.
	CollectJMSMetrics *bool `yaml:"collect_jms_metrics,omitempty" json:"collect_jms_metrics"`

	// CollectWebAppMetrics toggles Web application metrics.
	CollectWebAppMetrics *bool `yaml:"collect_webapp_metrics,omitempty" json:"collect_webapp_metrics"`

	// CollectSessionMetrics toggles HTTP session manager metrics.
	CollectSessionMetrics *bool `yaml:"collect_session_metrics,omitempty" json:"collect_session_metrics"`

	// CollectTransactionMetrics toggles transaction manager metrics.
	CollectTransactionMetrics *bool `yaml:"collect_transaction_metrics,omitempty" json:"collect_transaction_metrics"`

	// CollectClusterMetrics toggles cluster health metrics.
	CollectClusterMetrics *bool `yaml:"collect_cluster_metrics,omitempty" json:"collect_cluster_metrics"`

	// CollectServletMetrics toggles servlet response-time metrics.
	CollectServletMetrics *bool `yaml:"collect_servlet_metrics,omitempty" json:"collect_servlet_metrics"`

	// CollectEJBMetrics toggles Enterprise Java Bean metrics.
	CollectEJBMetrics *bool `yaml:"collect_ejb_metrics,omitempty" json:"collect_ejb_metrics"`

	// CollectJDBCAdvanced toggles advanced JDBC timing metrics.
	CollectJDBCAdvanced *bool `yaml:"collect_jdbc_advanced,omitempty" json:"collect_jdbc_advanced"`

	// MaxThreadPools caps the number of thread pools charted per server.
	MaxThreadPools int `yaml:"max_threadpools,omitempty" json:"max_threadpools"`

	// MaxJDBCPools caps the number of JDBC pools charted.
	MaxJDBCPools int `yaml:"max_jdbc_pools,omitempty" json:"max_jdbc_pools"`

	// MaxJCAPools caps the number of JCA pools charted.
	MaxJCAPools int `yaml:"max_jca_pools,omitempty" json:"max_jca_pools"`

	// MaxJMSDestinations caps the number of JMS destinations charted.
	MaxJMSDestinations int `yaml:"max_jms_destinations,omitempty" json:"max_jms_destinations"`

	// MaxApplications caps the number of web applications charted.
	MaxApplications int `yaml:"max_applications,omitempty" json:"max_applications"`

	// MaxServlets caps the number of servlets charted.
	MaxServlets int `yaml:"max_servlets,omitempty" json:"max_servlets"`

	// MaxEJBs caps the number of EJBs charted.
	MaxEJBs int `yaml:"max_ejbs,omitempty" json:"max_ejbs"`

	// CollectAppsMatching filters applications by name using glob patterns.
	CollectAppsMatching string `yaml:"collect_apps_matching,omitempty" json:"collect_apps_matching"`

	// CollectPoolsMatching filters pools (JDBC/JCA) by name using glob patterns.
	CollectPoolsMatching string `yaml:"collect_pools_matching,omitempty" json:"collect_pools_matching"`

	// CollectJMSMatching filters JMS destinations by name using glob patterns.
	CollectJMSMatching string `yaml:"collect_jms_matching,omitempty" json:"collect_jms_matching"`

	// CollectServletsMatching filters servlets by name using glob patterns.
	CollectServletsMatching string `yaml:"collect_servlets_matching,omitempty" json:"collect_servlets_matching"`

	// CollectEJBsMatching filters EJBs by name using glob patterns.
	CollectEJBsMatching string `yaml:"collect_ejbs_matching,omitempty" json:"collect_ejbs_matching"`
}
