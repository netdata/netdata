// SPDX-License-Identifier: GPL-3.0-or-later

package websphere_pmi

import (
	"context"
	_ "embed"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("websphere_pmi", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *WebSpherePMI {
	return &WebSpherePMI{
		Config: Config{
			UpdateEvery:    5,
			PMIStatsType:   "extended", // basic, extended, all, custom
			PMIRefreshRate: 60,         // seconds
			HTTPConfig: web.HTTPConfig{
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second * 5),
				},
			},

			// Default cardinality limits
			MaxThreadPools:     50,
			MaxJDBCPools:       50,
			MaxJCAPools:        50,
			MaxJMSDestinations: 50,
			MaxApplications:    100,
			MaxServlets:        50,
			MaxEJBs:            50,
		},

		charts:             &module.Charts{},
		pmiCache:           make(map[string]*pmiCacheEntry),
		loggedWarnings:     make(map[string]bool),
		collectedInstances: make(map[string]bool),
		seenInstances:      make(map[string]bool),
		timeStatCache:      make(map[string]*timeStatCacheEntry),
	}
}

type Config struct {
	Vnode       string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery int    `yaml:"update_every,omitempty" json:"update_every"`

	// PMI settings
	PMIStatsType        string   `yaml:"pmi_stats_type,omitempty" json:"pmi_stats_type"`
	PMIRefreshRate      int      `yaml:"pmi_refresh_rate,omitempty" json:"pmi_refresh_rate"`
	PMICustomStatsPaths []string `yaml:"pmi_custom_stats_paths,omitempty" json:"pmi_custom_stats_paths"`

	// Cluster identification
	ClusterName  string            `yaml:"cluster_name,omitempty" json:"cluster_name"`
	CellName     string            `yaml:"cell_name,omitempty" json:"cell_name"`
	NodeName     string            `yaml:"node_name,omitempty" json:"node_name"`
	ServerType   string            `yaml:"server_type,omitempty" json:"server_type"` // app_server, dmgr, nodeagent
	CustomLabels map[string]string `yaml:"custom_labels,omitempty" json:"custom_labels"`

	// Collection flags
	CollectJVMMetrics         *bool `yaml:"collect_jvm_metrics,omitempty" json:"collect_jvm_metrics"`
	CollectThreadPoolMetrics  *bool `yaml:"collect_threadpool_metrics,omitempty" json:"collect_threadpool_metrics"`
	CollectJDBCMetrics        *bool `yaml:"collect_jdbc_metrics,omitempty" json:"collect_jdbc_metrics"`
	CollectJCAMetrics         *bool `yaml:"collect_jca_metrics,omitempty" json:"collect_jca_metrics"`
	CollectJMSMetrics         *bool `yaml:"collect_jms_metrics,omitempty" json:"collect_jms_metrics"`
	CollectWebAppMetrics      *bool `yaml:"collect_webapp_metrics,omitempty" json:"collect_webapp_metrics"`
	CollectSessionMetrics     *bool `yaml:"collect_session_metrics,omitempty" json:"collect_session_metrics"`
	CollectTransactionMetrics *bool `yaml:"collect_transaction_metrics,omitempty" json:"collect_transaction_metrics"`
	CollectClusterMetrics     *bool `yaml:"collect_cluster_metrics,omitempty" json:"collect_cluster_metrics"`

	// APM Collection flags
	CollectServletMetrics *bool `yaml:"collect_servlet_metrics,omitempty" json:"collect_servlet_metrics"`
	CollectEJBMetrics     *bool `yaml:"collect_ejb_metrics,omitempty" json:"collect_ejb_metrics"`
	CollectJDBCAdvanced   *bool `yaml:"collect_jdbc_advanced,omitempty" json:"collect_jdbc_advanced"`

	// Cardinality control
	MaxThreadPools     int `yaml:"max_threadpools,omitempty" json:"max_threadpools"`
	MaxJDBCPools       int `yaml:"max_jdbc_pools,omitempty" json:"max_jdbc_pools"`
	MaxJCAPools        int `yaml:"max_jca_pools,omitempty" json:"max_jca_pools"`
	MaxJMSDestinations int `yaml:"max_jms_destinations,omitempty" json:"max_jms_destinations"`
	MaxApplications    int `yaml:"max_applications,omitempty" json:"max_applications"`
	MaxServlets        int `yaml:"max_servlets,omitempty" json:"max_servlets"`
	MaxEJBs            int `yaml:"max_ejbs,omitempty" json:"max_ejbs"`

	// Filtering
	CollectAppsMatching     string `yaml:"collect_apps_matching,omitempty" json:"collect_apps_matching"`
	CollectPoolsMatching    string `yaml:"collect_pools_matching,omitempty" json:"collect_pools_matching"`
	CollectJMSMatching      string `yaml:"collect_jms_matching,omitempty" json:"collect_jms_matching"`
	CollectServletsMatching string `yaml:"collect_servlets_matching,omitempty" json:"collect_servlets_matching"`
	CollectEJBsMatching     string `yaml:"collect_ejbs_matching,omitempty" json:"collect_ejbs_matching"`

	// HTTP Client settings
	web.HTTPConfig `yaml:",inline" json:""`
}

