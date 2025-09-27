package jmxbridge

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"sync"
	"testing"
	"time"
)

type testLogger struct{}

func (testLogger) Debugf(string, ...any)   {}
func (testLogger) Infof(string, ...any)    {}
func (testLogger) Warningf(string, ...any) {}
func (testLogger) Errorf(string, ...any)   {}

type fakeProcess struct {
	stdinR  *io.PipeReader
	stdinW  *io.PipeWriter
	stdoutR *io.PipeReader
	stdoutW *io.PipeWriter
	stderrR *io.PipeReader
	stderrW *io.PipeWriter
	done    chan struct{}
	handler func(map[string]interface{}) Response
}

func newFakeProcess(handler func(map[string]interface{}) Response) *fakeProcess {
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	stderrR, stderrW := io.Pipe()
	return &fakeProcess{
		stdinR:  stdinR,
		stdinW:  stdinW,
		stdoutR: stdoutR,
		stdoutW: stdoutW,
		stderrR: stderrR,
		stderrW: stderrW,
		done:    make(chan struct{}),
		handler: handler,
	}
}

func (p *fakeProcess) Start() error {
	go p.run()
	return nil
}

func (p *fakeProcess) run() {
	defer close(p.done)
	defer p.stdoutW.Close()
	defer p.stderrW.Close()
	scanner := bufio.NewScanner(p.stdinR)
	for scanner.Scan() {
		line := scanner.Text()
		var cmd map[string]interface{}
		if err := json.Unmarshal([]byte(line), &cmd); err != nil {
			continue
		}
		resp := p.handler(cmd)
		payload, _ := json.Marshal(resp)
		_, _ = p.stdoutW.Write(append(payload, '\n'))
	}
}

func (p *fakeProcess) Stdin() io.WriteCloser { return p.stdinW }
func (p *fakeProcess) Stdout() io.ReadCloser { return p.stdoutR }
func (p *fakeProcess) Stderr() io.ReadCloser { return p.stderrR }
func (p *fakeProcess) Wait() error           { <-p.done; return nil }

func TestClientStartAndSend(t *testing.T) {
	var call int
	handler := func(cmd map[string]interface{}) Response {
		call++
		if call == 1 {
			return Response{Status: "OK"}
		}
		return Response{Status: "OK", Data: map[string]interface{}{"value": 123}}
	}

	var procMu sync.Mutex
	var proc *fakeProcess

	factory := func(ctx context.Context, javaPath, jarPath string) (process, error) {
		procMu.Lock()
		defer procMu.Unlock()
		proc = newFakeProcess(handler)
		return proc, nil
	}

	client, err := NewClient(Config{JarPath: "helper.jar"}, testLogger{}, WithProcessFactory(factory))
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	if err := client.Start(context.Background(), Command{"command": "INIT"}); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	resp, err := client.Send(context.Background(), Command{"command": "SCRAPE"})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if value := resp.Data["value"]; value != float64(123) {
		t.Fatalf("unexpected response data: %#v", resp.Data)
	}

	client.Shutdown()
}

func TestClientErrorStatus(t *testing.T) {
	var call int
	handler := func(cmd map[string]interface{}) Response {
		call++
		if call == 1 {
			return Response{Status: "OK"}
		}
		return Response{Status: "ERROR", Message: "boom"}
	}

	factory := func(ctx context.Context, javaPath, jarPath string) (process, error) {
		return newFakeProcess(handler), nil
	}

	client, err := NewClient(Config{JarPath: "helper.jar"}, testLogger{}, WithProcessFactory(factory))
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	if err := client.Start(context.Background(), Command{"command": "INIT"}); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if _, err := client.Send(context.Background(), Command{"command": "SCRAPE"}); err == nil {
		t.Fatalf("expected error status failure")
	}

	client.Shutdown()
}

func TestClientCancellation(t *testing.T) {
	handler := func(cmd map[string]interface{}) Response {
		time.Sleep(200 * time.Millisecond)
		return Response{Status: "OK"}
	}

	factory := func(ctx context.Context, javaPath, jarPath string) (process, error) {
		return newFakeProcess(handler), nil
	}

	client, err := NewClient(Config{JarPath: "helper.jar"}, testLogger{}, WithProcessFactory(factory))
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	if err := client.Start(context.Background(), Command{"command": "INIT"}); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if _, err := client.Send(ctx, Command{"command": "SCRAPE"}); err == nil {
		t.Fatalf("expected cancellation error")
	}

	client.Shutdown()
}

func TestClientWritesJar(t *testing.T) {
	jarData := []byte("fake jar contents")
	client, err := NewClient(Config{JarData: jarData, JarFileName: "helper.jar"}, testLogger{}, WithProcessFactory(func(ctx context.Context, javaPath, jarPath string) (process, error) {
		if _, err := os.Stat(jarPath); err != nil {
			t.Fatalf("jar file not written: %v", err)
		}
		return newFakeProcess(func(cmd map[string]interface{}) Response { return Response{Status: "OK"} }), nil
	}))
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	if err := client.Start(context.Background(), Command{"command": "INIT"}); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	path := client.jarPath
	client.Shutdown()
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("jar file should be removed on shutdown")
	}
}
