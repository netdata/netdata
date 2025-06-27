// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo
// +build cgo

package websphere_jmx

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("websphere_jmx", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *WebSphereJMX {
	return &WebSphereJMX{
		Config: Config{
			UpdateEvery:   5,
			JMXTimeout:    confopt.Duration(time.Second * 5),
			InitTimeout:   confopt.Duration(time.Second * 30),
			ShutdownDelay: confopt.Duration(time.Millisecond * 100),

			// Default collection flags
			CollectJVMMetrics:         true,
			CollectThreadPoolMetrics:  true,
			CollectJDBCMetrics:        true,
			CollectJMSMetrics:         true,
			CollectWebAppMetrics:      true,
			CollectSessionMetrics:     true,
			CollectTransactionMetrics: true,
			CollectClusterMetrics:     true,

			// Default cardinality limits
			MaxThreadPools:     50,
			MaxJDBCPools:       50,
			MaxJMSDestinations: 50,
			MaxApplications:    100,

			// Default resilience settings
			MaxRetries:              3,
			RetryBackoffMultiplier:  2.0,
			CircuitBreakerThreshold: 5,
			HelperRestartMax:        3,
		},

		charts:             &module.Charts{},
		collectedApps:      make(map[string]bool),
		collectedPools:     make(map[string]bool),
		collectedJDBCPools: make(map[string]bool),
		collectedJMS:       make(map[string]bool),
		seenApps:           make(map[string]bool),
		seenPools:          make(map[string]bool),
		seenJDBCPools:      make(map[string]bool),
		seenJMS:            make(map[string]bool),
		lastGoodMetrics:    make(map[string]int64),
	}
}

type Config struct {
	Vnode       string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery int    `yaml:"update_every,omitempty" json:"update_every"`

	// JMX connection settings
	JMXURL       string `yaml:"jmx_url" json:"jmx_url"`
	JMXUsername  string `yaml:"jmx_username,omitempty" json:"jmx_username"`
	JMXPassword  string `yaml:"jmx_password,omitempty" json:"jmx_password"`
	JMXClasspath string `yaml:"jmx_classpath,omitempty" json:"jmx_classpath"`

	// Cluster identification
	ClusterName  string            `yaml:"cluster_name,omitempty" json:"cluster_name"`
	CellName     string            `yaml:"cell_name,omitempty" json:"cell_name"`
	NodeName     string            `yaml:"node_name,omitempty" json:"node_name"`
	ServerType   string            `yaml:"server_type,omitempty" json:"server_type"` // app_server, dmgr, nodeagent
	CustomLabels map[string]string `yaml:"custom_labels,omitempty" json:"custom_labels"`

	// Java process settings
	JavaExecPath  string           `yaml:"java_exec_path,omitempty" json:"java_exec_path"`
	JMXTimeout    confopt.Duration `yaml:"jmx_timeout,omitempty" json:"jmx_timeout"`
	InitTimeout   confopt.Duration `yaml:"init_timeout,omitempty" json:"init_timeout"`
	ShutdownDelay confopt.Duration `yaml:"shutdown_delay,omitempty" json:"shutdown_delay"`

	// Collection flags
	CollectJVMMetrics         bool `yaml:"collect_jvm_metrics" json:"collect_jvm_metrics"`
	CollectThreadPoolMetrics  bool `yaml:"collect_threadpool_metrics" json:"collect_threadpool_metrics"`
	CollectJDBCMetrics        bool `yaml:"collect_jdbc_metrics" json:"collect_jdbc_metrics"`
	CollectJMSMetrics         bool `yaml:"collect_jms_metrics" json:"collect_jms_metrics"`
	CollectWebAppMetrics      bool `yaml:"collect_webapp_metrics" json:"collect_webapp_metrics"`
	CollectSessionMetrics     bool `yaml:"collect_session_metrics" json:"collect_session_metrics"`
	CollectTransactionMetrics bool `yaml:"collect_transaction_metrics" json:"collect_transaction_metrics"`
	CollectClusterMetrics     bool `yaml:"collect_cluster_metrics" json:"collect_cluster_metrics"`

	// Cardinality control
	MaxThreadPools     int `yaml:"max_threadpools,omitempty" json:"max_threadpools"`
	MaxJDBCPools       int `yaml:"max_jdbc_pools,omitempty" json:"max_jdbc_pools"`
	MaxJMSDestinations int `yaml:"max_jms_destinations,omitempty" json:"max_jms_destinations"`
	MaxApplications    int `yaml:"max_applications,omitempty" json:"max_applications"`

	// Filtering
	CollectAppsMatching  string `yaml:"collect_apps_matching,omitempty" json:"collect_apps_matching"`
	CollectPoolsMatching string `yaml:"collect_pools_matching,omitempty" json:"collect_pools_matching"`
	CollectJMSMatching   string `yaml:"collect_jms_matching,omitempty" json:"collect_jms_matching"`

	// Resilience settings
	MaxRetries              int     `yaml:"max_retries,omitempty" json:"max_retries"`
	RetryBackoffMultiplier  float64 `yaml:"retry_backoff_multiplier,omitempty" json:"retry_backoff_multiplier"`
	CircuitBreakerThreshold int     `yaml:"circuit_breaker_threshold,omitempty" json:"circuit_breaker_threshold"`
	HelperRestartMax        int     `yaml:"helper_restart_max,omitempty" json:"helper_restart_max"`
}

type WebSphereJMX struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	// JMX helper process management
	jmxHelper    *jmxHelper
	shutdownOnce sync.Once

	// For tracking dynamic instances
	collectedApps      map[string]bool
	collectedPools     map[string]bool // Thread pools
	collectedJDBCPools map[string]bool // JDBC pools (separate tracking)
	collectedJMS       map[string]bool
	seenApps           map[string]bool
	seenPools          map[string]bool
	seenJDBCPools      map[string]bool
	seenJMS            map[string]bool

	// Selectors
	appSelector  matcher.Matcher
	poolSelector matcher.Matcher
	jmsSelector  matcher.Matcher

	// Resilience
	circuitBreaker  *circuitBreaker
	lastGoodMetrics map[string]int64
	lastSuccessTime time.Time
	helperMonitor   *helperMonitor
}