type WebSpherePMI struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	httpClient *http.Client
	pmiURL     string

	// Selectors
	appSelector     matcher.Matcher
	poolSelector    matcher.Matcher
	jmsSelector     matcher.Matcher
	servletSelector matcher.Matcher
	ejbSelector     matcher.Matcher

	// PMI data cache
	pmiCache      map[string]*pmiCacheEntry
	pmiCacheMutex sync.RWMutex
	lastRefresh   time.Time

	// For logging warnings only once
	loggedWarnings map[string]bool

	// WebSphere version information
	wasVersion string
	wasEdition string // traditional, liberty

	// Instance tracking for proper collection
	collectedInstances map[string]bool
	seenInstances      map[string]bool
	
	// TimeStatistic processing
	timeStatCache map[string]*timeStatCacheEntry
}

// timeStatCacheEntry stores previous TimeStatistic values for delta calculations
type timeStatCacheEntry struct {
	Count int64
	Total int64
}

type pmiCacheEntry struct {
	value     int64
	timestamp time.Time
}

// PMI XML structures
type pmiStatsResponse struct {
	XMLName        xml.Name  `xml:"PerformanceMonitor"`
	ResponseStatus string    `xml:"responseStatus,attr"`
	Version        string    `xml:"version,attr"`
	Nodes          []pmiNode `xml:"Node"`
	Stats          []pmiStat `xml:"Stat"` // For Liberty/other versions
}

type pmiNode struct {
	XMLName xml.Name    `xml:"Node"`
	Name    string      `xml:"name,attr"`
	Servers []pmiServer `xml:"Server"`
}

type pmiServer struct {
	XMLName xml.Name  `xml:"Server"`
	Name    string    `xml:"name,attr"`
	Stats   []pmiStat `xml:"Stat"`
}

type pmiStat struct {
	XMLName                xml.Name           `xml:"Stat"`
	Name                   string             `xml:"name,attr"`
	Type                   string             `xml:"type,attr"`
	ID                     string             `xml:"id,attr"`
	Path                   string             `xml:"path,attr"`
	Value                  *pmiValue          `xml:"Value"`
	CountStatistics        []countStat        `xml:"CountStatistic"`
	TimeStatistics         []timeStat         `xml:"TimeStatistic"`
	RangeStatistics        []rangeStat        `xml:"RangeStatistic"`
	BoundedRangeStatistics []boundedRangeStat `xml:"BoundedRangeStatistic"`
	DoubleStatistics       []doubleStat       `xml:"DoubleStatistic"`
	AverageStatistics      []averageStat      `xml:"AverageStatistic"`
	SubStats               []pmiStat          `xml:"Stat"`

	// Keep single references for backward compatibility (no XML tags!)
	CountStatistic        *countStat        `xml:"-"`
	TimeStatistic         *timeStat         `xml:"-"`
	RangeStatistic        *rangeStat        `xml:"-"`
	BoundedRangeStatistic *boundedRangeStat `xml:"-"`
	DoubleStatistic       *doubleStat       `xml:"-"`
	AverageStatistic      *averageStat      `xml:"-"`
}

type pmiValue struct {
	Value string `xml:",chardata"`
}

type countStat struct {
	Name  string `xml:"name,attr"`
	Count string `xml:"count,attr"`
}

