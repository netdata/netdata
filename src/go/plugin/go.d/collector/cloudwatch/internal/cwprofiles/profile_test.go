// SPDX-License-Identifier: GPL-3.0-or-later

package cwprofiles

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func profileDuration(value time.Duration) *confopt.LongDuration {
	v := confopt.LongDuration(value)
	return &v
}

// validProfile returns a minimal valid EC2-shaped profile with shorthand
// selectors (as authored in stock YAML, before Normalize).
func validProfile() Profile {
	return Profile{
		Version:     VersionV1,
		DisplayName: "AWS EC2",
		Namespace:   "AWS/EC2",
		Period:      300,
		Instance: InstanceSpec{Dimensions: []InstanceDimension{
			{Name: "InstanceId", Label: "instance_id"},
		}},
		Metrics: []Metric{
			{ID: "cpu_utilization", MetricName: "CPUUtilization", Statistics: []string{"average"}},
			{ID: "network_in", MetricName: "NetworkIn", Statistics: []string{"sum"}, Rate: true},
		},
		Template: charttpl.Group{
			Family:           "EC2",
			ContextNamespace: "ec2",
			ChartDefaults: &charttpl.ChartDefaults{
				Instances: &charttpl.Instances{ByLabels: []string{"account_id", "region", "instance_id"}},
			},
			Charts: []charttpl.Chart{
				{
					ID: "aws_cloudwatch_ec2_cpu", Context: "cpu_utilization", Title: "EC2 CPU Utilization",
					Family: "CPU", Units: "percentage", Algorithm: "absolute",
					Dimensions: []charttpl.Dimension{{Selector: "cpu_utilization_average", Name: "average"}},
				},
				{
					ID: "aws_cloudwatch_ec2_network", Context: "network_traffic", Title: "EC2 Network Traffic",
					Family: "Network", Units: "bytes/s", Algorithm: "absolute",
					Dimensions: []charttpl.Dimension{{Selector: "network_in_sum", Name: "in"}},
				},
			},
		},
	}
}

