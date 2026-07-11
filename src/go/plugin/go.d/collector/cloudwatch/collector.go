// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	_ "embed"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/awsauth"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"
)

// recurringLogEvery throttles warnings for conditions that recur every collect
// cycle (e.g. a persistently unreachable region): c.Limit(key, 1, recurringLogEvery).
const recurringLogEvery = time.Hour

const (
	logKeyDiscoveryTargetFailed  = "discovery_target_failed"
	logKeyHighInstanceCount      = "high_instance_count"
	logKeyQueryClientFailed      = "query_client_failed"
	logKeyGetMetricDataFailed    = "getmetricdata_failed"
	logKeyGetMetricDataForbidden = "getmetricdata_forbidden"
	logKeyAccountResolveFailed   = "account_resolve_failed"
	logKeyTagRefreshFailed       = "tag_refresh_failed"
	logKeyRuleShadowed           = "rule_shadowed"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	collectorapi.Register("cloudwatch", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery:        defaultUpdateEvery,
			AutoDetectionRetry: defaultAutoDetectRetry,
		},
		CreateV2: func() collectorapi.CollectorV2 { return New() },
		Config:   func() any { return &Config{} },
	})
}

func New() *Collector {
	c := &Collector{
		Config: Config{
			UpdateEvery:        defaultUpdateEvery,
			AutoDetectionRetry: defaultAutoDetectRetry,
			Discovery:          DiscoveryConfig{RefreshEvery: defaultDiscoveryRefresh},
			QueryOffset:        defaultQueryOffset,
			Limits:             LimitsConfig{MaxInstances: defaultMaxInstances},
			Timeout:            defaultTimeout,
		},
		store:               metrix.NewCollectorStore(),
		now:                 time.Now,
		newAWSConfig:        defaultNewAWSConfig,
		newCloudWatchClient: defaultNewCloudWatchClient,
		newSTSClient:        defaultNewSTSClient,
		newRGTAClient:       defaultNewRGTAClient,
	}
	c.observations = newObservationStore(c.store)
	c.clients = newClientCache(c.buildTargetRegionClient)
	c.rgtaClients = newClientCache(c.buildTargetRegionRGTAClient)
	return c
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	store metrix.CollectorStore
	now   func() time.Time

	// client seams (overridden in tests)
	newAWSConfig        func(ctx context.Context, id awsauth.Identity, region string) (aws.Config, error)
	newCloudWatchClient func(cfg aws.Config) cloudwatchClient
	newSTSClient        func(cfg aws.Config) stsClient
	newRGTAClient       func(cfg aws.Config) rgtaClient
	newCatalog          func() (cwprofiles.Catalog, error) // nil => cwprofiles.DefaultCatalog

	plan              *collectionPlan
	resolvedByRef     map[string]resolvedTarget
	chartTemplateYAML string

	clients        *clientCache[cloudwatchClient] // one per (target, region)
	rgtaClients    *clientCache[rgtaClient]       // one per (target, region)
	discovery      discoverySnapshot
	queryPlan      []plannedQuery
	queryGroups    []queryGroupKey
	queriesByGroup map[queryGroupKey][]plannedQuery
	planDirty      bool

	discoverySig string // last-logged discovered-resources summary; Info re-logs only when it changes

	observations *observationStore // retention cache + per-(target, region, period) query schedule

	tags          tagSnapshot              // resource-tag membership and label cache, refreshed with discovery
	tagLabelPlans map[string][]resolvedTag // per-profile resolved tag->label plans (nil until compiled)
}

func (c *Collector) Init(context.Context) error {
	c.applyDefaults()
	return c.validate()
}

func (c *Collector) Check(ctx context.Context) error {
	if err := c.ensurePlan(); err != nil {
		return err
	}
	return c.ensureTargets(ctx)
}

func (c *Collector) Collect(ctx context.Context) error {
	return c.collect(ctx)
}

func (c *Collector) Cleanup(context.Context) {
	// Reset runtime state so a framework re-Init (e.g. after a failed
	// autodetection retry on the same instance) starts clean, mirroring
	// azure_monitor. All ensure*/refresh paths rebuild lazily.
	c.plan = nil
	c.resolvedByRef = nil
	c.chartTemplateYAML = ""
	c.discovery = discoverySnapshot{}
	c.queryPlan = nil
	c.queryGroups = nil
	c.queriesByGroup = nil
	c.planDirty = true
	c.discoverySig = ""
	c.clients.reset()
	c.rgtaClients.reset()
	c.observations.reset()
	c.tags = tagSnapshot{}
	c.tagLabelPlans = nil
}

func (c *Collector) Configuration() any { return c.Config }

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string { return c.chartTemplateYAML }