type timeStat struct {
	Name      string `xml:"name,attr"`
	Count     string `xml:"count,attr"`
	Total     string `xml:"total,attr"`     // Some versions use "total"
	TotalTime string `xml:"totalTime,attr"` // WebSphere uses "totalTime"
	Mean      string `xml:"mean,attr"`
	Min       string `xml:"min,attr"`
	Max       string `xml:"max,attr"`
}

type rangeStat struct {
	Name          string `xml:"name,attr"`
	Current       string `xml:"value,attr"` // WebSphere uses "value" attribute for current value
	Integral      string `xml:"integral,attr"`
	Mean          string `xml:"mean,attr"`
	HighWaterMark string `xml:"highWaterMark,attr"`
	LowWaterMark  string `xml:"lowWaterMark,attr"`
}

type boundedRangeStat struct {
	Name          string `xml:"name,attr"`
	Current       string `xml:"value,attr"`
	Integral      string `xml:"integral,attr"`
	Mean          string `xml:"mean,attr"`
	LowerBound    string `xml:"lowerBound,attr"`
	UpperBound    string `xml:"upperBound,attr"`
	HighWaterMark string `xml:"highWaterMark,attr"`
	LowWaterMark  string `xml:"lowWaterMark,attr"`
}

type doubleStat struct {
	Name   string `xml:"name,attr"`
	Double string `xml:"double,attr"`
}

type averageStat struct {
	Name         string `xml:"name,attr"`
	Count        string `xml:"count,attr"`
	Total        string `xml:"total,attr"`
	Mean         string `xml:"mean,attr"`
	Min          string `xml:"min,attr"`
	Max          string `xml:"max,attr"`
	SumOfSquares string `xml:"sumOfSquares,attr"`
}

// populateBackwardCompatibility populates single references from arrays for backward compatibility
func (s *pmiStat) populateBackwardCompatibility() {
	if len(s.CountStatistics) > 0 {
		s.CountStatistic = &s.CountStatistics[0]
	}
	if len(s.TimeStatistics) > 0 {
		s.TimeStatistic = &s.TimeStatistics[0]
	}
	if len(s.RangeStatistics) > 0 {
		s.RangeStatistic = &s.RangeStatistics[0]
	}
	if len(s.BoundedRangeStatistics) > 0 {
		s.BoundedRangeStatistic = &s.BoundedRangeStatistics[0]
	}
	if len(s.DoubleStatistics) > 0 {
		s.DoubleStatistic = &s.DoubleStatistics[0]
	}
	if len(s.AverageStatistics) > 0 {
		s.AverageStatistic = &s.AverageStatistics[0]
	}

	// Recursively populate sub-stats
	for i := range s.SubStats {
		s.SubStats[i].populateBackwardCompatibility()
	}
}

func (w *WebSpherePMI) Configuration() any {
	return w.Config
}

func (w *WebSpherePMI) Init(ctx context.Context) error {
	// Set configuration defaults
	w.setConfigurationDefaults()

	// Validate configuration
	if err := w.validateConfig(); err != nil {
		return err
	}

	// Initialize selectors
	if err := w.initSelectors(); err != nil {
		return err
	}

	// Initialize HTTP client
	httpClient, err := web.NewHTTPClient(w.Config.HTTPConfig.ClientConfig)
	if err != nil {
		return fmt.Errorf("failed to create HTTP client: %w", err)
	}
	w.httpClient = httpClient

	// Build PMI URL
	if err := w.buildPMIURL(); err != nil {
		return fmt.Errorf("failed to build PMI URL: %w", err)
	}

	// Initialize base charts
	for _, chart := range *baseCharts.Copy() {
		// Add identity labels
		if w.ClusterName != "" {
			chart.Labels = append(chart.Labels, module.Label{Key: "cluster", Value: w.ClusterName})
		}
		if w.CellName != "" {
			chart.Labels = append(chart.Labels, module.Label{Key: "cell", Value: w.CellName})
		}
		if w.NodeName != "" {
			chart.Labels = append(chart.Labels, module.Label{Key: "node", Value: w.NodeName})
		}
		if w.ServerType != "" {
			chart.Labels = append(chart.Labels, module.Label{Key: "server_type", Value: w.ServerType})
		}
		// Add custom labels
		for k, v := range w.CustomLabels {
			chart.Labels = append(chart.Labels, module.Label{Key: k, Value: v})
		}
		w.charts.Add(chart)
	}

	w.Debugf("initialized websphere_pmi collector: url=%s, stats_type=%s, refresh_rate=%d", w.pmiURL, w.PMIStatsType, w.PMIRefreshRate)
	w.Debugf("collection flags: JVM=%v, ThreadPool=%v, JDBC=%v, JCA=%v, JMS=%v, WebApp=%v",
		*w.CollectJVMMetrics, *w.CollectThreadPoolMetrics, *w.CollectJDBCMetrics,
		*w.CollectJCAMetrics, *w.CollectJMSMetrics, *w.CollectWebAppMetrics)
	w.Debugf("APM flags: Servlet=%v, EJB=%v, JDBCAdvanced=%v",
		*w.CollectServletMetrics, *w.CollectEJBMetrics, *w.CollectJDBCAdvanced)
	if w.ClusterName != "" || w.CellName != "" || w.NodeName != "" {
		w.Debugf("identity labels: cluster=%s, cell=%s, node=%s, server_type=%s",
			w.ClusterName, w.CellName, w.NodeName, w.ServerType)
	}

	return nil
}

