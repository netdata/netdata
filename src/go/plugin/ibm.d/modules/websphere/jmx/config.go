package jmx

import (
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"
)

const (
	defaultUpdateEvery   = 5 * time.Second
	defaultScrapeTimeout = 5 * time.Second
	defaultInitTimeout   = 30 * time.Second
	defaultShutdownDelay = 100 * time.Millisecond
)

// Config captures user options for the WebSphere JMX framework module.
type Config struct {
	framework.Config `yaml:",inline" json:",inline"`

	Vnode string `yaml:"vnode,omitempty" json:"vnode"`

	// Connection settings
	JMXURL       string `yaml:"jmx_url" json:"jmx_url"`
	JMXUsername  string `yaml:"jmx_username,omitempty" json:"jmx_username"`
	JMXPassword  string `yaml:"jmx_password,omitempty" json:"jmx_password"`
	JMXClasspath string `yaml:"jmx_classpath,omitempty" json:"jmx_classpath"`
	JavaExecPath string `yaml:"java_exec_path,omitempty" json:"java_exec_path"`

	JMXTimeout    confopt.Duration `yaml:"jmx_timeout,omitempty" json:"jmx_timeout"`
	InitTimeout   confopt.Duration `yaml:"init_timeout,omitempty" json:"init_timeout"`
	ShutdownDelay confopt.Duration `yaml:"shutdown_delay,omitempty" json:"shutdown_delay"`

	// Identity labels
	ClusterName  string            `yaml:"cluster_name,omitempty" json:"cluster_name"`
	CellName     string            `yaml:"cell_name,omitempty" json:"cell_name"`
	NodeName     string            `yaml:"node_name,omitempty" json:"node_name"`
	ServerName   string            `yaml:"server_name,omitempty" json:"server_name"`
	ServerType   string            `yaml:"server_type,omitempty" json:"server_type"`
	CustomLabels map[string]string `yaml:"custom_labels,omitempty" json:"custom_labels"`

	// Metric toggles
	CollectJVMMetrics         bool `yaml:"collect_jvm_metrics" json:"collect_jvm_metrics"`
	CollectThreadPoolMetrics  bool `yaml:"collect_threadpool_metrics" json:"collect_threadpool_metrics"`
	CollectJDBCMetrics        bool `yaml:"collect_jdbc_metrics" json:"collect_jdbc_metrics"`
	CollectJCAMetrics         bool `yaml:"collect_jca_metrics" json:"collect_jca_metrics"`
	CollectJMSMetrics         bool `yaml:"collect_jms_metrics" json:"collect_jms_metrics"`
	CollectWebAppMetrics      bool `yaml:"collect_webapp_metrics" json:"collect_webapp_metrics"`
	CollectSessionMetrics     bool `yaml:"collect_session_metrics" json:"collect_session_metrics"`
	CollectTransactionMetrics bool `yaml:"collect_transaction_metrics" json:"collect_transaction_metrics"`
	CollectClusterMetrics     bool `yaml:"collect_cluster_metrics" json:"collect_cluster_metrics"`
	CollectServletMetrics     bool `yaml:"collect_servlet_metrics" json:"collect_servlet_metrics"`
	CollectEJBMetrics         bool `yaml:"collect_ejb_metrics" json:"collect_ejb_metrics"`
	CollectJDBCAdvanced       bool `yaml:"collect_jdbc_advanced" json:"collect_jdbc_advanced"`

	// Cardinality guards
	MaxThreadPools     int `yaml:"max_threadpools,omitempty" json:"max_threadpools"`
	MaxJDBCPools       int `yaml:"max_jdbc_pools,omitempty" json:"max_jdbc_pools"`
	MaxJCAPools        int `yaml:"max_jca_pools,omitempty" json:"max_jca_pools"`
	MaxJMSDestinations int `yaml:"max_jms_destinations,omitempty" json:"max_jms_destinations"`
	MaxApplications    int `yaml:"max_applications,omitempty" json:"max_applications"`
	MaxServlets        int `yaml:"max_servlets,omitempty" json:"max_servlets"`
	MaxEJBs            int `yaml:"max_ejbs,omitempty" json:"max_ejbs"`

	// Filters
	CollectPoolsMatching    string `yaml:"collect_pools_matching,omitempty" json:"collect_pools_matching"`
	CollectJMSMatching      string `yaml:"collect_jms_matching,omitempty" json:"collect_jms_matching"`
	CollectAppsMatching     string `yaml:"collect_apps_matching,omitempty" json:"collect_apps_matching"`
	CollectServletsMatching string `yaml:"collect_servlets_matching,omitempty" json:"collect_servlets_matching"`
	CollectEJBsMatching     string `yaml:"collect_ejbs_matching,omitempty" json:"collect_ejbs_matching"`

	// Resilience tuning
	MaxRetries              int     `yaml:"max_retries,omitempty" json:"max_retries"`
	RetryBackoffMultiplier  float64 `yaml:"retry_backoff_multiplier,omitempty" json:"retry_backoff_multiplier"`
	CircuitBreakerThreshold int     `yaml:"circuit_breaker_threshold,omitempty" json:"circuit_breaker_threshold"`
	HelperRestartMax        int     `yaml:"helper_restart_max,omitempty" json:"helper_restart_max"`
}