func TestProfile_Validate(t *testing.T) {
	tests := map[string]struct {
		mutate      func(p *Profile)
		wantErr     bool
		errContains string
	}{
		"valid": {
			mutate: func(*Profile) {},
		},
		"wrong version": {
			mutate:      func(p *Profile) { p.Version = "v2" },
			wantErr:     true,
			errContains: "version",
		},
		"missing display_name": {
			mutate:      func(p *Profile) { p.DisplayName = "" },
			wantErr:     true,
			errContains: "display_name",
		},
		"invalid namespace": {
			mutate:      func(p *Profile) { p.Namespace = "AWS EC2!" },
			wantErr:     true,
			errContains: "namespace",
		},
		"namespace with surrounding whitespace": {
			mutate:      func(p *Profile) { p.Namespace = " AWS/EC2 " },
			wantErr:     true,
			errContains: "namespace",
		},
		"period not multiple of 60": {
			mutate:      func(p *Profile) { p.Period = 90 },
			wantErr:     true,
			errContains: "period",
		},
		"period zero": {
			mutate:      func(p *Profile) { p.Period = 0 },
			wantErr:     true,
			errContains: "period",
		},
		"publication delay zero": {
			mutate: func(p *Profile) { p.PublicationDelay = profileDuration(0) },
		},
		"publication delay negative": {
			mutate:      func(p *Profile) { p.PublicationDelay = profileDuration(-time.Second) },
			wantErr:     true,
			errContains: "publication_delay",
		},
		"publication delay subsecond": {
			mutate:      func(p *Profile) { p.PublicationDelay = profileDuration(time.Second + time.Millisecond) },
			wantErr:     true,
			errContains: "whole seconds",
		},
		"supported regions valid": {
			mutate: func(p *Profile) { p.SupportedRegions = []string{"us-east-1", "eu-west-1"} },
		},
		"supported regions eusc partition valid": {
			mutate: func(p *Profile) { p.SupportedRegions = []string{"eusc-de-east-1"} },
		},
		"supported region with surrounding whitespace invalid": {
			mutate:      func(p *Profile) { p.SupportedRegions = []string{" us-east-1 "} },
			wantErr:     true,
			errContains: "canonical",
		},
		"supported region with uppercase invalid": {
			mutate:      func(p *Profile) { p.SupportedRegions = []string{"US-EAST-1"} },
			wantErr:     true,
			errContains: "canonical",
		},
		"supported regions explicit empty invalid": {
			mutate:      func(p *Profile) { p.SupportedRegions = []string{} },
			wantErr:     true,
			errContains: "supported_regions",
		},
		"supported regions invalid code": {
			mutate:      func(p *Profile) { p.SupportedRegions = []string{"global"} },
			wantErr:     true,
			errContains: "supported_regions",
		},
		"supported regions duplicate": {
			mutate:      func(p *Profile) { p.SupportedRegions = []string{"us-east-1", "us-east-1"} },
			wantErr:     true,
			errContains: "duplicate",
		},
		"per-metric period override invalid": {
			mutate:      func(p *Profile) { p.Metrics[0].Period = 45 },
			wantErr:     true,
			errContains: "period",
		},
		"no instance dimensions": {
			mutate:      func(p *Profile) { p.Instance.Dimensions = nil },
			wantErr:     true,
			errContains: "instance.dimensions",
		},
		"dimension missing name": {
			mutate:      func(p *Profile) { p.Instance.Dimensions[0].Name = "" },
			wantErr:     true,
			errContains: "name",
		},
		"dimension invalid label": {
			mutate:      func(p *Profile) { p.Instance.Dimensions[0].Label = "Instance_ID" },
			wantErr:     true,
			errContains: "label",
		},
		"dimension label is reserved": {
			mutate:      func(p *Profile) { p.Instance.Dimensions[0].Label = "region" },
			wantErr:     true,
			errContains: "reserved",
		},
		"constant dimension valid (match-and-query-only, no label required)": {
			mutate: func(p *Profile) {
				global := "Global"
				p.Instance.Dimensions = append(p.Instance.Dimensions, InstanceDimension{Name: "Region", Constant: &global})
			},
		},
		"dimension with both label and constant is invalid": {
			mutate: func(p *Profile) {
				global := "Global"
				p.Instance.Dimensions[0].Constant = &global
			},
			wantErr:     true,
			errContains: "exactly one",
		},
		"empty constant is invalid": {
			mutate: func(p *Profile) {
				empty := ""
				p.Instance.Dimensions = append(p.Instance.Dimensions, InstanceDimension{Name: "Region", Constant: &empty})
			},
			wantErr:     true,
			errContains: "constant",
		},
		"constant with surrounding whitespace is invalid": {
			mutate: func(p *Profile) {
				padded := "Global "
				p.Instance.Dimensions = append(p.Instance.Dimensions, InstanceDimension{Name: "Region", Constant: &padded})
			},
			wantErr:     true,
			errContains: "whitespace",
		},
		"dimension name with surrounding whitespace is invalid": {
			mutate:      func(p *Profile) { p.Instance.Dimensions[0].Name = "InstanceId " },
			wantErr:     true,
			errContains: "whitespace",
		},
		"period above maximum": {
			mutate:      func(p *Profile) { p.Period = 172800 },
			wantErr:     true,
			errContains: "period",
		},
		"by_labels wildcard excludes required label": {
			mutate: func(p *Profile) {
				p.Template.ChartDefaults.Instances.ByLabels = []string{"*", "!region"}
			},
			wantErr:     true,
			errContains: "by_labels must include",
		},
		"duplicate dimension label": {
			mutate: func(p *Profile) {
				p.Instance.Dimensions = append(p.Instance.Dimensions, InstanceDimension{Name: "ImageId", Label: "instance_id"})
			},
			wantErr:     true,
			errContains: "duplicate dimension label",
		},
		"no metrics": {
			mutate:      func(p *Profile) { p.Metrics = nil },
			wantErr:     true,
			errContains: "metrics",
		},
		"metric invalid id": {
			mutate:      func(p *Profile) { p.Metrics[0].ID = "CPU" },
			wantErr:     true,
			errContains: "id",
		},
		"metric missing metric_name": {
			mutate:      func(p *Profile) { p.Metrics[0].MetricName = "" },
			wantErr:     true,
			errContains: "metric_name",
		},
		"metric_name surrounding whitespace": {
			mutate:      func(p *Profile) { p.Metrics[0].MetricName = " CPUUtilization " },
			wantErr:     true,
			errContains: "must not have surrounding whitespace",
		},
		"metric no statistics": {
			mutate:      func(p *Profile) { p.Metrics[0].Statistics = nil },
			wantErr:     true,
			errContains: "statistics",
		},
		"metric invalid statistic": {
			mutate:      func(p *Profile) { p.Metrics[0].Statistics = []string{"bogus"} },
			wantErr:     true,
			errContains: "not a valid statistic",
		},
		"duplicate statistic": {
			mutate:      func(p *Profile) { p.Metrics[0].Statistics = []string{"average", "average"} },
			wantErr:     true,
			errContains: "duplicate statistic",
		},
		"rate without sum or sample_count": {
			mutate:      func(p *Profile) { p.Metrics[0].Rate = true },
			wantErr:     true,
			errContains: "requires a 'sum' or 'sample_count' statistic",
		},
		"rate with sum is valid": {
			mutate: func(p *Profile) {
				p.Metrics[0].Statistics = []string{"sum"}
				p.Metrics[0].Rate = true
				p.Template.Charts[0].Dimensions[0].Selector = "cpu_utilization_sum"
			},
		},
		"rate with sample_count is valid": {
			mutate: func(p *Profile) {
				p.Metrics[0].Statistics = []string{"sample_count"}
				p.Metrics[0].Rate = true
				p.Template.Charts[0].Dimensions[0].Selector = "cpu_utilization_sample_count"
			},
		},
		"percentile statistic is valid": {
			mutate: func(p *Profile) {
				p.Metrics[0].Statistics = []string{"p90"}
				p.Template.Charts[0].Dimensions[0].Selector = "cpu_utilization_p90"
			},
		},
		"duplicate metric id": {
			mutate: func(p *Profile) {
				p.Metrics = append(p.Metrics, Metric{ID: "cpu_utilization", MetricName: "Other", Statistics: []string{"average"}})
			},
			wantErr:     true,
			errContains: "duplicate metric id",
		},
		"duplicate metric_name": {
			mutate: func(p *Profile) {
				p.Metrics = append(p.Metrics, Metric{ID: "cpu2", MetricName: "CPUUtilization", Statistics: []string{"average"}})
			},
			wantErr:     true,
			errContains: "duplicate metric_name",
		},
		"template.metrics authored": {
			mutate:      func(p *Profile) { p.Template.Metrics = []string{"cpu_utilization_average"} },
			wantErr:     true,
			errContains: "collector-owned",
		},
		"no charts": {
			mutate:      func(p *Profile) { p.Template.Charts = nil },
			wantErr:     true,
			errContains: "at least one chart",
		},
		"chart algorithm not absolute": {
			mutate:      func(p *Profile) { p.Template.Charts[0].Algorithm = "incremental" },
			wantErr:     true,
			errContains: "must be 'absolute'",
		},
		"chart algorithm empty": {
			mutate:      func(p *Profile) { p.Template.Charts[0].Algorithm = "" },
			wantErr:     true,
			errContains: "must be 'absolute'",
		},
		"selector does not resolve": {
			mutate:      func(p *Profile) { p.Template.Charts[0].Dimensions[0].Selector = "nonexistent_average" },
			wantErr:     true,
			errContains: "not visible",
		},
		"by_labels missing account_id": {
			mutate: func(p *Profile) {
				p.Template.ChartDefaults.Instances.ByLabels = []string{"region", "instance_id"}
			},
			wantErr:     true,
			errContains: "by_labels must include",
		},
		"by_labels missing region": {
			mutate: func(p *Profile) {
				p.Template.ChartDefaults.Instances.ByLabels = []string{"account_id", "instance_id"}
			},
			wantErr:     true,
			errContains: "by_labels must include",
		},
		"by_labels missing dimension label": {
			mutate: func(p *Profile) {
				p.Template.ChartDefaults.Instances.ByLabels = []string{"account_id", "region"}
			},
			wantErr:     true,
			errContains: "by_labels must include",
		},
		"by_labels wildcard satisfies identity": {
			mutate: func(p *Profile) {
				p.Template.ChartDefaults.Instances.ByLabels = []string{"*"}
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			p := validProfile()
			tc.mutate(&p)

			require.NoError(t, p.Normalize("ec2"))
			err := p.Validate("profile \"ec2\"", "ec2")

			if tc.wantErr {
				require.Error(t, err)
				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestProfile_SupportedRegionsCanonicalAndMatch(t *testing.T) {
	p := validProfile()
	p.SupportedRegions = []string{"us-east-1", "eu-west-1"}
	require.NoError(t, p.Normalize("ec2"))
	require.NoError(t, p.Validate("profile", "ec2"))
	assert.Equal(t, []string{"us-east-1", "eu-west-1"}, p.SupportedRegions)
	assert.True(t, p.SupportsRegion("US-EAST-1"))
	assert.False(t, p.SupportsRegion("ap-southeast-1"))

	unrestricted := validProfile()
	assert.True(t, unrestricted.SupportsRegion("ap-southeast-1"))
}

func TestWalkChartAlgorithms_NestedGroup(t *testing.T) {
	group := charttpl.Group{
		Charts: []charttpl.Chart{{Context: "ok", Algorithm: "absolute"}},
		Groups: []charttpl.Group{{
			Charts: []charttpl.Chart{{Context: "bad", Algorithm: "incremental"}},
		}},
	}
	var errs []error
	walkChartAlgorithms("profile.template", group, &errs)

	require.Len(t, errs, 1, "the nested non-absolute chart must be caught")
	assert.Contains(t, errs[0].Error(), "bad")
	assert.Contains(t, errs[0].Error(), "absolute")
}

func TestProfile_Normalize_Selectors(t *testing.T) {
	tests := map[string]struct {
		mutate func(*Profile)
		want   map[int]string
	}{
		"prefixes selectors": {
			want: map[int]string{0: "ec2.cpu_utilization_average", 1: "ec2.network_in_sum"},
		},
		"leaves unknown selector untouched": {
			mutate: func(p *Profile) { p.Template.Charts[0].Dimensions[0].Selector = "unknown_series" },
			want:   map[int]string{0: "unknown_series"},
		},
		// A percentile token's dot does not make the shorthand selector qualified.
		"prefixes decimal percentile selector": {
			mutate: func(p *Profile) {
				p.Metrics = append(p.Metrics, Metric{ID: "latency", MetricName: "Latency", Statistics: []string{"p99.9"}})
				p.Template.Charts[0].Dimensions[0].Selector = "latency_p99.9"
			},
			want: map[int]string{0: "ec2.latency_p99.9"},
		},
		// An already-qualified visible selector must not be prefixed a second time.
		"leaves qualified selector untouched": {
			mutate: func(p *Profile) { p.Template.Charts[0].Dimensions[0].Selector = "ec2.cpu_utilization_average" },
			want:   map[int]string{0: "ec2.cpu_utilization_average"},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			p := validProfile()
			if tc.mutate != nil {
				tc.mutate(&p)
			}
			require.NoError(t, p.Normalize("ec2"))
			for chart, want := range tc.want {
				assert.Equal(t, want, p.Template.Charts[chart].Dimensions[0].Selector)
			}
		})
	}
}

func TestNormalizeStatistic(t *testing.T) {
	tests := map[string]string{
		"average":      "average",
		"AVERAGE":      "average",
		"  sum  ":      "sum",
		"minimum":      "minimum",
		"maximum":      "maximum",
		"sample_count": "sample_count",
		"p90":          "p90",
		"p99.9":        "p99.9",
		"p100":         "p100",
		"p100.0":       "p100.0",
		"p0":           "p0",
		"min":          "",
		"total":        "",
		"p101":         "",
		"p100.5":       "",
		"":             "",
		"bogus":        "",
	}
	for in, want := range tests {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, want, NormalizeStatistic(in))
		})
	}
}

func TestStatString(t *testing.T) {
	tests := map[string]string{
		"average":      "Average",
		"minimum":      "Minimum",
		"maximum":      "Maximum",
		"sum":          "Sum",
		"sample_count": "SampleCount",
		"p90":          "p90",
		"p99.9":        "p99.9",
	}
	for in, want := range tests {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, want, StatString(in))
		})
	}
}