func (w *WebSpherePMI) setConfigurationDefaults() {
	// Set defaults for boolean configuration fields only if not explicitly set
	if w.Config.CollectJVMMetrics == nil {
		defaultValue := true
		w.Config.CollectJVMMetrics = &defaultValue
	}
	if w.Config.CollectThreadPoolMetrics == nil {
		defaultValue := true
		w.Config.CollectThreadPoolMetrics = &defaultValue
	}
	if w.Config.CollectJDBCMetrics == nil {
		defaultValue := true
		w.Config.CollectJDBCMetrics = &defaultValue
	}
	if w.Config.CollectJCAMetrics == nil {
		defaultValue := true
		w.Config.CollectJCAMetrics = &defaultValue
	}
	if w.Config.CollectJMSMetrics == nil {
		defaultValue := true
		w.Config.CollectJMSMetrics = &defaultValue
	}
	if w.Config.CollectWebAppMetrics == nil {
		defaultValue := true
		w.Config.CollectWebAppMetrics = &defaultValue
	}
	if w.Config.CollectSessionMetrics == nil {
		defaultValue := true
		w.Config.CollectSessionMetrics = &defaultValue
	}
	if w.Config.CollectTransactionMetrics == nil {
		defaultValue := true
		w.Config.CollectTransactionMetrics = &defaultValue
	}
	if w.Config.CollectClusterMetrics == nil {
		defaultValue := true
		w.Config.CollectClusterMetrics = &defaultValue
	}

	// APM features default to false due to higher overhead
	if w.Config.CollectServletMetrics == nil {
		defaultValue := false
		w.Config.CollectServletMetrics = &defaultValue
	}
	if w.Config.CollectEJBMetrics == nil {
		defaultValue := false
		w.Config.CollectEJBMetrics = &defaultValue
	}
	if w.Config.CollectJDBCAdvanced == nil {
		defaultValue := false
		w.Config.CollectJDBCAdvanced = &defaultValue
	}

	// Set default server type if not specified
	if w.ServerType == "" {
		w.ServerType = "app_server"
	}
}

func (w *WebSpherePMI) validateConfig() error {
	if w.URL == "" {
		return errors.New("url is required")
	}

	// Validate and set default timeout
	if time.Duration(w.HTTPConfig.ClientConfig.Timeout) <= 0 {
		w.HTTPConfig.ClientConfig.Timeout = confopt.Duration(time.Second * 5)
	}

	// Validate PMI stats type
	validTypes := map[string]bool{
		"basic":    true,
		"extended": true,
		"all":      true,
		"custom":   true,
	}
	if !validTypes[w.PMIStatsType] {
		w.PMIStatsType = "extended"
	}

	// Validate cardinality limits
	if w.MaxThreadPools < 0 {
		w.MaxThreadPools = 0
	}
	if w.MaxJDBCPools < 0 {
		w.MaxJDBCPools = 0
	}
	if w.MaxJCAPools < 0 {
		w.MaxJCAPools = 0
	}
	if w.MaxJMSDestinations < 0 {
		w.MaxJMSDestinations = 0
	}
	if w.MaxApplications < 0 {
		w.MaxApplications = 0
	}
	if w.MaxServlets < 0 {
		w.MaxServlets = 0
	}
	if w.MaxEJBs < 0 {
		w.MaxEJBs = 0
	}

	return nil
}

