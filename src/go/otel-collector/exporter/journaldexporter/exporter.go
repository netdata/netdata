// SPDX-License-Identifier: GPL-3.0-or-later

package journaldexporter

import (
	"bytes"
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
		s      sender
		buf    bytes.Buffer
	}
	sender interface {
		sendMessage(ctx context.Context, msg []byte) error
		shutdown(ctx context.Context) error
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

func (e *journaldExporter) consumeLogs(ctx context.Context, ld plog.Logs) error {
	if e.s == nil {
		return nil
	}

	e.buf.Reset()

	e.logsToJournaldMessages(ld, &e.buf)

	select {
	case <-ctx.Done():
		return nil
	default:
		return e.s.sendMessage(ctx, e.buf.Bytes())
	}
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
