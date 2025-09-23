package pmi

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"
)

// Config defines the user-facing configuration for the WebSphere PMI module.
type Config struct {
	framework.Config `yaml:",inline" json:",inline"`
	web.HTTPConfig   `yaml:",inline" json:",inline"`

	Vnode string `yaml:"vnode,omitempty" json:"vnode"`

	PMIStatsType        string   `yaml:"pmi_stats_type,omitempty" json:"pmi_stats_type"`
	PMIRefreshRate      int      `yaml:"pmi_refresh_rate,omitempty" json:"pmi_refresh_rate"`
	PMICustomStatsPaths []string `yaml:"pmi_custom_stats_paths,omitempty" json:"pmi_custom_stats_paths"`

	ClusterName  string            `yaml:"cluster_name,omitempty" json:"cluster_name"`
	CellName     string            `yaml:"cell_name,omitempty" json:"cell_name"`
	NodeName     string            `yaml:"node_name,omitempty" json:"node_name"`
	ServerType   string            `yaml:"server_type,omitempty" json:"server_type"`
	CustomLabels map[string]string `yaml:"custom_labels,omitempty" json:"custom_labels"`

	CollectJVMMetrics         *bool `yaml:"collect_jvm_metrics,omitempty" json:"collect_jvm_metrics"`
	CollectThreadPoolMetrics  *bool `yaml:"collect_threadpool_metrics,omitempty" json:"collect_threadpool_metrics"`
	CollectJDBCMetrics        *bool `yaml:"collect_jdbc_metrics,omitempty" json:"collect_jdbc_metrics"`
	CollectJCAMetrics         *bool `yaml:"collect_jca_metrics,omitempty" json:"collect_jca_metrics"`
	CollectJMSMetrics         *bool `yaml:"collect_jms_metrics,omitempty" json:"collect_jms_metrics"`
	CollectWebAppMetrics      *bool `yaml:"collect_webapp_metrics,omitempty" json:"collect_webapp_metrics"`
	CollectSessionMetrics     *bool `yaml:"collect_session_metrics,omitempty" json:"collect_session_metrics"`
	CollectTransactionMetrics *bool `yaml:"collect_transaction_metrics,omitempty" json:"collect_transaction_metrics"`
	CollectClusterMetrics     *bool `yaml:"collect_cluster_metrics,omitempty" json:"collect_cluster_metrics"`

	CollectServletMetrics *bool `yaml:"collect_servlet_metrics,omitempty" json:"collect_servlet_metrics"`
	CollectEJBMetrics     *bool `yaml:"collect_ejb_metrics,omitempty" json:"collect_ejb_metrics"`
	CollectJDBCAdvanced   *bool `yaml:"collect_jdbc_advanced,omitempty" json:"collect_jdbc_advanced"`

	MaxThreadPools     int `yaml:"max_threadpools,omitempty" json:"max_threadpools"`
	MaxJDBCPools       int `yaml:"max_jdbc_pools,omitempty" json:"max_jdbc_pools"`
	MaxJCAPools        int `yaml:"max_jca_pools,omitempty" json:"max_jca_pools"`
	MaxJMSDestinations int `yaml:"max_jms_destinations,omitempty" json:"max_jms_destinations"`
	MaxApplications    int `yaml:"max_applications,omitempty" json:"max_applications"`
	MaxServlets        int `yaml:"max_servlets,omitempty" json:"max_servlets"`
	MaxEJBs            int `yaml:"max_ejbs,omitempty" json:"max_ejbs"`

	CollectAppsMatching     string `yaml:"collect_apps_matching,omitempty" json:"collect_apps_matching"`
	CollectPoolsMatching    string `yaml:"collect_pools_matching,omitempty" json:"collect_pools_matching"`
	CollectJMSMatching      string `yaml:"collect_jms_matching,omitempty" json:"collect_jms_matching"`
	CollectServletsMatching string `yaml:"collect_servlets_matching,omitempty" json:"collect_servlets_matching"`
	CollectEJBsMatching     string `yaml:"collect_ejbs_matching,omitempty" json:"collect_ejbs_matching"`
}