func (w *WebSpherePMI) initSelectors() error {
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

	if w.CollectServletsMatching != "" {
		m, err := matcher.NewSimplePatternsMatcher(w.CollectServletsMatching)
		if err != nil {
			return fmt.Errorf("invalid servlet selector pattern '%s': %v", w.CollectServletsMatching, err)
		}
		w.servletSelector = m
	}

	if w.CollectEJBsMatching != "" {
		m, err := matcher.NewSimplePatternsMatcher(w.CollectEJBsMatching)
		if err != nil {
			return fmt.Errorf("invalid ejb selector pattern '%s': %v", w.CollectEJBsMatching, err)
		}
		w.ejbSelector = m
	}

	return nil
}

func (w *WebSpherePMI) buildPMIURL() error {
	// Require complete URL - no path inference
	w.pmiURL = strings.TrimSpace(w.URL)

	if w.pmiURL == "" {
		return errors.New("url is required")
	}

	// Parse and validate
	parsedURL, err := url.Parse(w.pmiURL)
	if err != nil {
		return fmt.Errorf("invalid PMI URL: %w", err)
	}

	// Ensure URL has a path
	if parsedURL.Path == "" || parsedURL.Path == "/" {
		return errors.New("url must include the complete path to the PMI servlet (e.g., http://localhost:9080/wasPerfTool/servlet/perfservlet)")
	}

	return nil
}

