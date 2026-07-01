// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	_ "embed"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/cwprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/cloudauth"
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
			Namespaces:         NamespacesConfig{Mode: namespacesModeAuto},
			Discovery:          DiscoveryConfig{RefreshEvery: defaultDiscoveryRefresh},
			QueryOffset:        defaultQueryOffset,
			Timeout:            defaultTimeout,
		},
		store:               metrix.NewCollectorStore(),
		now:                 time.Now,
		newAWSConfig:        defaultNewAWSConfig,
		newCloudWatchClient: defaultNewCloudWatchClient,
		newSTSClient:        defaultNewSTSClient,
	}
	c.observations = newObservationStore(c.store)
	c.clients = newClientCache(c.buildRegionClient)
	return c
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	store metrix.CollectorStore
	now   func() time.Time

	// client seams (overridden in tests)
	newAWSConfig        func(ctx context.Context, auth cloudauth.AWSAuthConfig, region string) (aws.Config, error)
	newCloudWatchClient func(cfg aws.Config) cloudwatchClient
	newSTSClient        func(cfg aws.Config) stsClient
	newCatalog          func() (cwprofiles.Catalog, error) // nil => cwprofiles.DefaultCatalog

	accountID         string
	chartTemplateYAML string

	profiles  []cwprofiles.ResolvedProfile // candidate profiles selected per namespaces.mode
	clients   *clientCache                 // CloudWatch client cache, one per region
	discovery discoverySnapshot

	discoverySig string // last-logged discovered-resources summary; Info re-logs only when it changes

	observations *observationStore // retention cache + per-(region, period) query schedule
}

func (c *Collector) Init(context.Context) error {
	c.applyDefaults()
	return c.validate()
}

func (c *Collector) Check(ctx context.Context) error {
	if err := c.ensureAccountIdentity(ctx); err != nil {
		return err
	}
	// Resolve profiles and build the chart template here too: the framework
	// validates ChartTemplateYAML() in postCheck, before the first Collect, so an
	// empty template would fail the job before collection ever starts.
	return c.ensureProfiles()
}

func (c *Collector) Collect(ctx context.Context) error {
	return c.collect(ctx)
}

func (c *Collector) Cleanup(context.Context) {
	// Reset runtime state so a framework re-Init (e.g. after a failed
	// autodetection retry on the same instance) starts clean, mirroring
	// azure_monitor. All ensure*/refresh paths rebuild lazily.
	c.accountID = ""
	c.profiles = nil
	c.chartTemplateYAML = ""
	c.discovery = discoverySnapshot{}
	c.discoverySig = ""
	c.clients.reset()
	c.observations.reset()
}

func (c *Collector) Configuration() any { return c.Config }

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string { return c.chartTemplateYAML }
