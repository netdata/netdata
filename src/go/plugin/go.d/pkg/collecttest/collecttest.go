// SPDX-License-Identifier: GPL-3.0-or-later

package collecttest

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
)

func collectOnce(store metrix.CollectorStore, collectFn func(ctx context.Context) error) error {
	if store == nil {
		return fmt.Errorf("collecttest: nil metric store")
	}

	managed, ok := metrix.AsCycleManagedStore(store)
	if !ok {
		return fmt.Errorf("collecttest: metric store is not cycle-managed")
	}

	cc := managed.CycleController()
	committed := false
	cc.BeginCycle()
	defer func() {
		if committed {
			return
		}
		cc.AbortCycle()
	}()
	if err := collectFn(context.Background()); err != nil {
		return err
	}
	cc.CommitCycleSuccess()
	committed = true
	return nil
}

func scalarKeyFromLabelView(name string, labels metrix.LabelView) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if labels == nil || labels.Len() == 0 {
		return name
	}

	var b strings.Builder
	b.WriteString(name)
	b.WriteByte('{')
	first := true
	labels.Range(func(key, value string) bool {
		if !first {
			b.WriteByte(',')
		}
		first = false
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(strconv.Quote(value))
		return true
	})
	b.WriteByte('}')
	return b.String()
}

func scalarKeyFromLabelsMap(name string, labels metrix.Labels) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if len(labels) == 0 {
		return name
	}

	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString(name)
	b.WriteByte('{')
	for i, key := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(strconv.Quote(labels[key]))
	}
	b.WriteByte('}')
	return b.String()
}

func readScalarSeries(reader metrix.Reader) map[string]metrix.SampleValue {
	out := make(map[string]metrix.SampleValue)
	if reader == nil {
		return out
	}
	reader.ForEachSeries(func(name string, labels metrix.LabelView, v metrix.SampleValue) {
		out[scalarKeyFromLabelView(name, labels)] = v
	})
	return out
}

// CollectScalarSeries executes one collector cycle and returns
// scalar points from the collector store using the requested read view.
func CollectScalarSeries(
	collector interface {
		MetricStore() metrix.CollectorStore
		Collect(context.Context) error
	},
	readOpts ...metrix.ReadOption,
) (map[string]metrix.SampleValue, error) {
	if collector == nil {
		return nil, fmt.Errorf("collecttest: nil collector")
	}
	if err := collectOnce(collector.MetricStore(), collector.Collect); err != nil {
		return nil, err
	}
	return readScalarSeries(collector.MetricStore().Read(readOpts...)), nil
}

func buildPlanFromTemplate(templateYAML string, revision uint64, reader metrix.Reader) (chartengine.Plan, error) {
	engine, err := chartengine.New()
	if err != nil {
		return chartengine.Plan{}, err
	}
	if err := engine.LoadYAML([]byte(templateYAML), revision); err != nil {
		return chartengine.Plan{}, err
	}
	return engine.BuildPlan(reader)
}

type planFilter struct {
	ExcludeContexts map[string]struct{}
	ExcludeChartIDs map[string]struct{}
}

type materializedChart struct {
	ID         string
	Context    string
	Dimensions map[string]struct{}
}

func materializedCharts(plan chartengine.Plan, filter planFilter) map[string]materializedChart {
	out := make(map[string]materializedChart)

	include := func(chartID, context string) bool {
		if _, ok := filter.ExcludeChartIDs[chartID]; ok {
			return false
		}
		if _, ok := filter.ExcludeContexts[context]; ok {
			return false
		}
		return true
	}

	for _, action := range plan.Actions {
		switch v := action.(type) {
		case chartengine.CreateChartAction:
			if !include(v.ChartID, v.Meta.Context) {
				continue
			}
			mc := out[v.ChartID]
			mc.ID = v.ChartID
			mc.Context = v.Meta.Context
			if mc.Dimensions == nil {
				mc.Dimensions = make(map[string]struct{})
			}
			out[v.ChartID] = mc
		case chartengine.CreateDimensionAction:
			if !include(v.ChartID, v.ChartMeta.Context) {
				continue
			}
			mc := out[v.ChartID]
			mc.ID = v.ChartID
			if mc.Context == "" {
				mc.Context = v.ChartMeta.Context
			}
			if mc.Dimensions == nil {
				mc.Dimensions = make(map[string]struct{})
			}
			mc.Dimensions[v.Name] = struct{}{}
			out[v.ChartID] = mc
		}
	}

	return out
}