func (w *WebSpherePMI) Check(ctx context.Context) error {
	// Try to fetch PMI stats
	mx, err := w.collect(ctx)
	if err != nil {
		return fmt.Errorf("PMI check failed: %w", err)
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (w *WebSpherePMI) Charts() *module.Charts {
	return w.charts
}

func (w *WebSpherePMI) Collect(ctx context.Context) map[string]int64 {
	mx, err := w.collect(ctx)
	if err != nil {
		// Use more specific error messages based on error type
		switch {
		case errors.Is(err, context.Canceled):
			w.logOnce("context_cancelled", "collection cancelled: %v", err)
		case errors.Is(err, context.DeadlineExceeded):
			w.logOnce("context_timeout", "collection timed out: %v", err)
		case strings.Contains(err.Error(), "401"):
			w.logOnce("auth_failed", "authentication failed: %v", err)
		case strings.Contains(err.Error(), "connection refused"):
			w.logOnce("conn_refused", "connection refused to PMI servlet: %v", err)
		case strings.Contains(err.Error(), "timeout"):
			w.logOnce("timeout", "timeout connecting to PMI servlet: %v", err)
		case strings.Contains(err.Error(), "404"):
			w.logOnce("not_found", "PMI servlet not found - check URL configuration: %v", err)
		default:
			w.Error(err)
		}
		return nil
	}
	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (w *WebSpherePMI) Cleanup(context.Context) {
	if w.httpClient != nil {
		w.httpClient.CloseIdleConnections()
	}
}

func (w *WebSpherePMI) collect(ctx context.Context) (map[string]int64, error) {
	// Fetch PMI stats
	stats, err := w.fetchPMIStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PMI stats: %w", err)
	}

	// Detect WebSphere version on first successful collection
	w.detectWebSphereVersion(stats)

	// Use proper collection approach based on actual XML structure
	return w.collectProper(stats), nil
}

func (w *WebSpherePMI) fetchPMIStats(ctx context.Context) (*pmiStatsResponse, error) {
	// Build request URL with parameters
	reqURL := w.pmiURL + "?stats=" + w.PMIStatsType

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	// Add authentication if configured
	if w.HTTPConfig.Username != "" && w.HTTPConfig.Password != "" {
		req.SetBasicAuth(w.HTTPConfig.Username, w.HTTPConfig.Password)
	}

	// Execute request
	resp, err := w.httpClient.Do(req)
	if err != nil {
		// Check if context was cancelled
		if ctx.Err() != nil {
			return nil, fmt.Errorf("request cancelled: %w", ctx.Err())
		}
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("PMI request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse XML response with context awareness
	decoder := xml.NewDecoder(resp.Body)

	// Create a channel for decoding result
	type decodeResult struct {
		stats *pmiStatsResponse
		err   error
	}
	resultChan := make(chan decodeResult, 1)

	// Decode in goroutine to respect context cancellation
	go func() {
		var s pmiStatsResponse
		err := decoder.Decode(&s)
		resultChan <- decodeResult{stats: &s, err: err}
	}()

	// Wait for either decode completion or context cancellation
	select {
	case result := <-resultChan:
		if result.err != nil {
			w.Errorf("ISSUE: Failed to parse PMI XML response: %v", result.err)
			return nil, fmt.Errorf("failed to parse PMI XML response: %w", result.err)
		}
		return result.stats, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("parsing cancelled: %w", ctx.Err())
	}
}

func (w *WebSpherePMI) logOnce(key, format string, args ...any) {
	if w.loggedWarnings[key] {
		return
	}
	w.loggedWarnings[key] = true
	w.Warningf(format, args...)
}

func (w *WebSpherePMI) detectWebSphereVersion(stats *pmiStatsResponse) {
	// Try to detect version from PMI stats
	if w.wasVersion != "" {
		return // Already detected
	}

	// Get version from response attributes
	if stats.Version != "" {
		w.wasVersion = stats.Version
		w.Debugf("detected WebSphere version from response: %s", w.wasVersion)
	}

	// Check structure to determine edition
	if len(stats.Nodes) > 0 {
		w.wasEdition = "traditional"
		w.Debugf("detected traditional WebSphere Application Server from Node/Server structure")
	}

	// Look for version information in stats
	for _, stat := range stats.Stats {
		if stat.Path == "websphereVersion" || stat.Name == "version" {
			if stat.Value != nil {
				w.wasVersion = stat.Value.Value
			}
		}
		// Check for Liberty-specific stats
		if strings.Contains(stat.Path, "liberty") {
			w.wasEdition = "liberty"
		}
	}

	// Default to traditional if not Liberty
	if w.wasEdition == "" {
		w.wasEdition = "traditional"
	}

	// Log detected version
	if w.wasVersion != "" {
		w.Infof("detected WebSphere version: %s (%s)", w.wasVersion, w.wasEdition)
	} else {
		w.Debugf("WebSphere version not detected, assuming traditional WAS")
		w.wasVersion = "unknown"
	}
	
	// Always add version labels (even if unknown)
	w.addVersionLabelsToCharts()
	w.adjustCollectionBasedOnVersion()
}

func (w *WebSpherePMI) adjustCollectionBasedOnVersion() {
	// Version-specific adjustments for known differences
	// Note: Admin configuration always takes precedence (see setConfigurationDefaults)

	if w.wasEdition == "liberty" {
		// Liberty has different PMI paths and metrics
		w.Debugf("adjusting collection for WebSphere Liberty")

		// Liberty doesn't support some traditional metrics by default
		// But we still attempt collection if admin explicitly enabled them
		if w.CollectClusterMetrics != nil && *w.CollectClusterMetrics {
			w.logOnce("liberty_cluster", "cluster metrics collection enabled for Liberty - some metrics may not be available")
		}
	}

	// Version-specific feature detection
	if w.wasVersion != "" {
		// Extract major version for feature detection
		parts := strings.Split(w.wasVersion, ".")
		if len(parts) > 0 {
			majorVersion := parts[0]

			switch majorVersion {
			case "7":
				w.Debugf("WebSphere 7.x detected - some modern metrics may not be available")
			case "8":
				w.Debugf("WebSphere 8.x detected - full metric support expected")
			case "9":
				w.Debugf("WebSphere 9.x detected - enhanced metrics available")
			default:
				w.Debugf("WebSphere version %s detected", majorVersion)
			}
		}
	}
}

func (w *WebSpherePMI) getVersionLabels() []module.Label {
	version := w.wasVersion
	edition := w.wasEdition
	
	// Provide defaults if not set
	if version == "" {
		version = "unknown"
	}
	if edition == "" {
		edition = "traditional"
	}
	
	return []module.Label{
		{Key: "was_version", Value: version},
		{Key: "was_edition", Value: edition},
	}
}

func (w *WebSpherePMI) addVersionLabelsToCharts() {
	versionLabels := w.getVersionLabels()

	// Add to all existing charts
	for _, chart := range *w.charts {
		chart.Labels = append(chart.Labels, versionLabels...)
	}
}
