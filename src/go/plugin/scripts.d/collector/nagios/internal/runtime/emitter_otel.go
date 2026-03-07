// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
	collectorlogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// OTLPEmitterConfig configures the OTLP emitter.
type OTLPEmitterConfig struct {
	Endpoint   string
	Timeout    time.Duration
	UseTLS     bool
	Headers    map[string]string
	TLSConfig  tlscfg.TLSConfig
	ServerName string
}

const (
	DefaultOTLPEndpoint = "127.0.0.1:4317"
	DefaultOTLPTimeout  = 5 * time.Second
)

type otlpEmitter struct {
	client collectorlogs.LogsServiceClient
	conn   *grpc.ClientConn
	log    *logger.Logger

	timeout time.Duration
	headers metadata.MD

	resource *resourcepb.Resource
	scope    *commonpb.InstrumentationScope

	mu sync.Mutex
}

func NewOTLPEmitter(cfg OTLPEmitterConfig, log *logger.Logger) (ResultEmitter, error) {
	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = DefaultOTLPEndpoint
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultOTLPTimeout
	}

	dialOpts := []grpc.DialOption{grpc.WithBlock()}
	if cfg.UseTLS {
		tlsConf, err := tlscfg.NewTLSConfig(cfg.TLSConfig)
		if err != nil {
			return nil, err
		}
		if tlsConf == nil {
			tlsConf = &tls.Config{}
		}
		if tlsConf.MinVersion == 0 {
			tlsConf.MinVersion = tls.VersionTLS12
		}
		if tlsConf.ServerName == "" {
			if cfg.ServerName != "" {
				tlsConf.ServerName = cfg.ServerName
			} else if host, _, err := net.SplitHostPort(endpoint); err == nil {
				tlsConf.ServerName = host
			} else {
				tlsConf.ServerName = endpoint
			}
		}
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConf)))
	} else {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, endpoint, dialOpts...)
	if err != nil {
		return nil, err
	}

	md := metadata.New(nil)
	for k, v := range cfg.Headers {
		md.Set(k, v)
	}

	scope := &commonpb.InstrumentationScope{Name: "netdata/scripts", Version: "v0"}
	resource := &resourcepb.Resource{
		Attributes: []*commonpb.KeyValue{
			stringKV("service.name", "netdata-scripts-plugin"),
			stringKV("netdata.agent", "true"),
		},
	}

	return &otlpEmitter{
		client:   collectorlogs.NewLogsServiceClient(conn),
		conn:     conn,
		log:      log,
		timeout:  timeout,
		headers:  md,
		resource: resource,
		scope:    scope,
	}, nil
}

func (e *otlpEmitter) Emit(job JobRuntime, res ExecutionResult, snap JobSnapshot) {
	records := buildOTLPRecords(job, res, snap)
	if len(records) == 0 {
		return
	}

	scopeLogs := &logspb.ScopeLogs{
		Scope:      e.scope,
		LogRecords: records,
	}
	resourceLogs := &logspb.ResourceLogs{
		Resource:  e.resource,
		ScopeLogs: []*logspb.ScopeLogs{scopeLogs},
	}
	req := &collectorlogs.ExportLogsServiceRequest{
		ResourceLogs: []*logspb.ResourceLogs{resourceLogs},
	}

	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()
	if len(e.headers) > 0 {
		ctx = metadata.NewOutgoingContext(ctx, e.headers)
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if _, err := e.client.Export(ctx, req); err != nil && e.log != nil {
		e.log.Errorf("otel log export failed: %v", err)
	}
}

func (e *otlpEmitter) Close() error {
	if e.conn != nil {
		return e.conn.Close()
	}
	return nil
}

func stringKV(key, value string) *commonpb.KeyValue {
	return &commonpb.KeyValue{
		Key:   key,
		Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: value}},
	}
}

func int64KV(key string, value int64) *commonpb.KeyValue {
	return &commonpb.KeyValue{
		Key:   key,
		Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: value}},
	}
}

