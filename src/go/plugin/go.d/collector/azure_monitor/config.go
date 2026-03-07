// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
)

const (
	defaultUpdateEvery      = 60
	defaultAutoDetectRetry  = 0
	defaultCloud            = cloudPublic
	defaultDiscoveryEvery   = 300
	defaultQueryOffset      = 180
	defaultMaxConcurrency   = 4
	defaultMaxBatchResource = 50
	defaultMaxMetricsQuery  = 20
)

const (
	authModeServicePrincipal = "service_principal"
	authModeManagedIdentity  = "managed_identity"
	authModeDefault          = "default"
)

const profileAutoKeyword = "auto"

const (
	cloudPublic     = "public"
	cloudGovernment = "government"
	cloudChina      = "china"
)

var (
	reResourceType = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)
)

type Config struct {
	Vnode              string     `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int        `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int        `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	SubscriptionID     string     `yaml:"subscription_id" json:"subscription_id"`
	Cloud              string     `yaml:"cloud,omitempty" json:"cloud"`
	DiscoveryEvery     int        `yaml:"discovery_every,omitempty" json:"discovery_every"`
	QueryOffset        int        `yaml:"query_offset,omitempty" json:"query_offset"`
	MaxConcurrency     int        `yaml:"max_concurrency,omitempty" json:"max_concurrency"`
	MaxBatchResources  int        `yaml:"max_batch_resources,omitempty" json:"max_batch_resources"`
	MaxMetricsPerQuery int        `yaml:"max_metrics_per_query,omitempty" json:"max_metrics_per_query"`
	ResourceGroups     []string   `yaml:"resource_groups,omitempty" json:"resource_groups"`
	Profiles           []string   `yaml:"profiles,omitempty" json:"profiles"`
	Auth               AuthConfig `yaml:"auth" json:"auth"`
}

type AuthConfig struct {
	Mode                    string `yaml:"mode,omitempty" json:"mode,omitempty"`
	TenantID                string `yaml:"tenant_id,omitempty" json:"tenant_id,omitempty"`
	ClientID                string `yaml:"client_id,omitempty" json:"client_id,omitempty"`
	ClientSecret            string `yaml:"client_secret,omitempty" json:"client_secret,omitempty"`
	ManagedIdentityClientID string `yaml:"managed_identity_client_id,omitempty" json:"managed_identity_client_id,omitempty"`
}

type ProfileConfig struct {
	Name            string         `yaml:"name" json:"name,omitempty"`
	ResourceType    string         `yaml:"resource_type" json:"resource_type,omitempty"`
	MetricNamespace string         `yaml:"metric_namespace,omitempty" json:"metric_namespace,omitempty"`
	Metrics         []MetricConfig `yaml:"metrics" json:"metrics,omitempty"`
	Charts          []ProfileChart `yaml:"charts" json:"charts,omitempty"`
}

type MetricConfig struct {
	Name         string   `yaml:"name" json:"name,omitempty"`
	Units        string   `yaml:"units,omitempty" json:"units,omitempty"`
	Aggregations []string `yaml:"aggregations,omitempty" json:"aggregations,omitempty"`
	TimeGrain    string   `yaml:"time_grain,omitempty" json:"time_grain,omitempty"`
}

type ProfileChart struct {
	ID            string                  `yaml:"id" json:"id,omitempty"`
	Title         string                  `yaml:"title" json:"title,omitempty"`
	Context       string                  `yaml:"context" json:"context,omitempty"`
	Family        string                  `yaml:"family,omitempty" json:"family,omitempty"`
	Units         string                  `yaml:"units" json:"units,omitempty"`
	Algorithm     string                  `yaml:"algorithm,omitempty" json:"algorithm,omitempty"`
	Type          string                  `yaml:"type,omitempty" json:"type,omitempty"`
	Priority      int                     `yaml:"priority,omitempty" json:"priority,omitempty"`
	LabelPromoted []string                `yaml:"label_promotion,omitempty" json:"label_promotion,omitempty"`
	Instances     *charttpl.Instances     `yaml:"instances,omitempty" json:"instances,omitempty"`
	Lifecycle     *charttpl.Lifecycle     `yaml:"lifecycle,omitempty" json:"lifecycle,omitempty"`
	Dimensions    []ProfileChartDimension `yaml:"dimensions" json:"dimensions,omitempty"`
}

type ProfileChartDimension struct {
	Metric        string                     `yaml:"metric" json:"metric,omitempty"`
	Aggregation   string                     `yaml:"aggregation" json:"aggregation,omitempty"`
	Name          string                     `yaml:"name,omitempty" json:"name,omitempty"`
	NameFromLabel string                     `yaml:"name_from_label,omitempty" json:"name_from_label,omitempty"`
	Options       *charttpl.DimensionOptions `yaml:"options,omitempty" json:"options,omitempty"`
}

func (c *Config) applyDefaults() {
	if c.UpdateEvery <= 0 {
		c.UpdateEvery = defaultUpdateEvery
	}
	if c.AutoDetectionRetry < 0 {
		c.AutoDetectionRetry = defaultAutoDetectRetry
	}
	if strings.TrimSpace(c.Cloud) == "" {
		c.Cloud = defaultCloud
	}
	if c.DiscoveryEvery <= 0 {
		c.DiscoveryEvery = defaultDiscoveryEvery
	}
	if c.QueryOffset <= 0 {
		c.QueryOffset = defaultQueryOffset
	}
	if c.MaxConcurrency <= 0 {
		c.MaxConcurrency = defaultMaxConcurrency
	}
	if c.MaxBatchResources <= 0 {
		c.MaxBatchResources = defaultMaxBatchResource
	}
	if c.MaxMetricsPerQuery <= 0 {
		c.MaxMetricsPerQuery = defaultMaxMetricsQuery
	}
	if strings.TrimSpace(c.Auth.Mode) == "" {
		c.Auth.Mode = authModeDefault
	}
}

func (c Config) validate() error {
	var errs []error

	if strings.TrimSpace(c.SubscriptionID) == "" {
		errs = append(errs, errors.New("'subscription_id' is required"))
	}
	if c.UpdateEvery < 60 {
		errs = append(errs, errors.New("'update_every' must be >= 60 seconds"))
	}
	if c.DiscoveryEvery < 60 {
		errs = append(errs, errors.New("'discovery_every' must be >= 60 seconds"))
	}
	if c.QueryOffset < 60 {
		errs = append(errs, errors.New("'query_offset' must be >= 60 seconds"))
	}
	if c.MaxConcurrency < 1 || c.MaxConcurrency > 64 {
		errs = append(errs, errors.New("'max_concurrency' must be between 1 and 64"))
	}
	if c.MaxBatchResources < 1 || c.MaxBatchResources > 50 {
		errs = append(errs, errors.New("'max_batch_resources' must be between 1 and 50"))
	}
	if c.MaxMetricsPerQuery < 1 || c.MaxMetricsPerQuery > 20 {
		errs = append(errs, errors.New("'max_metrics_per_query' must be between 1 and 20"))
	}

	switch strings.ToLower(strings.TrimSpace(c.Cloud)) {
	case cloudPublic, cloudGovernment, cloudChina:
	default:
		errs = append(errs, fmt.Errorf("'cloud' must be one of: %s, %s, %s", cloudPublic, cloudGovernment, cloudChina))
	}

	if err := c.Auth.validate(); err != nil {
		errs = append(errs, err)
	}

	seenProfiles := map[string]struct{}{}
	for _, name := range c.Profiles {
		n := strings.TrimSpace(name)
		if n == "" {
			errs = append(errs, errors.New("'profiles' contains an empty value"))
			continue
		}
		norm := stringsLowerTrim(n)
		if _, ok := seenProfiles[norm]; ok {
			errs = append(errs, fmt.Errorf("'profiles' contains duplicate value '%s'", n))
		}
		seenProfiles[norm] = struct{}{}
	}

	return errors.Join(errs...)
}

func (a AuthConfig) validate() error {
	mode := strings.ToLower(strings.TrimSpace(a.Mode))
	if mode == "" {
		mode = authModeDefault
	}

	switch mode {
	case authModeServicePrincipal:
		var errs []error
		if strings.TrimSpace(a.TenantID) == "" {
			errs = append(errs, errors.New("'auth.tenant_id' is required for service_principal mode"))
		}
		if strings.TrimSpace(a.ClientID) == "" {
			errs = append(errs, errors.New("'auth.client_id' is required for service_principal mode"))
		}
		if strings.TrimSpace(a.ClientSecret) == "" {
			errs = append(errs, errors.New("'auth.client_secret' is required for service_principal mode"))
		}
		return errors.Join(errs...)
	case authModeManagedIdentity, authModeDefault:
		return nil
	default:
		return fmt.Errorf("'auth.mode' must be one of: %s, %s, %s", authModeServicePrincipal, authModeManagedIdentity, authModeDefault)
	}
}

func (p ProfileConfig) validate(prefix string) error {
	var errs []error

	if strings.TrimSpace(p.Name) == "" {
		errs = append(errs, fmt.Errorf("%s: 'name' is required", prefix))
	}
	if !reResourceType.MatchString(strings.TrimSpace(p.ResourceType)) {
		errs = append(errs, fmt.Errorf("%s: 'resource_type' is invalid", prefix))
	}
	if strings.TrimSpace(p.MetricNamespace) != "" && !reResourceType.MatchString(strings.TrimSpace(p.MetricNamespace)) {
		errs = append(errs, fmt.Errorf("%s: 'metric_namespace' is invalid", prefix))
	}
	if len(p.Metrics) == 0 {
		errs = append(errs, fmt.Errorf("%s: 'metrics' must contain at least one metric", prefix))
	}
	if len(p.Charts) == 0 {
		errs = append(errs, fmt.Errorf("%s: 'charts' must contain at least one chart", prefix))
	}

	seenMetricNames := map[string]struct{}{}
	for j, m := range p.Metrics {
		if err := m.validate(prefix, j); err != nil {
			errs = append(errs, err)
		}
		name := strings.ToLower(strings.TrimSpace(m.Name))
		if name == "" {
			continue
		}
		if _, ok := seenMetricNames[name]; ok {
			errs = append(errs, fmt.Errorf("%s: duplicate metric name '%s'", prefix, m.Name))
		}
		seenMetricNames[name] = struct{}{}
	}

	seenChartIDs := map[string]struct{}{}
	for i, ch := range p.Charts {
		if err := ch.validate(prefix, i); err != nil {
			errs = append(errs, err)
		}
		id := stringsLowerTrim(ch.ID)
		if id == "" {
			continue
		}
		if _, ok := seenChartIDs[id]; ok {
			errs = append(errs, fmt.Errorf("%s: duplicate chart id '%s'", prefix, ch.ID))
		}
		seenChartIDs[id] = struct{}{}
	}

	return errors.Join(errs...)
}

func (m MetricConfig) validate(profilePrefix string, idx int) error {
	prefix := fmt.Sprintf("%s.metrics[%d]", profilePrefix, idx)
	var errs []error

	if strings.TrimSpace(m.Name) == "" {
		errs = append(errs, fmt.Errorf("%s: 'name' is required", prefix))
	}
	if strings.TrimSpace(m.TimeGrain) != "" {
		if _, ok := supportedTimeGrains[strings.ToUpper(strings.TrimSpace(m.TimeGrain))]; !ok {
			errs = append(errs, fmt.Errorf("%s: unsupported time_grain '%s'", prefix, m.TimeGrain))
		}
	}
	if len(m.Aggregations) == 0 {
		return errors.Join(errs...)
	}

	seen := map[string]struct{}{}
	for _, a := range m.Aggregations {
		agg := normalizeAggregation(a)
		if agg == "" {
			errs = append(errs, fmt.Errorf("%s: aggregation '%s' is invalid", prefix, a))
			continue
		}
		if _, ok := seen[agg]; ok {
			errs = append(errs, fmt.Errorf("%s: duplicate aggregation '%s'", prefix, agg))
			continue
		}
		seen[agg] = struct{}{}
	}
	return errors.Join(errs...)
}

func (c ProfileChart) validate(profilePrefix string, idx int) error {
	prefix := fmt.Sprintf("%s.charts[%d]", profilePrefix, idx)
	var errs []error

	if stringsTrim(c.ID) == "" {
		errs = append(errs, fmt.Errorf("%s: 'id' is required", prefix))
	}
	if stringsTrim(c.Title) == "" {
		errs = append(errs, fmt.Errorf("%s: 'title' is required", prefix))
	}
	if stringsTrim(c.Context) == "" {
		errs = append(errs, fmt.Errorf("%s: 'context' is required", prefix))
	}
	if stringsTrim(c.Units) == "" {
		errs = append(errs, fmt.Errorf("%s: 'units' is required", prefix))
	}
	if len(c.Dimensions) == 0 {
		errs = append(errs, fmt.Errorf("%s: 'dimensions' must contain at least one dimension", prefix))
	}

	seenNames := map[string]struct{}{}
	for i, d := range c.Dimensions {
		if err := d.validate(prefix, i); err != nil {
			errs = append(errs, err)
		}
		name := stringsLowerTrim(d.Name)
		if name == "" {
			continue
		}
		if _, ok := seenNames[name]; ok {
			errs = append(errs, fmt.Errorf("%s: duplicate dimension name '%s'", prefix, d.Name))
			continue
		}
		seenNames[name] = struct{}{}
	}

	return errors.Join(errs...)
}

func (d ProfileChartDimension) validate(chartPrefix string, idx int) error {
	prefix := fmt.Sprintf("%s.dimensions[%d]", chartPrefix, idx)
	var errs []error

	if stringsTrim(d.Metric) == "" {
		errs = append(errs, fmt.Errorf("%s: 'metric' is required", prefix))
	}
	if normalizeAggregation(d.Aggregation) == "" {
		errs = append(errs, fmt.Errorf("%s: 'aggregation' must be one of average, minimum, maximum, total, count", prefix))
	}
	if stringsTrim(d.Name) != "" && stringsTrim(d.NameFromLabel) != "" {
		errs = append(errs, fmt.Errorf("%s: only one of 'name' and 'name_from_label' may be set", prefix))
	}

	return errors.Join(errs...)
}

func normalizeAggregation(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "average":
		return "average"
	case "minimum", "min":
		return "minimum"
	case "maximum", "max":
		return "maximum"
	case "total", "sum":
		return "total"
	case "count":
		return "count"
	default:
		return ""
	}
}

func normalizeAggregations(values []string) []string {
	if len(values) == 0 {
		return []string{"average"}
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, v := range values {
		n := normalizeAggregation(v)
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	if len(out) == 0 {
		out = append(out, "average")
	}
	slices.Sort(out)
	return out
}