func TestExportedSeriesName(t *testing.T) {
	tests := map[string]struct {
		profile, metric, statistic, want string
	}{
		"standard statistic": {profile: "ec2", metric: "cpu_utilization", statistic: "average", want: "ec2.cpu_utilization_average"},
		"percentile":         {profile: "lambda", metric: "duration", statistic: "p90", want: "lambda.duration_p90"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, ExportedSeriesName(tc.profile, tc.metric, tc.statistic))
		})
	}
}

func TestProfile_EffectivePeriod(t *testing.T) {
	p := Profile{Period: 300}
	tests := map[string]struct {
		metric Metric
		want   int
	}{
		"inherits profile": {want: 300},
		"metric override":  {metric: Metric{Period: 60}, want: 60},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, p.EffectivePeriod(tc.metric))
		})
	}
}

func TestProfile_DimensionNames(t *testing.T) {
	p := Profile{Instance: InstanceSpec{Dimensions: []InstanceDimension{
		{Name: "BucketName", Label: "bucket_name"},
		{Name: "StorageType", Label: "storage_type"},
	}}}
	assert.Equal(t, []string{"BucketName", "StorageType"}, p.DimensionNames())
}

func TestMetric_EmitZeroOnNoData(t *testing.T) {
	yes, no := true, false
	tests := map[string]struct {
		metric Metric
		token  string
		want   bool
	}{
		"rate sum defaults to zero":          {metric: Metric{Rate: true}, token: "sum", want: true},
		"rate sample_count defaults to zero": {metric: Metric{Rate: true}, token: "sample_count", want: true},
		"rate average defaults to gap":       {metric: Metric{Rate: true}, token: "average", want: false},
		"gauge sum defaults to gap":          {metric: Metric{Rate: false}, token: "sum", want: false},
		"explicit true overrides (any stat)": {metric: Metric{Rate: false, NilAsZero: &yes}, token: "average", want: true},
		"explicit false overrides rate":      {metric: Metric{Rate: true, NilAsZero: &no}, token: "sum", want: false},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.metric.EmitZeroOnNoData(tc.token))
		})
	}
}
