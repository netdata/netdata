// SPDX-License-Identifier: GPL-3.0-or-later
package vsphere

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const v1CompatManifestPath = "testdata/v1_compat_manifest.json"

type v1CompatManifest struct {
	Charts  []v1CompatChart  `json:"charts"`
	Metrics []v1CompatMetric `json:"metrics"`
}

type v1CompatChart struct {
	ID          string              `json:"id"`
	Context     string              `json:"context"`
	Title       string              `json:"title"`
	Units       string              `json:"units"`
	Family      string              `json:"family"`
	Type        string              `json:"type"`
	Priority    int                 `json:"priority"`
	UpdateEvery int                 `json:"update_every,omitempty"`
	SkipGaps    bool                `json:"skip_gaps,omitempty"`
	Detail      bool                `json:"detail,omitempty"`
	Hidden      bool                `json:"hidden,omitempty"`
	StoreFirst  bool                `json:"store_first,omitempty"`
	Labels      []v1CompatLabel     `json:"labels,omitempty"`
	Dimensions  []v1CompatDimension `json:"dimensions"`
}

type v1CompatLabel struct {
	Key    string `json:"key"`
	Source int    `json:"source,omitempty"`
}

type v1CompatDimension struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Algorithm  string `json:"algorithm"`
	Multiplier int    `json:"multiplier"`
	Divisor    int    `json:"divisor"`
	Hidden     bool   `json:"hidden,omitempty"`
	NoReset    bool   `json:"no_reset,omitempty"`
	NoOverflow bool   `json:"no_overflow,omitempty"`
	Float      bool   `json:"float,omitempty"`
}

type v1CompatMetric struct {
	ID    string `json:"id"`
	Value int64  `json:"value"`
}

func TestCollector_V1CompatibilityManifest(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()

	require.NoError(t, collr.Init(context.Background()))
	collr.scraper = mockScraper{collr.scraper}

	mx := collectMapForTest(t, collr)
	require.NotEmpty(t, mx)

	actual := buildV1CompatManifest(collr.Charts(), mx)
	bs, err := json.MarshalIndent(actual, "", "  ")
	require.NoError(t, err)
	bs = append(bs, '\n')

	if os.Getenv("UPDATE_VSPHERE_COMPAT") == "1" {
		require.NoError(t, os.WriteFile(v1CompatManifestPath, bs, 0644))
	}

	expected, err := os.ReadFile(v1CompatManifestPath)
	require.NoError(t, err)
	require.Equal(t, string(expected), string(bs))
}

func TestCollector_V2CompatibilitySurface(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()

	require.NoError(t, collr.Init(context.Background()))
	collr.scraper = mockScraper{collr.scraper}

	require.NotEmpty(t, collectMapForTest(t, collr))

	plan := buildV2PlanForTest(t, collr)
	createdCharts, createdDims := v2CreatedChartsAndDims(plan)
	require.Len(t, createdCharts, len(*collr.Charts()))

	for _, chart := range *collr.Charts() {
		resourceID, ok := chartResourceID(chart)
		require.True(t, ok, "chart %s must have a resource ID", chart.ID)

		v2ChartID := v2ChartTemplateID(chart.Ctx) + "_" + resourceID
		require.NotEqual(t, chart.ID, v2ChartID, "chart IDs are the accepted V2 break")

		created, ok := createdCharts[v2ChartID]
		require.True(t, ok, "missing V2 chart %s for V1 chart %s", v2ChartID, chart.ID)
		require.Equal(t, chart.Title, created.Meta.Title)
		require.Equal(t, chart.Fam, created.Meta.Family)
		require.Equal(t, chart.Ctx, created.Meta.Context)
		require.Equal(t, chart.Units, created.Meta.Units)
		require.Equal(t, chart.Type.String(), string(created.Meta.Type))
		require.Equal(t, chart.Priority, created.Meta.Priority)
		requireV2LabelsCompatible(t, chartLabelsMap(chart), created.Labels, resourceID)

		dims := createdDims[v2ChartID]
		require.Len(t, dims, len(chart.Dims), "dimension count for %s", chart.Ctx)
		for _, dim := range chart.Dims {
			createdDim, ok := dims[dim.Name]
			require.True(t, ok, "missing V2 dimension %s on %s", dim.Name, chart.Ctx)
			require.Equal(t, dim.Algo.String(), string(createdDim.Algorithm))
			require.Equal(t, normalizeV1ChartScale(dim.Mul), createdDim.Multiplier)
			require.Equal(t, normalizeV1ChartScale(dim.Div), createdDim.Divisor)
			require.Equal(t, dim.Hidden, createdDim.Hidden)
			require.Equal(t, dim.Float, createdDim.Float)
		}
	}
}

