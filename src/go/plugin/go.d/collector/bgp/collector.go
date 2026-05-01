// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	collectorapi.Register("bgp", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Create:          func() collectorapi.CollectorV1 { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			Backend:                 backendFRR,
			SocketPath:              defaultSocketPathFRR,
			Address:                 defaultAddressGoBGP,
			RIBSummaryEvery:         confopt.Duration(time.Minute),
			Timeout:                 confopt.Duration(time.Second),
			MaxFamilies:             50,
			MaxPeers:                100,
			MaxVNIs:                 100,
			DeepPeerPrefixMetrics:   false,
			MaxDeepQueriesPerScrape: 40,
		},
		newClient:         newBackendClient,
		charts:            &Charts{},
		familySeen:        make(map[string]time.Time),
		peerSeen:          make(map[string]time.Time),
		neighborSeen:      make(map[string]time.Time),
		vniSeen:           make(map[string]time.Time),
		rpkiSeen:          make(map[string]time.Time),
		rpkiInventorySeen: make(map[string]time.Time),
		cleanupEvery:      30 * time.Second,
		obsoleteAfter:     time.Minute,
	}
}

type Config struct {
	Vnode                   string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery             int    `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry      int    `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	Backend                 string `yaml:"backend,omitempty" json:"backend"`
	SocketPath              string `yaml:"socket_path,omitempty" json:"socket_path"`
	ZebraSocketPath         string `yaml:"zebra_socket_path,omitempty" json:"zebra_socket_path"`
	Address                 string `yaml:"address,omitempty" json:"address"`
	APIURL                  string `yaml:"api_url,omitempty" json:"api_url"`
	ServerName              string `yaml:"server_name,omitempty" json:"server_name"`
	tlscfg.TLSConfig        `yaml:",inline" json:",inline"`
	Timeout                 confopt.Duration   `yaml:"timeout,omitempty" json:"timeout"`
	CollectRIBSummaries     bool               `yaml:"collect_rib_summaries,omitempty" json:"collect_rib_summaries"`
	RIBSummaryEvery         confopt.Duration   `yaml:"rib_summary_every,omitempty" json:"rib_summary_every"`
	MaxFamilies             int                `yaml:"max_families,omitempty" json:"max_families"`
	SelectFamilies          matcher.SimpleExpr `yaml:"select_families,omitempty" json:"select_families"`
	MaxPeers                int                `yaml:"max_peers,omitempty" json:"max_peers"`
	MaxVNIs                 int                `yaml:"max_vnis,omitempty" json:"max_vnis"`
	SelectVNIs              matcher.SimpleExpr `yaml:"select_vnis,omitempty" json:"select_vnis"`
	DeepPeerPrefixMetrics   bool               `yaml:"deep_peer_prefix_metrics,omitempty" json:"deep_peer_prefix_metrics"`
	MaxDeepQueriesPerScrape int                `yaml:"max_deep_queries_per_scrape,omitempty" json:"max_deep_queries_per_scrape"`
	SelectPeers             matcher.SimpleExpr `yaml:"select_peers,omitempty" json:"select_peers"`
}

type (
	Collector struct {
		collectorapi.Base
		Config `yaml:",inline" json:""`

		charts *Charts

		client    bgpClient
		newClient func(Config) (bgpClient, error)

		selectFamilyMatcher matcher.Matcher
		selectPeerMatcher   matcher.Matcher
		selectVNIMatcher    matcher.Matcher

		cleanupLastTime      time.Time
		cleanupEvery         time.Duration
		obsoleteAfter        time.Duration
		familySeen           map[string]time.Time
		peerSeen             map[string]time.Time
		neighborSeen         map[string]time.Time
		vniSeen              map[string]time.Time
		rpkiSeen             map[string]time.Time
		rpkiInventorySeen    map[string]time.Time
		neighborDetailsCache map[string]map[string]neighborDetails
		openbgpdRIBCache     openbgpdRIBCache
		gobgpValidationCache gobgpValidationCache
	}

	bgpClient interface {
		Close() error
	}

	frrClientAPI interface {
		Summary(afi, safi string) ([]byte, error)
		Neighbors() ([]byte, error)
		RPKICacheServers() ([]byte, error)
		RPKICacheConnections() ([]byte, error)
		RPKIPrefixCount() ([]byte, error)
		EVPNVNI() ([]byte, error)
		PeerRoutes(vrf, afi, safi, neighbor string) ([]byte, error)
		PeerAdvertisedRoutes(vrf, afi, safi, neighbor string) ([]byte, error)
	}

	birdClientAPI interface {
		ProtocolsAll() ([]byte, error)
	}

	openbgpdClientAPI interface {
		Neighbors() ([]byte, error)
		RIB() ([]byte, error)
		RIBFiltered(family string) ([]byte, error)
	}

	gobgpClientAPI interface {
		GetBgp() (*gobgpGlobalInfo, error)
		ListPeers() ([]*gobgpPeerInfo, error)
		ListRpki() ([]*gobgpRpkiInfo, error)
		GetTable(*gobgpFamilyRef) (uint64, error)
		ListPathValidation(*gobgpFamilyRef) (gobgpValidationSummary, error)
	}
)

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if err := c.validateConfig(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	c.charts = c.initCharts()

	m, err := c.initSelectMatcher(c.SelectFamilies)
	if err != nil {
		return fmt.Errorf("init select_families matcher: %w", err)
	}
	c.selectFamilyMatcher = m

	m, err = c.initSelectMatcher(c.SelectPeers)
	if err != nil {
		return fmt.Errorf("init select_peers matcher: %w", err)
	}
	c.selectPeerMatcher = m

	m, err = c.initSelectMatcher(c.SelectVNIs)
	if err != nil {
		return fmt.Errorf("init select_vnis matcher: %w", err)
	}
	c.selectVNIMatcher = m

	client, err := c.newClient(c.Config)
	if err != nil {
		return fmt.Errorf("init client: %w", err)
	}
	c.client = client

	c.Debugf("using backend: %s", c.Backend)
	c.Debugf("using socket_path: %s", c.SocketPath)
	c.Debugf("using zebra_socket_path: %s", c.ZebraSocketPath)
	c.Debugf("using address: %s", c.Address)
	c.Debugf("using api_url: %s", c.APIURL)
	c.Debugf("using server_name: %s", c.ServerName)
	c.Debugf("using timeout: %s", c.Timeout)
	c.Debugf("using collect_rib_summaries: %v", c.CollectRIBSummaries)
	c.Debugf("using rib_summary_every: %s", c.RIBSummaryEvery)
	c.Debugf("using max_families: %d", c.MaxFamilies)
	c.Debugf("using max_peers: %d", c.MaxPeers)
	c.Debugf("using max_vnis: %d", c.MaxVNIs)
	c.Debugf("using deep_peer_prefix_metrics: %v", c.DeepPeerPrefixMetrics)
	c.Debugf("using max_deep_queries_per_scrape: %d", c.MaxDeepQueriesPerScrape)

	return nil
}

func (c *Collector) Check(context.Context) error {
	mx, hasBGPData, err := c.collect()
	if err != nil {
		return err
	}
	if len(mx) == 0 || !hasBGPData {
		return errors.New("no BGP metrics collected")
	}
	return nil
}

func (c *Collector) Charts() *Charts {
	return c.charts
}

func (c *Collector) Collect(context.Context) map[string]int64 {
	mx, _, err := c.collect()
	if err != nil {
		c.Error(err)
	}
	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (c *Collector) Cleanup(context.Context) {
	if c.client != nil {
		_ = c.client.Close()
	}
}
