// SPDX-License-Identifier: GPL-3.0-or-later
package otelorchestratorpoc_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
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

	logsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/grpc"
	_ "google.golang.org/grpc/encoding/gzip" // Accept the OTLP exporter's default compression.
)

// Set OTEL_POC_BINARY to .build/otel-orchestrator.plugin.
func TestOrchestratorLifecycleAndTelemetry(t *testing.T) {
	p, sink := startDistribution(t)
	const host = "otel-orchestrator:collector:hostmetrics:host"
	const logs = "otel-orchestrator:collector:filelogs:logs"
	const literal = "literal$$name-${unsupported:value}"
	p.wait(func(lines []string) bool {
		return strings.Contains(strings.Join(lines, "\n"), "otel-orchestrator:collector:hostmetrics create")
	})
	for _, kind := range []string{"hostmetrics", "filelogs"} {
		body := p.call("otel-orchestrator:collector:"+kind+" schema", "", 200)
		var schema map[string]any
		if err := json.Unmarshal([]byte(body), &schema); err != nil || schema["jsonSchema"] == nil {
			t.Fatalf("invalid schema: %s (%v)", body, err)
		}
	}
	hostConfig := `{"name":"host","collection_interval":"1s","service_name":"host-initial"}`
	p.call("otel-orchestrator:collector:hostmetrics test host", hostConfig, 200)
	p.call("otel-orchestrator:collector:hostmetrics add host", hostConfig, 202)
	p.requireConfig(host, "host-initial")
	time.Sleep(1200 * time.Millisecond)
	if len(p.workers()) != 0 {
		t.Fatal("idle plugin started a worker before ENABLE")
	}
	if len(sink.snapshot()) != 0 {
		t.Fatal("passive ADD emitted telemetry")
	}
	p.call(host+" enable", "", 202)
	p.running(host, 0)
	sink.wait(t, func(e telemetry) bool {
		return e.job == "hostmetrics:host" && e.service == "host-initial" && e.metric == "system.memory.usage"
	})
	hostPID := p.onlyNewWorker(nil)
	path := filepath.Join(t.TempDir(), "input$$.log")
	appendLine(t, path, strings.Repeat("old-prefix-", 150))
	logConfig, _ := json.Marshal(map[string]any{"name": "logs", "include": []string{path}, "service_name": literal, "start_at": "beginning"})
	p.call("otel-orchestrator:collector:filelogs add logs", string(logConfig), 202)
	p.call(logs+" enable", "", 202)
	p.running(logs, 0)
	p.requireConfig(logs, literal)
	logPID := p.onlyNewWorker(map[int]bool{hostPID: true})
	const marker = "before-restart-unique"
	appendLine(t, path, marker)
	sink.wait(t, func(e telemetry) bool { return e.job == "filelogs:logs" && e.service == literal && e.body == marker })
	p.call(host+" update", `{"name":"host","collection_interval":"0s","service_name":"rejected"}`, 422)
	p.requireConfig(host, "host-initial")
	p.requireWorkers(hostPID, logPID)
	mark := p.mark()
	p.call(host+" update", `{"name":"host","collection_interval":"1s","service_name":"host-updated"}`, 202)
	p.running(host, mark)
	sink.wait(t, func(e telemetry) bool { return e.job == "hostmetrics:host" && e.service == "host-updated" })
	newHostPID := p.onlyNewWorker(map[int]bool{hostPID: true, logPID: true})
	p.requireWorkers(newHostPID, logPID)
	requireGone(t, hostPID)
	appendLine(t, path, "after-sibling-update")
	sink.wait(t, func(e telemetry) bool { return e.job == "filelogs:logs" && e.body == "after-sibling-update" })
	// With start_at=beginning, only persisted storage prevents replay.
	p.call(logs+" disable", "", 200)
	p.requireWorkers(newHostPID)
	requireGone(t, logPID)
	mark = p.mark()
	p.call(logs+" enable", "", 202)
	p.running(logs, mark)
	newLogPID := p.onlyNewWorker(map[int]bool{newHostPID: true, logPID: true})
	p.requireWorkers(newHostPID, newLogPID)
	appendLine(t, path, "after-own-restart")
	sink.wait(t, func(e telemetry) bool { return e.job == "filelogs:logs" && e.body == "after-own-restart" })
	count := 0
	for _, e := range sink.snapshot() {
		if e.job == "filelogs:logs" && e.body == marker {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("offset persistence replayed marker: %d copies", count)
	}
	// Kill only the worker whose current ancestry belongs to this fixture.
	if !p.workers()[newHostPID] {
		t.Fatal("host worker identity changed before crash injection")
	}
	crashed, err := os.FindProcess(newHostPID)
	if err != nil {
		t.Fatal(err)
	}
	mark = p.mark()
	if err := crashed.Kill(); err != nil {
		t.Fatal(err)
	}
	p.status(host, "failed", mark)
	requireGone(t, newHostPID)
	p.requireWorkers(newLogPID)
	appendLine(t, path, "after-sibling-crash")
	sink.wait(t, func(e telemetry) bool { return e.job == "filelogs:logs" && e.body == "after-sibling-crash" })
	mark = p.mark()
	before := len(sink.snapshot())
	p.call(host+" restart", "", 200)
	p.running(host, mark)
	sink.wait(t, func(e telemetry) bool {
		return e.ordinal >= before && e.job == "hostmetrics:host" && e.service == "host-updated"
	})
	newHostPID = p.onlyNewWorker(map[int]bool{newLogPID: true})
	p.requireWorkers(newHostPID, newLogPID)
	p.call(host+" disable", "", 200)
	p.requireWorkers(newLogPID)
	requireGone(t, newHostPID)
	p.call(host+" remove", "", 200)
	p.call(host+" get", "", 404)
	p.stop(false)
	requireGone(t, newLogPID)
}

func TestOrchestratorFileDiscovery(t *testing.T) {
	p, sink := startDistribution(t, map[string]string{"hostmetrics.conf": "jobs:\n  - name: discovered\n    collection_interval: 1s\n    service_name: file-discovered\n"})
	const id = "otel-orchestrator:collector:hostmetrics:discovered"
	p.wait(func(lines []string) bool { return strings.Contains(strings.Join(lines, "\n"), id+" create") })
	p.call(id+" enable", "", 202)
	p.running(id, 0)
	p.requireConfig(id, "file-discovered")
	sink.wait(t, func(e telemetry) bool {
		return e.job == "hostmetrics:discovered" && e.service == "file-discovered" && e.metric == "system.cpu.time"
	})
	pid := p.onlyNewWorker(nil)
	p.stop(true)
	requireGone(t, pid)
}

func TestOrchestratorQuit(t *testing.T) {
	p, sink := startDistribution(t)
	p.wait(func(lines []string) bool {
		return strings.Contains(strings.Join(lines, "\n"), "otel-orchestrator:collector:hostmetrics create")
	})
	p.call("otel-orchestrator:collector:hostmetrics add quit", `{"name":"quit","collection_interval":"1s"}`, 202)
	p.call("otel-orchestrator:collector:hostmetrics:quit enable", "", 202)
	sink.wait(t, func(e telemetry) bool { return e.job == "hostmetrics:quit" && e.metric != "" })
	pid := p.onlyNewWorker(nil)
	p.stop(true)
	requireGone(t, pid)
}

func TestOrchestratorAbruptParentExit(t *testing.T) {
	p, sink := startDistribution(t)
	p.wait(func(lines []string) bool {
		return strings.Contains(strings.Join(lines, "\n"), "otel-orchestrator:collector:hostmetrics create")
	})
	p.call("otel-orchestrator:collector:hostmetrics add orphan", `{"name":"orphan","collection_interval":"1s"}`, 202)
	p.call("otel-orchestrator:collector:hostmetrics:orphan enable", "", 202)
	sink.wait(t, func(e telemetry) bool { return e.job == "hostmetrics:orphan" && e.metric != "" })
	pid := p.onlyNewWorker(nil)
	if err := p.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	requireGone(t, pid)
}

// Identify real workers through OS process ancestry, not lifecycle hooks.
func (p *distribution) workers() map[int]bool {
	p.t.Helper()
	output, err := exec.Command("ps", "-axo", "pid=,ppid=,command=").Output()
	if err != nil {
		p.t.Fatal(err)
	}
	parents := map[int]int{}
	candidates := map[int]bool{}
	for _, line := range strings.Split(string(output), "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		pid, _ := strconv.Atoi(f[0])
		parent, _ := strconv.Atoi(f[1])
		parents[pid] = parent
		if filepath.Base(f[2]) == "otel-worker" && !strings.Contains(line, " validate ") {
			candidates[pid] = true
		}
	}
	result := map[int]bool{}
	for pid := range candidates {
		for parent, steps := parents[pid], 0; parent > 1 && steps < 30; parent, steps = parents[parent], steps+1 {
			if parent == p.cmd.Process.Pid {
				result[pid] = true
				break
			}
		}
	}
	return result
}
func (p *distribution) onlyNewWorker(old map[int]bool) int {
	p.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		found := 0
		count := 0
		for pid := range p.workers() {
			if !old[pid] {
				found = pid
				count++
			}
		}
		if count == 1 {
			return found
		}
		time.Sleep(25 * time.Millisecond)
	}
	p.t.Fatalf("expected one new worker, got %v (previous %v)", p.workers(), old)
	return 0
}
func (p *distribution) requireWorkers(pids ...int) {
	p.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		got := p.workers()
		ok := len(got) == len(pids)
		for _, pid := range pids {
			ok = ok && got[pid]
		}
		if ok {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	p.t.Fatalf("workers = %v, want %v", p.workers(), pids)
}
func requireGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "pid=").Run(); err != nil {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("worker %d survived manager shutdown", pid)
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

func startDistribution(t *testing.T, configs ...map[string]string) (*distribution, *telemetrySink) {
	t.Helper()
	binary := os.Getenv("OTEL_POC_BINARY")
	if binary == "" {
		t.Skip("set OTEL_POC_BINARY to the built orchestrator executable")
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
	p.cmd = exec.Command(binary, "1")
	stateDir := filepath.Join(t.TempDir(), "state$$-${unsupported:value}")
	configDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(configDir, "otel-orchestrator"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, files := range configs {
		for name, content := range files {
			if err := os.WriteFile(filepath.Join(configDir, "otel-orchestrator", name), []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	p.cmd.Env = append(os.Environ(), "NETDATA_OTEL_POC_ENDPOINT="+listener.Addr().String(), "NETDATA_OTEL_POC_STATE_DIR="+stateDir, "NETDATA_USER_CONFIG_DIR="+configDir, "NETDATA_STOCK_CONFIG_DIR="+t.TempDir(), "NETDATA_LIB_DIR="+t.TempDir())
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
			var protocol []string
			for _, line := range p.lines {
				if strings.HasPrefix(line, "CONFIG ") || strings.HasPrefix(line, "FUNCTION_RESULT_") {
					protocol = append(protocol, line)
				}
			}
			t.Logf("protocol:\n%s\nstderr:\n%s", strings.Join(protocol, "\n"), strings.Join(p.stderr, "\n"))
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
func (p *distribution) running(id string, from int) { p.status(id, "running", from) }
func (p *distribution) status(id, want string, from int) {
	p.t.Helper()
	p.wait(func(lines []string) bool {
		for _, line := range lines[from:] {
			fields := strings.Fields(line)
			if len(fields) == 4 && fields[0] == "CONFIG" && strings.Trim(fields[1], "'\"") == id && strings.EqualFold(fields[2], "status") && strings.Trim(fields[3], "'\"") == want {
				return true
			}
		}
		return false
	})
}
func (p *distribution) requireConfig(id, service string) {
	p.t.Helper()
	body := p.call(id+" get", "", 200)
	var schema struct {
		JSONSchema struct {
			Properties map[string]any `json:"properties"`
			Required   []string       `json:"required"`
		} `json:"jsonSchema"`
	}
	if err := json.Unmarshal([]byte(p.call(id+" schema", "", 200)), &schema); err != nil {
		p.t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal([]byte(body), &fields); err != nil {
		p.t.Fatal(err)
	}
	for key := range fields {
		if _, ok := schema.JSONSchema.Properties[key]; !ok {
			p.t.Fatalf("GET contains field %q absent from schema", key)
		}
	}
	for _, key := range schema.JSONSchema.Required {
		if _, ok := fields[key]; !ok {
			p.t.Fatalf("GET lacks required schema field %q", key)
		}
	}
	var cfg struct {
		Service string `json:"service_name"`
	}
	if err := json.Unmarshal([]byte(body), &cfg); err != nil || cfg.Service != service {
		p.t.Fatalf("GET %s = %s; expected service %q (%v)", id, body, service, err)
	}
}

// The real framework treats stdin EOF as a failed input stream; QUIT is the
// graceful protocol exit. Both paths must release their worker processes.
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
		if quit && p.exitErr != nil {
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
