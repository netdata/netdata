// SPDX-License-Identifier: GPL-3.0-or-later

package rum

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/dem/config"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/agg"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/ingest"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/otlp"
	"github.com/netdata/netdata/go/plugins/plugin/dem/runtimehub"
	"github.com/netdata/netdata/go/plugins/plugin/dem/secrets"
	"github.com/netdata/netdata/go/plugins/plugin/dem/store"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed charts.yaml
var charts string

//go:embed config_schema.json
var schema string

type Config struct {
	UpdateEvery    int              `yaml:"update_every,omitempty" json:"update_every"`
	Window         confopt.Duration `yaml:"window"                 json:"window"`
	config.RumSite `yaml:",inline" json:""`
	OTLP           config.OTelCfg `yaml:"otlp"                   json:"otlp"`
}
type Dependencies struct {
	Hub     *runtimehub.Hub
	History *store.Store
}
type Collector struct {
	collectorapi.Base
	Config     `yaml:",inline" json:""`
	deps       Dependencies
	store      metrix.CollectorStore
	aggregator *agg.Aggregator
	redactor   *secrets.Redactor
}

func Creator(deps Dependencies) collectorapi.Creator {
	return collectorapi.Creator{
		Defaults: collectorapi.Defaults{
			UpdateEvery: 10,
		},
		CreateV2:        func() collectorapi.CollectorV2 { return New(deps) },
		Config:          func() any { return &Config{} },
		JobConfigSchema: schema,
		StoreFirst:      true,
	}
}
func New(deps Dependencies) *Collector {
	return &Collector{
		Config: Config{
			Window: confopt.Duration(5 * time.Minute),
			RumSite: config.RumSite{
				PageGroups:        20,
				Countries:         20,
				MeasureSampleRate: 1,
				Bots:              config.BotsExclude,
			},
			OTLP: config.OTelCfg{
				Enabled:  "auto",
				Endpoint: "127.0.0.1:4317",
			},
		},
		deps:  deps,
		store: metrix.NewCollectorStore(),
	}
}
func (c *Collector) Configuration() any                 { return c.Config }
func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }
func (c *Collector) ChartTemplateYAML() string          { return charts }

var siteKey = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

func (c *Collector) Init(ctx context.Context) error {
	if c.deps.Hub == nil || c.deps.History == nil {
		return errors.New("missing runtime/history dependencies")
	}
	if !siteKey.MatchString(c.Key) {
		return errors.New("name must be a nonempty URL-safe site key: letters, digits, underscores or hyphens")
	}
	if time.Duration(c.Window) < time.Minute || time.Duration(c.Window) > 30*time.Minute {
		return errors.New("window must be between 1m and 30m")
	}
	if c.PageGroups < 1 || c.PageGroups > 100 || c.Countries < 1 || c.Countries > 100 {
		return errors.New("page_groups and countries must be between 1 and 100")
	}
	if len(c.AllowedOrigins) == 0 {
		return errors.New("allowed_origins is required")
	}
	for _, origin := range c.AllowedOrigins {
		u, err := url.Parse(origin)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil ||
			(u.Path != "" && u.Path != "/") ||
			u.RawQuery != "" ||
			u.Fragment != "" {
			return fmt.Errorf("allowed origin %q must be http(s)://host[:port]", origin)
		}
		host := u.Hostname()
		if strings.Contains(host, "*") &&
			(!strings.HasPrefix(host, "*.") || strings.Contains(host[2:], "*") || len(host) == 2) {
			return errors.New("origin wildcard is only allowed as a leading *.")
		}
	}
	if errs := config.ValidateSiteExtras(c.RumSite); len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	if math.IsNaN(c.MeasureSampleRate) || c.MeasureSampleRate < 0 || c.MeasureSampleRate > 1 {
		return errors.New("measure_sample_rate must be between 0 and 1")
	}
	if c.Investigate != nil {
		if math.IsNaN(c.Investigate.SampleRate) || c.Investigate.SampleRate < 0 || c.Investigate.SampleRate > 1 {
			return errors.New("investigate sample_rate must be between 0 and 1")
		}
		for _, keep := range c.Investigate.AlwaysKeep {
			if keep != config.KeepErrors && keep != config.KeepPoorVitals {
				return errors.New("unknown investigate always_keep condition")
			}
		}
	}
	if c.PublicURL != "" {
		u, err := url.Parse(c.PublicURL)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil ||
			u.RawQuery != "" ||
			u.Fragment != "" {
			return errors.New("public_url must be an http(s) base URL")
		}
	}
	if c.OTLP.Enabled != "auto" && c.OTLP.Enabled != "yes" && c.OTLP.Enabled != "no" {
		return errors.New("otlp.enabled must be auto, yes or no")
	}
	if err := otlp.ValidateConfig(ctx, c.OTLP); err != nil {
		return err
	}
	c.redactor = secrets.NewRedactor(c.OTLP.AuthToken)
	c.aggregator = agg.New(time.Duration(c.Window))
	c.aggregator.Configure(
		time.Duration(c.Window),
		[]agg.SiteCfg{
			{
				Key:        c.Key,
				Name:       c.DisplayName(),
				PageGroups: c.PageGroups,
				Countries:  c.Countries,
				Investigate: agg.InvestigateCfg{
					Rate:           c.InvestigateRate(),
					KeepErrors:     c.KeepsErrors(),
					KeepPoorVitals: c.KeepsPoorVitals(),
				},
			},
		},
	)
	return nil
}
func (c *Collector) Check(context.Context) error {
	if c.aggregator == nil {
		return errors.New("site is not initialized")
	}
	return nil
}

// Run admits an independent site without waiting for receiver startup. Worker
// cancellation occurs only after exact route/read leases have drained.
func (c *Collector) Run(ctx context.Context, ready func()) error {
	if c.aggregator == nil {
		return errors.New("site is not initialized")
	}
	exporter, err := otlp.New(ctx, c.OTLP, c.aggregator, c.redactor)
	if err != nil {
		return err
	}
	defer exporter.Close()
	writer := history.New(c.deps.History, c.aggregator, c.redactor)
	c.aggregator.SetHistorySink(writer)
	route := ingest.NewRoute(c.RumSite, beacon.MultiSink{c.aggregator, exporter})
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return err
	}
	retire, err := c.deps.Hub.Register(
		c.Key,
		&runtimehub.Site{
			Route:      route,
			Aggregator: c.aggregator,
			Generation: hex.EncodeToString(id[:]),
			Redactor:   c.redactor,
		},
	)
	if err != nil {
		return err
	}
	workersCtx, stopWorkers := context.WithCancel(context.Background())
	var workers sync.WaitGroup
	workers.Go(func() { writer.Run(workersCtx) })
	workers.Go(func() { exporter.Run(workersCtx) })
	probeCtx, stopProbes := context.WithCancel(ctx)
	probesDone := make(chan struct{})
	client := &http.Client{
		Timeout: 15 * time.Second,
	}
	go func() {
		defer close(probesDone)
		route.RunReachability(probeCtx, 5*time.Minute, c.probeBase, client)
	}()
	ready()
	<-ctx.Done()
	stopProbes()
	retire()
	<-probesDone
	client.CloseIdleConnections()
	stopWorkers()
	workers.Wait()
	return nil
}
func (c *Collector) Cleanup(context.Context) {}

// probeBase supplies only an explicitly configured address. With none, the
// route probes the trusted-proxy address it learned before promoting it.
func (c *Collector) probeBase() string {
	if c.PublicURL != "" {
		return c.PublicURL
	}
	return c.deps.Hub.Availability().PublicURL
}