func buildV1CompatManifest(charts *collectorapi.Charts, mx map[string]int64) v1CompatManifest {
	var manifest v1CompatManifest

	for _, chart := range *charts {
		cc := v1CompatChart{
			ID:          chart.ID,
			Context:     chart.Ctx,
			Title:       chart.Title,
			Units:       chart.Units,
			Family:      chart.Fam,
			Type:        chart.Type.String(),
			Priority:    chart.Priority,
			UpdateEvery: chart.UpdateEvery,
			SkipGaps:    chart.SkipGaps,
			Detail:      chart.Detail,
			Hidden:      chart.Hidden,
			StoreFirst:  chart.StoreFirst,
		}
		for _, label := range chart.Labels {
			source := label.Source
			if source == 0 {
				source = collectorapi.LabelSourceAuto
			}
			cc.Labels = append(cc.Labels, v1CompatLabel{
				Key:    label.Key,
				Source: source,
			})
		}
		for _, dim := range chart.Dims {
			cc.Dimensions = append(cc.Dimensions, v1CompatDimension{
				ID:         dim.ID,
				Name:       dim.Name,
				Algorithm:  dim.Algo.String(),
				Multiplier: normalizeV1ChartScale(dim.Mul),
				Divisor:    normalizeV1ChartScale(dim.Div),
				Hidden:     dim.Hidden,
				NoReset:    dim.NoReset,
				NoOverflow: dim.NoOverflow,
				Float:      dim.Float,
			})
		}
		manifest.Charts = append(manifest.Charts, cc)
	}
	sort.Slice(manifest.Charts, func(i, j int) bool {
		return manifest.Charts[i].ID < manifest.Charts[j].ID
	})

	for id, value := range mx {
		manifest.Metrics = append(manifest.Metrics, v1CompatMetric{ID: id, Value: value})
	}
	sort.Slice(manifest.Metrics, func(i, j int) bool {
		return manifest.Metrics[i].ID < manifest.Metrics[j].ID
	})

	return manifest
}

func normalizeV1ChartScale(v int) int {
	if v == 0 {
		return 1
	}
	return v
}

func buildV2PlanForTest(t *testing.T, collr *Collector) chartengine.Plan {
	t.Helper()

	engine, err := chartengine.New()
	require.NoError(t, err)
	require.NoError(t, engine.LoadYAML([]byte(collr.ChartTemplateYAML()), 1))

	reader := collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten())
	attempt, err := engine.PreparePlan(reader)
	require.NoError(t, err)
	defer attempt.Abort()

	plan := attempt.Plan()
	require.NoError(t, attempt.Commit())
	return plan
}

func v2CreatedChartsAndDims(plan chartengine.Plan) (map[string]chartengine.CreateChartAction, map[string]map[string]chartengine.CreateDimensionAction) {
	charts := make(map[string]chartengine.CreateChartAction)
	dims := make(map[string]map[string]chartengine.CreateDimensionAction)
	for _, action := range plan.Actions {
		switch v := action.(type) {
		case chartengine.CreateChartAction:
			charts[v.ChartID] = v
		case chartengine.CreateDimensionAction:
			if _, ok := dims[v.ChartID]; !ok {
				dims[v.ChartID] = make(map[string]chartengine.CreateDimensionAction)
			}
			dims[v.ChartID][v.Name] = v
		}
	}
	return charts, dims
}

func chartLabelsMap(chart *collectorapi.Chart) map[string]string {
	labels := make(map[string]string, len(chart.Labels))
	for _, label := range chart.Labels {
		key := strings.TrimSpace(label.Key)
		if key == "" {
			continue
		}
		labels[key] = label.Value
	}
	return labels
}

func requireV2LabelsCompatible(t *testing.T, expected, actual map[string]string, resourceID string) {
	t.Helper()

	require.Equal(t, len(expected)+1, len(actual), "V2 should preserve V1 labels and add only the instance id label")
	for key, value := range expected {
		require.Equal(t, value, actual[key], "label %s", key)
	}
	require.Equal(t, resourceID, actual["id"])
}
