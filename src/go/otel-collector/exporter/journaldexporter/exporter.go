package journaldexporter

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"strconv"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.uber.org/zap"
)

type (
	journaldExporter struct {
		log    *zap.Logger
		conf   *Config
		fields commonFields
	}
	commonFields struct {
		syslogID  string
		pid       string
		uid       string
		bootID    string
		machineID string
		hostname  string
	}
)

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

func (e *journaldExporter) Start(_ context.Context, _ component.Host) error {
	e.fields.syslogID = "nd-otel-collector"
	e.fields.pid = strconv.Itoa(os.Getpid())
	e.fields.hostname, _ = os.Hostname()
	e.fields.bootID = getBootID()
	e.fields.machineID = getMachineID()
	if cu, err := user.Current(); err == nil {
		e.fields.uid = cu.Uid
	}

	return nil
}

func (e *journaldExporter) Shutdown(context.Context) error {
	fmt.Println("Shutting down MyExporter")
	return nil
}
