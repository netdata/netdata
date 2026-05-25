// SPDX-License-Identifier: GPL-3.0-or-later

package panos

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed "config_schema.json"
var configSchema string

//go:embed "charts.yaml"
var chartTemplateYAML string

func init() {
	collectorapi.Register("panos", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: 60,
		},
		CreateV2: func() collectorapi.CollectorV2 { return New() },
		Config:   func() any { return &Config{} },
	})
}

func New() *Collector {
	store := metrix.NewCollectorStore()

	return &Collector{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL: "https://127.0.0.1",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(10 * time.Second),
				},
			},
			CollectBGP:                  true,
			CollectSystem:               true,
			CollectHA:                   true,
			CollectEnvironment:          true,
			CollectLicenses:             true,
			CollectIPSec:                true,
			MaxBGPPeers:                 defaultMaxBGPPeers,
			MaxBGPPrefixFamiliesPerPeer: defaultMaxBGPPrefixFamiliesPerPeer,
			MaxBGPVirtualRouters:        defaultMaxBGPVirtualRouters,
			MaxEnvironmentSensors:       defaultMaxEnvironmentSensors,
			MaxLicenses:                 defaultMaxLicenses,
			MaxIPSecTunnels:             defaultMaxIPSecTunnels,
		},
		charts:               &collectorapi.Charts{},
		metrics:              make(map[string]int64, 512),
		store:                store,
		activePeerCharts:     make(map[string]bool),
		activePrefixCharts:   make(map[string]bool),
		activeVRCharts:       make(map[string]bool),
		activeDynamicCharts:  make(map[string]bool),
		missingPeerCharts:    make(map[string]int),
		missingPrefixCharts:  make(map[string]int),
		missingVRCharts:      make(map[string]int),
		missingDynamicCharts: make(map[string]int),
		seenDynamicCharts:    make(map[string]bool),
		cardinalityLogState:  make(map[string]string),
		runtimeLogState:      make(map[string]string),
		routingEngine:        routingEngineUnknown,
		newAPIClient:         newPangoAPIClient,
		advancedBGPCommands:  advancedBGPPeerCommands,
		now:                  time.Now,
	}
}

type Config struct {
	Vnode              string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int    `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int    `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	web.HTTPConfig     `yaml:",inline" json:""`
	APIKey             string `yaml:"api_key,omitempty" json:"api_key"`
	Vsys               string `yaml:"vsys,omitempty" json:"vsys"`
	CollectBGP         bool   `yaml:"collect_bgp,omitempty" json:"collect_bgp"`
	CollectSystem      bool   `yaml:"collect_system,omitempty" json:"collect_system"`
	CollectHA          bool   `yaml:"collect_ha,omitempty" json:"collect_ha"`
	CollectEnvironment bool   `yaml:"collect_environment,omitempty" json:"collect_environment"`
	CollectLicenses    bool   `yaml:"collect_licenses,omitempty" json:"collect_licenses"`
	CollectIPSec       bool   `yaml:"collect_ipsec,omitempty" json:"collect_ipsec"`

	MaxBGPPeers                 int `yaml:"max_bgp_peers,omitempty" json:"max_bgp_peers"`
	MaxBGPPrefixFamiliesPerPeer int `yaml:"max_bgp_prefix_families_per_peer,omitempty" json:"max_bgp_prefix_families_per_peer"`
	MaxBGPVirtualRouters        int `yaml:"max_bgp_virtual_routers,omitempty" json:"max_bgp_virtual_routers"`
	MaxEnvironmentSensors       int `yaml:"max_environment_sensors,omitempty" json:"max_environment_sensors"`
	MaxLicenses                 int `yaml:"max_licenses,omitempty" json:"max_licenses"`
	MaxIPSecTunnels             int `yaml:"max_ipsec_tunnels,omitempty" json:"max_ipsec_tunnels"`

	BGPPeers           matcher.SimpleExpr `yaml:"bgp_peers,omitempty" json:"bgp_peers"`
	BGPPrefixFamilies  matcher.SimpleExpr `yaml:"bgp_prefix_families,omitempty" json:"bgp_prefix_families"`
	BGPVirtualRouters  matcher.SimpleExpr `yaml:"bgp_virtual_routers,omitempty" json:"bgp_virtual_routers"`
	EnvironmentSensors matcher.SimpleExpr `yaml:"environment_sensors,omitempty" json:"environment_sensors"`
	Licenses           matcher.SimpleExpr `yaml:"licenses,omitempty" json:"licenses"`
	IPSecTunnels       matcher.SimpleExpr `yaml:"ipsec_tunnels,omitempty" json:"ipsec_tunnels"`
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	charts  *collectorapi.Charts
	metrics map[string]int64
	store   metrix.CollectorStore

	apiClient panosAPIClient

	activePeerCharts     map[string]bool
	activePrefixCharts   map[string]bool
	activeVRCharts       map[string]bool
	activeDynamicCharts  map[string]bool
	missingPeerCharts    map[string]int
	missingPrefixCharts  map[string]int
	missingVRCharts      map[string]int
	missingDynamicCharts map[string]int
	seenDynamicCharts    map[string]bool
	cardinalityLogState  map[string]string
	runtimeLogState      map[string]string
	lastCollectError     string

	routingEngine    routingEngine
	bgpCommand       string
	noBGPProbedAt    time.Time
	systemInfoLogged bool

	newAPIClient        func(Config) (panosAPIClient, error)
	advancedBGPCommands []string
	now                 func() time.Time

	bgpPeerMatcher           matcher.Matcher
	bgpPrefixFamilyMatcher   matcher.Matcher
	bgpVirtualRouterMatcher  matcher.Matcher
	environmentSensorMatcher matcher.Matcher
	licenseMatcher           matcher.Matcher
	ipsecTunnelMatcher       matcher.Matcher
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if err := c.validateConfig(); err != nil {
		return err
	}
	if err := c.initEntitySelectors(); err != nil {
		return err
	}

	client, err := c.newAPIClient(c.Config)
	if err != nil {
		return fmt.Errorf("init PAN-OS API client: %w", err)
	}
	c.apiClient = client

	c.Debugf("using URL %s", c.URL)
	c.Debugf("using timeout: %s", c.Timeout)

	return nil
}

