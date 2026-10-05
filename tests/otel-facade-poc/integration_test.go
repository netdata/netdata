// SPDX-License-Identifier: GPL-3.0-or-later
package otelfacadepoc_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	logsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/grpc"
	_ "google.golang.org/grpc/encoding/gzip" // Accept the OTLP exporter's default compression.
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Set OTEL_POC_BINARY to the generated distribution to exercise actual receivers.
func TestDistributionDynCfgAndTelemetry(t *testing.T) {
	p, sink := startDistribution(t)
	const host = "otel-poc:hostmetrics:host"
	const logs = "otel-poc:filelogs:logs"
	for _, kind := range []string{"hostmetrics", "filelogs"} {
		body := p.call("otel-poc:"+kind+" schema", "", 200)
		var schema struct {
			JSONSchema struct {
				Properties map[string]any `json:"properties"`
			} `json:"jsonSchema"`
		}
		if err := json.Unmarshal([]byte(body), &schema); err != nil || len(schema.JSONSchema.Properties) == 0 {
			t.Fatalf("invalid schema: %s (%v)", body, err)
		}
	}
	hostConfig := `{"name":"host","interval":"1s","service_name":"host-initial"}`
	p.call("otel-poc:hostmetrics test", hostConfig, 200)
	p.call("otel-poc:hostmetrics add host", hostConfig, 202)
	p.requireConfig(host, "host-initial")
	time.Sleep(1200 * time.Millisecond) // Observe idle behavior across a collection interval.
	if got := sink.snapshot(); len(got) != 0 {
		t.Fatalf("passive ADD emitted telemetry: %+v", got)
	}
	mark := p.mark()
	p.call(host+" enable", "", 202)
	p.running(host, mark)
	sink.wait(t, func(e telemetry) bool { return e.job == host && e.service == "host-initial" && e.metric != "" })
	logPath := filepath.Join(t.TempDir(), "input.log")
	appendLine(t, logPath, strings.Repeat("old-prefix-", 150)) // Stable fingerprint before discovery.
	logConfig, _ := json.Marshal(map[string]any{"name": "logs", "paths": []string{logPath}, "service_name": "logs"})
	p.call("otel-poc:filelogs add logs", string(logConfig), 202)
	mark = p.mark()
	p.call(logs+" enable", "", 202)
	p.running(logs, mark)
	// Readiness precedes discovery. An observed probe establishes an actual offset.
	for i := 0; i < 40; i++ {
		probe := fmt.Sprintf("probe-%d", i)
		appendLine(t, logPath, probe)
		if sink.waitFor(300*time.Millisecond, func(e telemetry) bool { return e.job == logs && e.body == probe }) {
			break
		}
		if i == 39 {
			t.Fatal("file receiver never emitted an appended probe")
		}
	}
	marker := "before-reload-unique"
	appendLine(t, logPath, marker)
	sink.wait(t, func(e telemetry) bool { return e.job == logs && e.service == "logs" && e.body == marker })
	before := len(sink.snapshot())
	p.call(host+" update", `{"interval":"0s","service_name":"rejected"}`, 400)
	p.requireConfig(host, "host-initial")
	sink.wait(t, func(e telemetry) bool { return e.ordinal >= before && e.job == host && e.service == "host-initial" })
	mark = p.mark()
	p.call(host+" restart", "", 202)
	p.running(host, mark)
	p.running(logs, mark)
	appendLine(t, logPath, "after-reload-unique")
	sink.wait(t, func(e telemetry) bool { return e.job == logs && e.body == "after-reload-unique" })
	count := 0
	for _, e := range sink.snapshot() {
		if e.job == logs && e.body == marker {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("persisted file offset replayed old line: got %d copies", count)
	}
	// Queue mutations without waiting for replies, including changes during reload.
	mark = p.mark()
	var ids []string
	for i := 0; i < 12; i++ {
		ids = append(ids, p.send(host+" update", fmt.Sprintf(`{"interval":"1s","service_name":"burst-%d"}`, i)))
	}
	for _, uid := range ids {
		p.response(uid, 202)
	}
	p.requireConfig(host, "burst-11")
	sink.wait(t, func(e telemetry) bool { return e.job == host && e.service == "burst-11" })
	p.running(host, mark)
	p.call(host+" disable", "", 200)
	p.call(host+" update", `{"interval":"1s","service_name":"disabled-edit"}`, 200)
	p.requireConfig(host, "disabled-edit")
	p.call(host+" remove", "", 200)
	p.call(host+" get", "", 404)
	p.call(logs+" disable", "", 200)
	p.call(logs+" remove", "", 200)
	p.call(logs+" get", "", 404)
	p.stop(false)
}
func TestDistributionQuit(t *testing.T) {
	p, _ := startDistribution(t)
	p.call("otel-poc:hostmetrics schema", "", 200)
	p.stop(true)
}

func TestDistributionLiteralConfig(t *testing.T) {
	p, sink := startDistribution(t)
	const service = "literal$$name-${unsupported:value}"
	const host = "otel-poc:hostmetrics:literal"
	config, _ := json.Marshal(map[string]any{"interval": "1s", "service_name": service})
	p.call("otel-poc:hostmetrics add literal", string(config), 202)
	p.call(host+" enable", "", 202)
	p.running(host, 0)
	sink.wait(t, func(e telemetry) bool { return e.job == host && e.service == service })
	p.requireConfig(host, service)

	const logs = "otel-poc:filelogs:literal"
	// Braces have their own glob meaning in file_log. Use plain dollar signs
	// here to isolate Collector expansion from the receiver's glob semantics.
	path := filepath.Join(t.TempDir(), "input$$.log")
	appendLine(t, path, strings.Repeat("old-prefix-", 150))
	config, _ = json.Marshal(map[string]any{"paths": []string{path}, "service_name": service})
	p.call("otel-poc:filelogs add literal", string(config), 202)
	p.call(logs+" enable", "", 202)
	p.running(logs, 0)
	for i := 0; i < 40; i++ {
		appendLine(t, path, "literal-path-probe")
		if sink.waitFor(300*time.Millisecond, func(e telemetry) bool {
			return e.job == logs && e.service == service && e.body == "literal-path-probe"
		}) {
			p.stop(false)
			return
		}
	}
	t.Fatal("literal file path did not emit telemetry")
}

type telemetry struct {
	job, service, metric, body string
	ordinal                    int
}
type telemetrySink struct {
	mu     sync.Mutex
	events []telemetry
}

func (s *telemetrySink) add(r *resourcev1.Resource, metric, body string) {
	e := telemetry{metric: metric, body: body}
	for _, a := range r.GetAttributes() {
		switch a.Key {
		case "netdata.poc.job":
			e.job = a.Value.GetStringValue()
		case "service.name":
			e.service = a.Value.GetStringValue()
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e.ordinal = len(s.events)
	s.events = append(s.events, e)
}
func (s *telemetrySink) snapshot() []telemetry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]telemetry(nil), s.events...)
}
func (s *telemetrySink) waitFor(d time.Duration, pred func(telemetry) bool) bool {
	deadline := time.Now().Add(d)
	for {
		for _, e := range s.snapshot() {
			if pred(e) {
				return true
			}
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
}
func (s *telemetrySink) wait(t *testing.T, pred func(telemetry) bool) {
	t.Helper()
	if !s.waitFor(20*time.Second, pred) {
		t.Fatalf("timed out waiting for telemetry; received %+v", s.snapshot())
	}
}

type metricServer struct {
	metricsv1.UnimplementedMetricsServiceServer
	sink *telemetrySink
}

func (s *metricServer) Export(_ context.Context, r *metricsv1.ExportMetricsServiceRequest) (*metricsv1.ExportMetricsServiceResponse, error) {
	for _, rm := range r.ResourceMetrics {
		for _, sm := range rm.ScopeMetrics {
			for _, m := range sm.Metrics {
				s.sink.add(rm.Resource, m.Name, "")
			}
		}
	}
	return &metricsv1.ExportMetricsServiceResponse{}, nil
}

type logServer struct {
	logsv1.UnimplementedLogsServiceServer
	sink *telemetrySink
}

func (s *logServer) Export(_ context.Context, r *logsv1.ExportLogsServiceRequest) (*logsv1.ExportLogsServiceResponse, error) {
	for _, rl := range r.ResourceLogs {
		for _, sl := range rl.ScopeLogs {
			for _, l := range sl.LogRecords {
				s.sink.add(rl.Resource, "", l.Body.GetStringValue())
			}
		}
	}
	return &logsv1.ExportLogsServiceResponse{}, nil
}

type distribution struct {
	t             *testing.T
	cmd           *exec.Cmd
	stdin         io.WriteCloser
	done          chan struct{}
	exitErr       error
	mu            sync.Mutex
	lines, stderr []string
	next          int
}

func startDistribution(t *testing.T) (*distribution, *telemetrySink) {
	t.Helper()
	binary := os.Getenv("OTEL_POC_BINARY")
	if binary == "" {
		t.Skip("set OTEL_POC_BINARY to the generated OCB executable")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	sink := &telemetrySink{}
	server := grpc.NewServer()
	metricsv1.RegisterMetricsServiceServer(server, &metricServer{sink: sink})
	logsv1.RegisterLogsServiceServer(server, &logServer{sink: sink})
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	p := &distribution{t: t, done: make(chan struct{})}
	p.cmd = exec.Command(stageLauncher(t, binary), "1")
	stateDir := filepath.Join(t.TempDir(), "state$$-${unsupported:value}")
	p.cmd.Env = append(os.Environ(), "NETDATA_OTEL_POC_ENDPOINT="+listener.Addr().String(), "NETDATA_OTEL_POC_STATE_DIR="+stateDir)
	p.stdin, err = p.cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := p.cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := p.cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = p.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var readers sync.WaitGroup
	readers.Add(2)
	go func() { defer readers.Done(); p.scan(stdout, false) }()
	go func() { defer readers.Done(); p.scan(stderr, true) }()
	go func() { readers.Wait(); p.exitErr = p.cmd.Wait(); close(p.done) }()
	t.Cleanup(func() {
		_ = p.stdin.Close()
		select {
		case <-p.done:
		case <-time.After(5 * time.Second):
			_ = p.cmd.Process.Kill()
			<-p.done
		}
		if t.Failed() {
			p.mu.Lock()
			defer p.mu.Unlock()
			t.Logf("stdout:\n%s\nstderr:\n%s", strings.Join(p.lines, "\n"), strings.Join(p.stderr, "\n"))
		}
	})
	return p, sink
}
func (p *distribution) scan(r io.Reader, stderr bool) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		p.mu.Lock()
		if stderr {
			p.stderr = append(p.stderr, scanner.Text())
		} else {
			p.lines = append(p.lines, scanner.Text())
		}
		p.mu.Unlock()
	}
}
func (p *distribution) mark() int { p.mu.Lock(); defer p.mu.Unlock(); return len(p.lines) }
func (p *distribution) send(command, payload string) string {
	p.t.Helper()
	p.next++
	uid := fmt.Sprintf("request-%d", p.next)
	frame := fmt.Sprintf("FUNCTION %s 30 %q \"0xffff\" \"poc-test\"\n", uid, "config "+command)
	if payload != "" {
		frame = fmt.Sprintf("FUNCTION_PAYLOAD %s 30 %q \"0xffff\" \"poc-test\" \"application/json\"\n%s\nFUNCTION_PAYLOAD_END\n", uid, "config "+command, payload)
	}
	if _, err := io.WriteString(p.stdin, frame); err != nil {
		p.t.Fatal(err)
	}
	return uid
}
func (p *distribution) wait(pred func([]string) bool) {
	p.t.Helper()
	deadline := time.Now().Add(25 * time.Second)
	for {
		p.mu.Lock()
		ok := pred(p.lines)
		p.mu.Unlock()
		if ok {
			return
		}
		select {
		case <-p.done:
			p.t.Fatalf("Collector exited while waiting for protocol output: %v", p.exitErr)
		default:
		}
		if time.Now().After(deadline) {
			p.t.Fatal("timed out waiting for protocol output")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
func (p *distribution) response(uid string, code int) string {
	p.t.Helper()
	var body string
	var actual int
	p.wait(func(lines []string) bool {
		for i, line := range lines {
			fields := strings.Fields(line)
			if len(fields) < 3 || fields[0] != "FUNCTION_RESULT_BEGIN" || strings.Trim(fields[1], "'\"") != uid {
				continue
			}
			for j := i + 1; j < len(lines); j++ {
				if lines[j] == "FUNCTION_RESULT_END" {
					actual, _ = strconv.Atoi(fields[2])
					body = strings.Join(lines[i+1:j], "\n")
					return true
				}
			}
		}
		return false
	})
	if actual != code {
		p.t.Fatalf("request %s: expected %d, got %d: %s", uid, code, actual, body)
	}
	return body
}
func (p *distribution) call(command, payload string, code int) string {
	p.t.Helper()
	return p.response(p.send(command, payload), code)
}
func (p *distribution) running(id string, from int) {
	p.t.Helper()
	p.wait(func(lines []string) bool {
		for _, line := range lines[from:] {
			fields := strings.Fields(line)
			if len(fields) == 4 && fields[0] == "CONFIG" && strings.Trim(fields[1], "'\"") == id && strings.EqualFold(fields[2], "status") && strings.Trim(fields[3], "'\"") == "running" {
				return true
			}
		}
		return false
	})
}
func (p *distribution) requireConfig(id, service string) {
	p.t.Helper()
	body := p.call(id+" get", "", 200)
	var cfg struct {
		Service string `json:"service_name"`
	}
	if err := json.Unmarshal([]byte(body), &cfg); err != nil || cfg.Service != service {
		p.t.Fatalf("GET %s = %s; expected service %q (%v)", id, body, service, err)
	}
}
func (p *distribution) stop(quit bool) {
	p.t.Helper()
	if quit {
		if _, err := io.WriteString(p.stdin, "QUIT\n"); err != nil {
			p.t.Fatal(err)
		}
	} else {
		_ = p.stdin.Close()
	}
	select {
	case <-p.done:
		if p.exitErr != nil {
			p.t.Fatalf("Collector shutdown failed: %v", p.exitErr)
		}
	case <-time.After(15 * time.Second):
		p.t.Fatal("Collector did not stop gracefully")
	}
}
func appendLine(t *testing.T, path, line string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fmt.Fprintln(f, line); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
}
