package journaldexporter

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

type journaldExporter struct {
	metricsMarshaler pmetric.Marshaler
}

func newMyExporter() *journaldExporter {
	return &journaldExporter{
		metricsMarshaler: &pmetric.JSONMarshaler{},
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