func (c *Collector) Check(context.Context) error {
	mx, err := c.collect()
	if len(mx) > 0 {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New("no metrics collected")
}

func (c *Collector) Charts() *collectorapi.Charts {
	return c.charts
}

func (c *Collector) Collect(context.Context) error {
	mx, err := c.collect()
	if err != nil {
		c.logCollectError(err)
	} else {
		c.clearCollectError()
	}
	if len(mx) == 0 {
		return err
	}
	c.writeMetricStore(mx)
	return nil
}

func (c *Collector) Cleanup(context.Context) {
	if c.apiClient != nil {
		c.apiClient.closeIdleConnections()
	}
}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string { return chartTemplateYAML }

func (c *Collector) validateConfig() error {
	if c.URL == "" {
		return errors.New("config: url not configured")
	}
	if c.APIKey == "" && (c.Username == "" || c.Password == "") {
		return errors.New("config: api_key or username/password must be set")
	}
	if c.ForceHTTP2 {
		return errors.New("config: force_http2 is not supported by the panos collector")
	}
	if c.BearerTokenFile != "" {
		return errors.New("config: bearer_token_file is not supported by the panos collector")
	}
	if c.Method != "" {
		return errors.New("config: method is not supported by the panos collector")
	}
	if c.Body != "" {
		return errors.New("config: body is not supported by the panos collector")
	}
	if c.NotFollowRedirect {
		return errors.New("config: not_follow_redirects is not supported by the panos collector")
	}
	if c.ProxyUsername != "" || c.ProxyPassword != "" {
		return errors.New("config: proxy_username/proxy_password are not supported; include proxy credentials in proxy_url")
	}
	if (c.TLSCert != "" && c.TLSKey == "") || (c.TLSKey != "" && c.TLSCert == "") {
		return errors.New("config: tls_cert and tls_key must both be set")
	}
	if !c.CollectBGP && !c.CollectSystem && !c.CollectHA && !c.CollectEnvironment && !c.CollectLicenses && !c.CollectIPSec {
		return errors.New("config: at least one PAN-OS metricset must be enabled")
	}
	for _, item := range []struct {
		name  string
		value int
	}{
		{name: "max_bgp_peers", value: c.MaxBGPPeers},
		{name: "max_bgp_prefix_families_per_peer", value: c.MaxBGPPrefixFamiliesPerPeer},
		{name: "max_bgp_virtual_routers", value: c.MaxBGPVirtualRouters},
		{name: "max_environment_sensors", value: c.MaxEnvironmentSensors},
		{name: "max_licenses", value: c.MaxLicenses},
		{name: "max_ipsec_tunnels", value: c.MaxIPSecTunnels},
	} {
		if item.value <= 0 {
			return fmt.Errorf("config: %s must be greater than zero", item.name)
		}
	}
	return nil
}

func (c *Collector) initEntitySelectors() error {
	var err error

	if c.bgpPeerMatcher, err = initEntitySelector("bgp_peers", c.BGPPeers); err != nil {
		return err
	}
	if c.bgpPrefixFamilyMatcher, err = initEntitySelector("bgp_prefix_families", c.BGPPrefixFamilies); err != nil {
		return err
	}
	if c.bgpVirtualRouterMatcher, err = initEntitySelector("bgp_virtual_routers", c.BGPVirtualRouters); err != nil {
		return err
	}
	if c.environmentSensorMatcher, err = initEntitySelector("environment_sensors", c.EnvironmentSensors); err != nil {
		return err
	}
	if c.licenseMatcher, err = initEntitySelector("licenses", c.Licenses); err != nil {
		return err
	}
	if c.ipsecTunnelMatcher, err = initEntitySelector("ipsec_tunnels", c.IPSecTunnels); err != nil {
		return err
	}

	return nil
}

func initEntitySelector(name string, expr matcher.SimpleExpr) (matcher.Matcher, error) {
	if expr.Empty() {
		return nil, nil
	}
	m, err := expr.Parse()
	if err != nil {
		return nil, fmt.Errorf("config: %s selector: %w", name, err)
	}
	return m, nil
}

func (c *Collector) resetMetrics() map[string]int64 {
	if c.metrics == nil {
		c.metrics = make(map[string]int64, 512)
	}
	clear(c.metrics)
	return c.metrics
}

func (c *Collector) logCollectError(err error) {
	msg := err.Error()
	if msg == c.lastCollectError {
		c.Debugf("PAN-OS collection error still active: %v", err)
		return
	}
	c.lastCollectError = msg
	c.Error(err)
}

func (c *Collector) clearCollectError() {
	if c.lastCollectError == "" {
		return
	}
	c.Infof("PAN-OS collection recovered after previous error: %s", c.lastCollectError)
	c.lastCollectError = ""
}

func (c *Collector) warningOnce(key, msg string) {
	if c.runtimeLogState == nil {
		c.runtimeLogState = make(map[string]string)
	}
	if c.runtimeLogState[key] == msg {
		return
	}
	c.runtimeLogState[key] = msg
	c.Warning(msg)
}

func (c *Collector) infoOnce(key, msg string) {
	if c.runtimeLogState == nil {
		c.runtimeLogState = make(map[string]string)
	}
	if c.runtimeLogState[key] == msg {
		return
	}
	c.runtimeLogState[key] = msg
	c.Info(msg)
}

func (c *Collector) clearRuntimeLogState(keys ...string) {
	for _, key := range keys {
		delete(c.runtimeLogState, key)
	}
}

func (c *Collector) logSystemInfoOnce() {
	if c.systemInfoLogged || c.apiClient == nil {
		return
	}

	info := c.apiClient.systemInfo()
	if len(info) == 0 {
		return
	}
	c.systemInfoLogged = true

	hostname := firstNonEmpty(info["hostname"], info["devicename"])
	model := info["model"]
	swVersion := info["sw-version"]
	serial := info["serial"]
	haState := firstNonEmpty(info["ha-state"], info["state"])

	parts := make([]string, 0, 4)
	if hostname != "" {
		parts = append(parts, "hostname="+hostname)
	}
	if model != "" {
		parts = append(parts, "model="+model)
	}
	if swVersion != "" {
		parts = append(parts, "sw_version="+swVersion)
	}
	if serial != "" {
		parts = append(parts, "serial="+serial)
	}
	if haState != "" {
		parts = append(parts, "ha_state="+haState)
	}

	if len(parts) > 0 {
		c.Infof("connected to PAN-OS device: %s", strings.Join(parts, ", "))
	}
	if strings.Contains(strings.ToLower(model), "panorama") {
		c.Warningf("PAN-OS device appears to be Panorama (model=%s); Panorama target proxy mode is not supported by this collector version", model)
	}
}
