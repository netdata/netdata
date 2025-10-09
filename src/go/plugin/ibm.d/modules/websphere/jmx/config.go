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

	Vnode string `yaml:"vnode,omitempty" json:"vnode" ui:"group:Connection"`

	// Connection settings
	JMXURL       string `yaml:"jmx_url" json:"jmx_url" ui:"group:Connection"`
	JMXUsername  string `yaml:"jmx_username,omitempty" json:"jmx_username" ui:"group:Connection"`
	JMXPassword  string `yaml:"jmx_password,omitempty" json:"jmx_password" ui:"group:Connection"`
	JMXClasspath string `yaml:"jmx_classpath,omitempty" json:"jmx_classpath" ui:"group:Connection"`
	JavaExecPath string `yaml:"java_exec_path,omitempty" json:"java_exec_path" ui:"group:Connection"`

	JMXTimeout    confopt.Duration `yaml:"jmx_timeout,omitempty" json:"jmx_timeout" ui:"group:Advanced"`
	InitTimeout   confopt.Duration `yaml:"init_timeout,omitempty" json:"init_timeout" ui:"group:Advanced"`
	ShutdownDelay confopt.Duration `yaml:"shutdown_delay,omitempty" json:"shutdown_delay" ui:"group:Advanced"`

	// Identity labels
	ClusterName  string            `yaml:"cluster_name,omitempty" json:"cluster_name" ui:"group:Identity"`
	CellName     string            `yaml:"cell_name,omitempty" json:"cell_name" ui:"group:Identity"`
	NodeName     string            `yaml:"node_name,omitempty" json:"node_name" ui:"group:Identity"`
	ServerName   string            `yaml:"server_name,omitempty" json:"server_name" ui:"group:Identity"`
	ServerType   string            `yaml:"server_type,omitempty" json:"server_type" ui:"group:Identity"`
	CustomLabels map[string]string `yaml:"custom_labels,omitempty" json:"custom_labels" ui:"group:Identity"`

	// Metric toggles
	CollectJVMMetrics         framework.AutoBool `yaml:"collect_jvm_metrics" json:"collect_jvm_metrics" ui:"group:Other Metrics"`
	CollectThreadPoolMetrics  framework.AutoBool `yaml:"collect_threadpool_metrics" json:"collect_threadpool_metrics" ui:"group:Thread Pools"`
	CollectJDBCMetrics        framework.AutoBool `yaml:"collect_jdbc_metrics" json:"collect_jdbc_metrics" ui:"group:JDBC"`
	CollectJCAMetrics         framework.AutoBool `yaml:"collect_jca_metrics" json:"collect_jca_metrics" ui:"group:JCA"`
	CollectJMSMetrics         framework.AutoBool `yaml:"collect_jms_metrics" json:"collect_jms_metrics" ui:"group:JMS"`
	CollectWebAppMetrics      framework.AutoBool `yaml:"collect_webapp_metrics" json:"collect_webapp_metrics" ui:"group:Applications"`
	CollectSessionMetrics     framework.AutoBool `yaml:"collect_session_metrics" json:"collect_session_metrics" ui:"group:Other Metrics"`
	CollectTransactionMetrics framework.AutoBool `yaml:"collect_transaction_metrics" json:"collect_transaction_metrics" ui:"group:Other Metrics"`
	CollectClusterMetrics     framework.AutoBool `yaml:"collect_cluster_metrics" json:"collect_cluster_metrics" ui:"group:Other Metrics"`
	CollectServletMetrics     framework.AutoBool `yaml:"collect_servlet_metrics" json:"collect_servlet_metrics" ui:"group:Servlets"`
	CollectEJBMetrics         framework.AutoBool `yaml:"collect_ejb_metrics" json:"collect_ejb_metrics" ui:"group:EJBs"`
	CollectJDBCAdvanced       framework.AutoBool `yaml:"collect_jdbc_advanced" json:"collect_jdbc_advanced" ui:"group:JDBC"`

	// Cardinality guards
	MaxThreadPools     int `yaml:"max_threadpools,omitempty" json:"max_threadpools" ui:"group:Thread Pools"`
	MaxJDBCPools       int `yaml:"max_jdbc_pools,omitempty" json:"max_jdbc_pools" ui:"group:JDBC"`
	MaxJCAPools        int `yaml:"max_jca_pools,omitempty" json:"max_jca_pools" ui:"group:JCA"`
	MaxJMSDestinations int `yaml:"max_jms_destinations,omitempty" json:"max_jms_destinations" ui:"group:JMS"`
	MaxApplications    int `yaml:"max_applications,omitempty" json:"max_applications" ui:"group:Applications"`
	MaxServlets        int `yaml:"max_servlets,omitempty" json:"max_servlets" ui:"group:Servlets"`
	MaxEJBs            int `yaml:"max_ejbs,omitempty" json:"max_ejbs" ui:"group:EJBs"`

	// Filters
	CollectPoolsMatching    string `yaml:"collect_pools_matching,omitempty" json:"collect_pools_matching" ui:"group:Thread Pools"`
	CollectJMSMatching      string `yaml:"collect_jms_matching,omitempty" json:"collect_jms_matching" ui:"group:JMS"`
	CollectAppsMatching     string `yaml:"collect_apps_matching,omitempty" json:"collect_apps_matching" ui:"group:Applications"`
	CollectServletsMatching string `yaml:"collect_servlets_matching,omitempty" json:"collect_servlets_matching" ui:"group:Servlets"`
	CollectEJBsMatching     string `yaml:"collect_ejbs_matching,omitempty" json:"collect_ejbs_matching" ui:"group:EJBs"`

	// Resilience tuning
	MaxRetries              int     `yaml:"max_retries,omitempty" json:"max_retries" ui:"group:Advanced"`
	RetryBackoffMultiplier  float64 `yaml:"retry_backoff_multiplier,omitempty" json:"retry_backoff_multiplier" ui:"group:Advanced"`
	CircuitBreakerThreshold int     `yaml:"circuit_breaker_threshold,omitempty" json:"circuit_breaker_threshold" ui:"group:Advanced"`
	HelperRestartMax        int     `yaml:"helper_restart_max,omitempty" json:"helper_restart_max" ui:"group:Advanced"`
}
