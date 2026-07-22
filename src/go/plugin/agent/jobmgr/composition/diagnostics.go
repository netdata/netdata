// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"fmt"
	"io"
	"log/slog"
	"sync/atomic"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

const jobManagerTraceEnabledValue = "enabled"

// jobManagerTrace is set only by plugin/go.d/hack/go-build.sh. Production
// build paths leave it empty.
var jobManagerTrace string

func jobManagerTraceEnabled() bool {
	return jobManagerTrace == jobManagerTraceEnabledValue
}

type diagnosticLogger struct {
	logger   *logger.Logger
	sequence atomic.Uint64
	trace    bool
}

func newDiagnosticLogger(writer io.Writer, trace bool) *diagnosticLogger {
	return &diagnosticLogger{
		logger: logger.NewWithWriter(writer).With(slog.String("component", "job manager")),
		trace:  trace,
	}
}

func newProcessDiagnosticLogger() *diagnosticLogger {
	trace := jobManagerTraceEnabled()
	if trace {
		logger.Level.Set(slog.LevelDebug)
	}
	return newDiagnosticLogger(nil, trace)
}

func (dl *diagnosticLogger) TraceEnabled() bool {
	return dl != nil && dl.trace
}

func (dl *diagnosticLogger) ObserveDiagnostic(event jobmgr.DiagnosticEvent) {
	if dl == nil || event.Name == "" || event.Level == jobmgr.DiagnosticTrace && !dl.trace {
		return
	}
	sequence := dl.sequence.Add(1)
	attributes := []any{
		slog.Uint64("event_sequence", sequence),
	}
	if event.Level == jobmgr.DiagnosticTrace {
		attributes = append(attributes, slog.Bool("trace", true))
	}
	if event.Generation != 0 {
		attributes = append(attributes, slog.Uint64("run_generation", event.Generation))
	}
	if event.Operation != 0 {
		attributes = append(attributes, slog.Uint64("operation", uint64(event.Operation)))
	}
	if event.UID != "" {
		attributes = append(attributes, slog.String("uid", event.UID))
	}
	if event.Source.Valid() {
		attributes = append(attributes, slog.String("source", diagnosticSource(event.Source)))
	}
	if event.Route != "" {
		attributes = append(attributes, slog.String("route", event.Route))
	}
	if event.Lane != "" {
		attributes = append(attributes, slog.String("lane", event.Lane))
	}
	if event.Resource != "" {
		attributes = append(attributes, slog.String("resource", event.Resource))
	}
	if event.ResourceGeneration != 0 {
		attributes = append(attributes, slog.Uint64("resource_generation", event.ResourceGeneration))
	}
	if event.Claim != "" {
		attributes = append(attributes, slog.String("claim", event.Claim))
	}
	if event.Command != "" {
		attributes = append(attributes, slog.String("command", event.Command))
	}
	if event.Phase != "" {
		attributes = append(attributes, slog.String("phase", event.Phase))
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
	if event.TaskRequest.Valid() {
		attributes = append(
			attributes,
			slog.Uint64("task_request_generation", uint64(event.TaskRequest.Generation)),
			slog.Uint64("task_request_slot", uint64(event.TaskRequest.Slot)),
		)
	}
	if event.Action != 0 {
		attributes = append(attributes, slog.String("action", diagnosticAction(event.Action)))
	}
	if event.Disposition != 0 {
		attributes = append(attributes, slog.String("disposition", diagnosticDisposition(event.Disposition)))
	}
	if event.Control != 0 {
		attributes = append(attributes, slog.Int("status", int(event.Control)))
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
	if event.Rollback {
		attributes = append(attributes, slog.Bool("rollback", true))
	}
	if event.Composite {
		attributes = append(attributes, slog.Bool("composite", true))
	}
	if event.Err != nil {
		attributes = append(attributes, slog.String("error", event.Err.Error()))
	}
	log := dl.logger.With(attributes...)
	switch event.Level {
	case jobmgr.DiagnosticTrace:
		log.Debug(event.Name)
	case jobmgr.DiagnosticInfo:
		log.Info(event.Name)
	case jobmgr.DiagnosticWarning:
		log.Warning(event.Name)
	case jobmgr.DiagnosticError:
		log.Error(event.Name)
	}
}

func diagnosticSource(source lifecycle.Source) string {
	switch source {
	case lifecycle.SourceJobManager:
		return "job_manager"
	case lifecycle.SourceFunction:
		return "function"
	default:
		return fmt.Sprintf("unknown(%d)", source)
	}
}

func diagnosticAction(action lifecycle.TaskActionKind) string {
	switch action {
	case lifecycle.TaskActionEncodeWrite:
		return "encode_write"
	case lifecycle.TaskActionStopResource:
		return "stop_resource"
	case lifecycle.TaskActionFinalizeResource:
		return "finalize_resource"
	case lifecycle.TaskActionApplyResourceTransaction:
		return "apply_resource_transaction"
	case lifecycle.TaskActionDispose:
		return "dispose"
	case lifecycle.TaskActionCleanup:
		return "cleanup"
	case lifecycle.TaskActionTerminate:
		return "terminate"
	default:
		return fmt.Sprintf("unknown(%d)", action)
	}
}

func diagnosticDisposition(disposition lifecycle.ResourceTransactionDisposition) string {
	switch disposition {
	case lifecycle.ResourceTransactionUnchanged:
		return "unchanged"
	case lifecycle.ResourceTransactionInstalled:
		return "installed"
	case lifecycle.ResourceTransactionRemoved:
		return "removed"
	case lifecycle.ResourceTransactionReplaced:
		return "replaced"
	default:
		return fmt.Sprintf("unknown(%d)", disposition)
	}
}
