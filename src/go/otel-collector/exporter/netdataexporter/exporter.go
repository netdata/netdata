package netdataexporter

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"
)

type netdataExporter struct {
	log  *zap.Logger
	conf *Config

	api *netdataAPI

	//MaxLabelCount      int  // Maximum number of labels to include per chart (0 = unlimited)
	//UseShortIDs        bool // Use shortened IDs for better performance
	//GroupSimilarPoints bool // Group similar data points into a single dimension
	//
	//// State management
	//charts     map[string]*ChartDefinition // Map to store and track charts across iterations
	//chartsMu   sync.RWMutex                // Mutex to protect the charts map during concurrent access
	//lastUpdate time.Time                   // Track the last update time

	charts map[string]*ChartDefinition
}

func newNetdataExporter(cfg component.Config, logger *zap.Logger) *netdataExporter {
	return &netdataExporter{
		log:  logger,
		conf: cfg.(*Config),
		api:  newNetdataStdoutApi(),
	}
}

func (e *netdataExporter) consumeMetrics(ctx context.Context, pm pmetric.Metrics) error {
	e.convert(pm)
	e.sendCharts()
	e.updateCharts(0)

	return nil
}

func (e *netdataExporter) Start(_ context.Context, _ component.Host) error {
	return nil
}

func (e *netdataExporter) Shutdown(context.Context) error {
	return nil
}

func (e *netdataExporter) sendCharts() {
	for _, chart := range e.charts {
		if chart.IsNew {
			opts := ChartOpts{
				TypeID:      "",
				ID:          chart.ID,
				Title:       chart.Title,
				Units:       chart.Units,
				Family:      chart.Family,
				Context:     chart.Context,
				ChartType:   chart.Type,
				Priority:    1000,
				UpdateEvery: 1,
				Options:     chart.Options,
				Plugin:      "otel",
				Module:      "metrics",
			}

			e.api.chart(opts)
			for _, label := range chart.Labels {
				e.api.clabel(label.Name, label.Value)
			}
			e.api.clabelcommit()

			for _, dim := range chart.Dimensions {
				dimOpts := DimensionOpts{
					ID:         dim.ID,
					Name:       dim.Name,
					Algorithm:  dim.Algo,
					Multiplier: 1,
					Divisor:    1,
					Options:    "",
				}
				e.api.dimension(dimOpts)
			}

			chart.IsNew = false
		}
	}
}

func (e *netdataExporter) updateCharts(msSince int) {
	for id, chart := range e.charts {
		if len(chart.Dimensions) == 0 {
			continue
		}

		e.api.begin(id, msSince)

		for _, dim := range chart.Dimensions {
			value := int64(dim.Value)
			e.api.set(dim.ID, value)
		}
		e.api.end()
	}
}
