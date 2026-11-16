// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"io"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	functionadapter "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/functions"
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
	if event.Age > 0 {
		attributes = append(attributes, slog.Duration("age", event.Age))
	}
	if event.Err != nil {
		attributes = append(attributes, slog.String("error", event.Err.Error()))
	}
	log := dl.logger.With(attributes...)
	switch event.Name {
	case functionadapter.DiagnosticAvailabilityCallbackFailed,
		functionadapter.DiagnosticAvailabilityAttemptFailed,
		functionadapter.DiagnosticAvailabilityReconciliationFailed:
		// Poll failures can recur every scheduler tick across many bundles. Use
		// fixed category keys shared across jobs and run generations.
		limited := log.Limit(event.Name, 1, time.Hour)
		switch event.Level {
		case jobmgr.DiagnosticWarning:
			limited.Warning(event.Name)
		case jobmgr.DiagnosticError:
			limited.Error(event.Name)
		}
		return
	}
	// The jobmgr.ObserveDiagnostic production gateway rejects invalid levels before dispatch.
	switch event.Level {
	case jobmgr.DiagnosticInfo:
		log.Info(event.Name)
	case jobmgr.DiagnosticWarning:
		log.Warning(event.Name)
	case jobmgr.DiagnosticError:
		log.Error(event.Name)
	}
}