func (w *WebSphereJMX) Configuration() any {
	return w.Config
}

func (w *WebSphereJMX) Init(ctx context.Context) error {
	// 1. Validate all configuration first
	if err := w.validateConfig(); err != nil {
		return err
	}

	// 2. Initialize selectors
	if err := w.initSelectors(); err != nil {
		return err
	}

	// 3. Initialize resilience features
	w.circuitBreaker = newCircuitBreaker(w.CircuitBreakerThreshold, 30*time.Second)
	w.helperMonitor = newHelperMonitor(w)

	// 4. Only then start the helper process
	if err := w.startJMXHelper(ctx); err != nil {
		return err
	}

	// 5. Start helper monitoring
	w.helperMonitor.start(ctx)

	w.Debugf("initialized websphere_jmx collector: url=%s", w.JMXURL)

	return nil
}

func (w *WebSphereJMX) validateConfig() error {
	if w.JMXURL == "" {
		return errors.New("jmx_url is required")
	}

	// Validate and set default timeouts
	if time.Duration(w.JMXTimeout) <= 0 {
		w.JMXTimeout = confopt.Duration(time.Second * 5)
	}
	if time.Duration(w.InitTimeout) <= 0 {
		w.InitTimeout = confopt.Duration(time.Second * 30)
	}
	if time.Duration(w.ShutdownDelay) <= 0 {
		w.ShutdownDelay = confopt.Duration(time.Millisecond * 100)
	}

	// Validate and normalize cardinality limits
	if w.MaxThreadPools < 0 {
		w.MaxThreadPools = 0
	}
	if w.MaxJDBCPools < 0 {
		w.MaxJDBCPools = 0
	}
	if w.MaxJMSDestinations < 0 {
		w.MaxJMSDestinations = 0
	}
	if w.MaxApplications < 0 {
		w.MaxApplications = 0
	}

	return nil
}

func (w *WebSphereJMX) initSelectors() error {
	if w.CollectAppsMatching != "" {
		m, err := matcher.NewSimplePatternsMatcher(w.CollectAppsMatching)
		if err != nil {
			return fmt.Errorf("invalid app selector pattern '%s': %v", w.CollectAppsMatching, err)
		}
		w.appSelector = m
	}

	if w.CollectPoolsMatching != "" {
		m, err := matcher.NewSimplePatternsMatcher(w.CollectPoolsMatching)
		if err != nil {
			return fmt.Errorf("invalid pool selector pattern '%s': %v", w.CollectPoolsMatching, err)
		}
		w.poolSelector = m
	}

	if w.CollectJMSMatching != "" {
		m, err := matcher.NewSimplePatternsMatcher(w.CollectJMSMatching)
		if err != nil {
			return fmt.Errorf("invalid jms selector pattern '%s': %v", w.CollectJMSMatching, err)
		}
		w.jmsSelector = m
	}

	return nil
}

func (w *WebSphereJMX) startJMXHelper(ctx context.Context) error {
	// Initialize JMX helper
	helper, err := newJMXHelper(w.Config, *w.Logger)
	if err != nil {
		return fmt.Errorf("failed to initialize JMX helper: %w", err)
	}
	w.jmxHelper = helper

	// Start the helper process
	initCtx, cancel := context.WithTimeout(ctx, time.Duration(w.InitTimeout))
	defer cancel()

	if err := w.jmxHelper.start(initCtx); err != nil {
		return fmt.Errorf("failed to start JMX helper: %w", err)
	}

	return nil
}

func (w *WebSphereJMX) Check(ctx context.Context) error {
	// Try a simple health check
	if err := w.checkJMXHealth(ctx); err != nil {
		return fmt.Errorf("JMX health check failed: %w", err)
	}

	// Perform initial collection
	mx, err := w.collect(ctx)
	if err != nil {
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (w *WebSphereJMX) Charts() *module.Charts {
	return w.charts
}

func (w *WebSphereJMX) Collect(ctx context.Context) map[string]int64 {
	mx, err := w.collectWithResilience(ctx)
	if err != nil {
		w.Error(err)
		return nil
	}
	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (w *WebSphereJMX) Cleanup(context.Context) {
	w.shutdownOnce.Do(func() {
		// Stop helper monitor first
		if w.helperMonitor != nil {
			w.helperMonitor.stop()
		}

		if w.jmxHelper != nil {
			w.jmxHelper.shutdown()
		}
	})
}

func (w *WebSphereJMX) checkJMXHealth(ctx context.Context) error {
	healthCtx, cancel := context.WithTimeout(ctx, time.Duration(w.JMXTimeout))
	defer cancel()

	resp, err := w.jmxHelper.sendCommand(healthCtx, jmxCommand{
		Command: "PING",
	})
	if err != nil {
		return err
	}

	if resp.Status != "OK" {
		return fmt.Errorf("health check failed: %s", resp.Message)
	}

	return nil
}
