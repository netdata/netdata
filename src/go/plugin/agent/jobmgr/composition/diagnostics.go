// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"io"
	"log/slog"
	"sync/atomic"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
)

type diagnosticLogger struct {
	logger   *logger.Logger
	sequence atomic.Uint64
}

func newDiagnosticLogger(writer io.Writer) *diagnosticLogger {
	return &diagnosticLogger{
		logger: logger.NewWithWriter(writer).With(slog.String("component", "job manager")),
	}
}

func newProcessDiagnosticLogger() *diagnosticLogger {
	return newDiagnosticLogger(nil)
}

func (dl *diagnosticLogger) ObserveDiagnostic(event jobmgr.DiagnosticEvent) {
	if dl == nil || event.Name == "" {
		return
	}
	sequence := dl.sequence.Add(1)
	attributes := []any{
		slog.Uint64("event_sequence", sequence),
	}
	if event.Generation != 0 {
		attributes = append(attributes, slog.Uint64("run_generation", event.Generation))
	}
	if event.Resource != "" {
		attributes = append(attributes, slog.String("resource", event.Resource))
	}
	if event.Command != "" {
		attributes = append(attributes, slog.String("command", event.Command))
	}
	if event.State != "" {
		attributes = append(attributes, slog.String("state", event.State))
	}
	if event.Task.Valid() {
		attributes = append(
			attributes,
			slog.Uint64("task_generation", event.Task.Generation),
			slog.Uint64("task_slot", uint64(event.Task.Slot)),
		)
	}
	if event.Sequence != 0 {
		attributes = append(attributes, slog.Int("phase_sequence", int(event.Sequence)))
	}
	if event.Count != 0 {
		attributes = append(attributes, slog.Int("count", event.Count))
	}
	if event.ResultStatus != 0 {
		attributes = append(attributes, slog.Int("result_status", event.ResultStatus))
	}
	if event.Err != nil {
		attributes = append(attributes, slog.String("error", event.Err.Error()))
	}
	log := dl.logger.With(attributes...)
	switch event.Level {
	case jobmgr.DiagnosticInfo:
		log.Info(event.Name)
	case jobmgr.DiagnosticWarning:
		log.Warning(event.Name)
	case jobmgr.DiagnosticError:
		log.Error(event.Name)
	}
}
