package main

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	logapi "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
)

// The OTel plugin partitions log streams by (service.namespace, service.name).
const (
	otlpServiceNamespace = "netdata"
	otlpServiceName      = "agent-events"
)

// otlpEmitter exports accepted events as OTel log records over OTLP/gRPC.
type otlpEmitter struct {
	provider *sdklog.LoggerProvider
	logger   logapi.Logger
	endpoint string
}

func newOTLPEmitter(ctx context.Context, endpoint string) (*otlpEmitter, error) {
	// Insecure transport: the target is the local OTel plugin listener, which
	// defaults to plaintext gRPC on localhost.
	exporter, err := otlploggrpc.New(ctx,
		otlploggrpc.WithEndpoint(endpoint),
		otlploggrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP log exporter: %w", err)
	}
	res := resource.NewSchemaless(
		attribute.String("service.namespace", otlpServiceNamespace),
		attribute.String("service.name", otlpServiceName),
	)
	provider := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)),
		sdklog.WithResource(res),
	)
	return &otlpEmitter{
		provider: provider,
		logger:   provider.Logger(otlpServiceName),
		endpoint: endpoint,
	}, nil
}

// emit queues one event for asynchronous export; it never blocks the request.
func (e *otlpEmitter) emit(ctx context.Context, event map[string]interface{}, payload []byte) {
	e.logger.Emit(ctx, buildRecord(time.Now(), event, payload))
}

func (e *otlpEmitter) shutdown(ctx context.Context) error {
	return e.provider.Shutdown(ctx)
}

// buildRecord is the single mapping seam between the event document and the
// OTel log record. The whole enriched document travels as a JSON string body:
// the ingestion side explodes it into typed body.* fields, keeping this server
// agnostic to status-file schema changes. Fields promoted to explicit OTLP
// attributes in the future must be added here, with the remainder staying in
// the body.
func buildRecord(ts time.Time, event map[string]interface{}, payload []byte) logapi.Record {
	var rec logapi.Record
	rec.SetTimestamp(ts)
	rec.SetObservedTimestamp(ts)
	rec.SetBody(logapi.StringValue(string(payload)))
	if sev, text, ok := severityFromPriority(event["priority"]); ok {
		rec.SetSeverity(sev)
		rec.SetSeverityText(text)
	}
	return rec
}

// severityFromPriority maps the event's syslog priority to an OTel severity
// per the OTel Logs Data Model severity mappings. An absent or malformed
// priority leaves the severity unspecified rather than guessing.
func severityFromPriority(v interface{}) (logapi.Severity, string, bool) {
	f, ok := v.(float64) // encoding/json decodes JSON numbers as float64
	if !ok || f != float64(int(f)) {
		return logapi.SeverityUndefined, "", false
	}
	switch int(f) {
	case 0:
		return logapi.SeverityFatal, "emergency", true
	case 1:
		return logapi.SeverityError3, "alert", true
	case 2:
		return logapi.SeverityError2, "critical", true
	case 3:
		return logapi.SeverityError, "error", true
	case 4:
		return logapi.SeverityWarn, "warning", true
	case 5:
		return logapi.SeverityInfo2, "notice", true
	case 6:
		return logapi.SeverityInfo, "info", true
	case 7:
		return logapi.SeverityDebug, "debug", true
	default:
		return logapi.SeverityUndefined, "", false
	}
}
