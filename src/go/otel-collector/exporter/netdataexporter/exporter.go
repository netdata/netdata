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
}

func newNetdataExporter(cfg component.Config, logger *zap.Logger) *netdataExporter {
	return &netdataExporter{
		log:  logger,
		conf: cfg.(*Config),
	}
}

func (e *netdataExporter) consumeMetrics(ctx context.Context, pm pmetric.Metrics) error {
	return nil
}

func (e *netdataExporter) Start(_ context.Context, _ component.Host) error {
	return nil
}

func (e *netdataExporter) Shutdown(context.Context) error {
	return nil
}