func buildOTLPRecords(job JobRuntime, res ExecutionResult, snap JobSnapshot) []*logspb.LogRecord {
	timestamp := snap.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now()
	}
	baseAttrs := []*commonpb.KeyValue{
		stringKV("NAGIOS_PLUGIN", job.Spec.Plugin),
		stringKV("NAGIOS_JOB", job.Spec.Name),
		stringKV("NAGIOS_STATE", snap.HardState),
		int64KV("NAGIOS_EXIT_CODE", int64(res.ExitCode)),
		int64KV("NAGIOS_DURATION_MS", int64(snap.Duration/time.Millisecond)),
	}
	if job.Spec.Vnode != "" {
		baseAttrs = append(baseAttrs, stringKV("NAGIOS_VNODE", job.Spec.Vnode))
	}
	if res.Command != "" {
		baseAttrs = append(baseAttrs, stringKV("NAGIOS_COMMAND", res.Command))
	}

	var records []*logspb.LogRecord

	execBody := fmt.Sprintf("plugin %s finished with state %s", job.Spec.Name, snap.HardState)
	records = append(records, buildLogRecord(timestamp, execBody, messageIDExecution, logspb.SeverityNumber_SEVERITY_NUMBER_INFO, baseAttrs))

	if snap.Output.StatusLine != "" {
		records = append(records, buildLogRecord(timestamp, snap.Output.StatusLine, messageIDStdout, logspb.SeverityNumber_SEVERITY_NUMBER_INFO, baseAttrs))
	}

	if snap.Output.LongOutput != "" {
		records = append(records, buildLogRecord(timestamp, snap.Output.LongOutput, messageIDLongOutput, logspb.SeverityNumber_SEVERITY_NUMBER_INFO, baseAttrs))
	}

	if res.Err != nil {
		records = append(records, buildLogRecord(timestamp, res.Err.Error(), messageIDStderr, logspb.SeverityNumber_SEVERITY_NUMBER_ERROR, baseAttrs))
	}

	if snap.PrevHardState != "" && snap.PrevHardState != snap.HardState {
		attrs := append(baseAttrs, stringKV("NAGIOS_OLD_STATE", snap.PrevHardState))
		attrs = append(attrs, stringKV("NAGIOS_NEW_STATE", snap.HardState))
		attrs = append(attrs, int64KV("NAGIOS_ATTEMPT", int64(snap.Attempts)))
		severity := severityForState(snap.HardState)
		body := fmt.Sprintf("state transition %s -> %s", snap.PrevHardState, snap.HardState)
		records = append(records, buildLogRecord(timestamp, body, messageIDStateTransition, severity, attrs))
	}

	return records
}

func buildLogRecord(ts time.Time, body string, messageID string, severity logspb.SeverityNumber, attrs []*commonpb.KeyValue) *logspb.LogRecord {
	lr := &logspb.LogRecord{
		TimeUnixNano:         uint64(ts.UnixNano()),
		ObservedTimeUnixNano: uint64(time.Now().UnixNano()),
		SeverityNumber:       severity,
		Body:                 &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: body}},
		Attributes:           append([]*commonpb.KeyValue{stringKV("MESSAGE_ID", messageID)}, attrs...),
	}
	return lr
}

func severityForState(state string) logspb.SeverityNumber {
	switch strings.ToUpper(state) {
	case "CRITICAL":
		return logspb.SeverityNumber_SEVERITY_NUMBER_ERROR
	case "WARNING":
		return logspb.SeverityNumber_SEVERITY_NUMBER_WARN
	default:
		return logspb.SeverityNumber_SEVERITY_NUMBER_INFO
	}
}

const (
	messageIDExecution       = "4fdf40816c124623a032b7fe73beacb8"
	messageIDStdout          = "ec87a56120d5431bace51e2fb8bba243"
	messageIDStderr          = "23e93dfccbf64e11aac858b9410d8a82"
	messageIDLongOutput      = "d1f59606dd4d41e3b217a0cfcae8e632"
	messageIDStateTransition = "9ce0cb58ab8b44df82c4bf1ad9ee22de"
)
