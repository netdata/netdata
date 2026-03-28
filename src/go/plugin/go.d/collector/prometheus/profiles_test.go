// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
)

func TestCollector_validateConfigProfiles(t *testing.T) {
	tests := map[string]struct {
		mode     string
		profiles []string
		wantMode string
		wantErr  bool
	}{
		"default mode is auto": {
			wantMode: profileSelectionModeAuto,
		},
		"auto rejects explicit profiles": {
			mode:     profileSelectionModeAuto,
			profiles: []string{"demo"},
			wantErr:  true,
		},
		"exact requires explicit profiles": {
			mode:    profileSelectionModeExact,
			wantErr: true,
		},
		"combined requires explicit profiles": {
			mode:    profileSelectionModeCombined,
			wantErr: true,
		},
		"exact accepts explicit profiles": {
			mode:     profileSelectionModeExact,
			profiles: []string{"demo"},
			wantMode: profileSelectionModeExact,
		},
		"combined accepts explicit profiles": {
			mode:     profileSelectionModeCombined,
			profiles: []string{"demo"},
			wantMode: profileSelectionModeCombined,
		},
		"duplicate explicit profiles fail": {
			mode:     profileSelectionModeExact,
			profiles: []string{"demo", "DEMO"},
			wantErr:  true,
		},
		"empty explicit profile fails": {
			mode:     profileSelectionModeExact,
			profiles: []string{" "},
			wantErr:  true,
		},
		"unsupported mode fails": {
			mode:    "bad",
			wantErr: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.URL = "http://127.0.0.1:9090/metrics"
			collr.ProfileSelectionMode = test.mode
			collr.Profiles = append([]string(nil), test.profiles...)

			err := collr.validateConfig()
			if test.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.wantMode, collr.ProfileSelectionMode)
		})
	}
}

func TestCollector_InitLoadProfileCatalogError(t *testing.T) {
	collr := New()
	collr.URL = "http://127.0.0.1:9090/metrics"
	collr.loadProfileCatalog = func() (promprofiles.Catalog, error) {
		return promprofiles.Catalog{}, errors.New("boom")
	}

	err := collr.Init(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "load profiles catalog")
}

func TestCollector_ChartTemplateYAML_RuntimeOverride(t *testing.T) {
	collr := New()
	runtime, err := buildCollectorRuntimeFromProfiles([]promprofiles.Profile{testPromProfile("demo")})
	require.NoError(t, err)

	collr.runtime = runtime

	templateYAML := collr.ChartTemplateYAML()
	collecttest.AssertChartTemplateSchema(t, templateYAML)
	assert.Contains(t, templateYAML, "demo_metric_total")
	assert.Contains(t, templateYAML, "enabled: true")
}

func testPromProfile(id string) promprofiles.Profile {
	return promprofiles.Profile{
		ID:    id,
		Name:  "Demo",
		Match: "demo_*",
		Template: charttpl.Group{
			Family:  "prometheus_curated",
			Metrics: []string{"demo_metric_total"},
			Charts: []charttpl.Chart{
				{
					ID:      id + "_chart",
					Title:   "Demo Metric",
					Context: "prometheus.demo.metric",
					Units:   "events",
					Dimensions: []charttpl.Dimension{
						{Selector: "demo_metric_total", Name: "value"},
					},
				},
			},
		},
	}
}
