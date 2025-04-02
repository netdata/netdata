package journaldexporter

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.uber.org/zap"
)

type journaldExporter struct {
	log  *zap.Logger
	conf *Config
}

func newJournaldExporter(cfg component.Config, logger *zap.Logger) *journaldExporter {
	return &journaldExporter{
		log:  logger,
		conf: cfg.(*Config),
	}
}

func (e *journaldExporter) consumeLogs(_ context.Context, ld plog.Logs) error {
	fmt.Println("consume logs")
	return nil
}

func (e *journaldExporter) Start(_ context.Context, host component.Host) error {
	host.GetExtensions()
	fmt.Println("Starting MyExporter")
	return nil
}

func (e *journaldExporter) Shutdown(context.Context) error {
	fmt.Println("Shutting down MyExporter")
	return nil
}
