//go:build cgo
// +build cgo

package jmx

import (
	"context"
	"errors"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/protocols/jmxbridge"
)

type nopLogger struct{}

func (nopLogger) Debugf(string, ...any)   {}
func (nopLogger) Infof(string, ...any)    {}
func (nopLogger) Warningf(string, ...any) {}
func (nopLogger) Errorf(string, ...any)   {}

type fakeBridge struct {
	started   bool
	responses map[string]*jmxbridge.Response
}

func (f *fakeBridge) Start(context.Context, jmxbridge.Command) error {
	f.started = true
	return nil
}

func (f *fakeBridge) Send(ctx context.Context, cmd jmxbridge.Command) (*jmxbridge.Response, error) {
	if !f.started {
		return nil, errors.New("bridge not started")
	}
	target, _ := cmd["target"].(string)
	if target == "" {
		return nil, errors.New("missing target")
	}
	resp := f.responses[target]
	if resp == nil {
		return &jmxbridge.Response{Status: "OK", Data: map[string]interface{}{}}, nil
	}
	return resp, nil
}

func (f *fakeBridge) Shutdown() {
	f.started = false
}

func TestClientFetchJVM(t *testing.T) {
	bridge := &fakeBridge{
		responses: map[string]*jmxbridge.Response{
			"JVM": {
				Status: "OK",
				Data: map[string]interface{}{
					"heap": map[string]interface{}{
						"used":      512.0,
						"committed": 1024.0,
						"max":       2048.0,
					},
					"nonheap": map[string]interface{}{
						"used":      128.0,
						"committed": 256.0,
					},
					"gc": map[string]interface{}{
						"count": 12.0,
						"time":  345.0,
					},
					"threads": map[string]interface{}{
						"count":        44.0,
						"daemon":       30.0,
						"peak":         60.0,
						"totalStarted": 100.0,
					},
					"classes": map[string]interface{}{
						"loaded":   5000.0,
						"unloaded": 200.0,
					},
					"cpu": map[string]interface{}{
						"processCpuUsage": 0.42,
					},
					"uptime": 900.0,
				},
			},
		},
	}

	client, err := NewClient(Config{JMXURL: "service:jmx:iiop://localhost:2809"}, nopLogger{}, WithBridge(bridge))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	stats, err := client.FetchJVM(context.Background())
	if err != nil {
		t.Fatalf("fetch JVM failed: %v", err)
	}

	if stats.Heap.Used != 512.0 || stats.Heap.Max != 2048.0 {
		t.Fatalf("unexpected heap stats: %+v", stats.Heap)
	}
	if stats.Threads.Count != 44.0 || stats.Threads.Peak != 60.0 {
		t.Fatalf("unexpected thread stats: %+v", stats.Threads)
	}
	if stats.Classes.Loaded != 5000.0 || stats.CPU.ProcessUsage != 0.42 {
		t.Fatalf("unexpected stats: classes=%+v cpu=%+v", stats.Classes, stats.CPU)
	}
	if stats.Uptime != 900.0 {
		t.Fatalf("unexpected uptime: %.2f", stats.Uptime)
	}
}

func TestClientFetchThreadPools(t *testing.T) {
	bridge := &fakeBridge{
		responses: map[string]*jmxbridge.Response{
			"THREADPOOLS": {
				Status: "OK",
				Data: map[string]interface{}{
					"threadPools": []interface{}{
						map[string]interface{}{
							"name":            "Default",
							"poolSize":        50.0,
							"activeCount":     5.0,
							"maximumPoolSize": 75.0,
						},
						map[string]interface{}{
							"name":            "WebContainer",
							"poolSize":        80.0,
							"activeCount":     12.0,
							"maximumPoolSize": 100.0,
						},
					},
				},
			},
		},
	}

	client, err := NewClient(Config{JMXURL: "service:jmx:iiop://localhost:2809"}, nopLogger{}, WithBridge(bridge))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	pools, err := client.FetchThreadPools(context.Background(), 10)
	if err != nil {
		t.Fatalf("fetch thread pools failed: %v", err)
	}

	if len(pools) != 2 {
		t.Fatalf("expected 2 pools, got %d", len(pools))
	}
	if pools[0].Name != "Default" || pools[0].ActiveCount != 5.0 {
		t.Fatalf("unexpected pool[0]: %+v", pools[0])
	}
	if pools[1].PoolSize != 80.0 || pools[1].MaximumPoolSize != 100.0 {
		t.Fatalf("unexpected pool[1]: %+v", pools[1])
	}
}

func TestClientFetchJDBCPools(t *testing.T) {
	bridge := &fakeBridge{
		responses: map[string]*jmxbridge.Response{
			"JDBC": {
				Status: "OK",
				Data: map[string]interface{}{
					"jdbcPools": []interface{}{
						map[string]interface{}{
							"name":                    "DefaultDS",
							"poolSize":                40.0,
							"numConnectionsUsed":      5.0,
							"numConnectionsFree":      10.0,
							"avgWaitTime":             1.5,
							"avgInUseTime":            3.2,
							"numConnectionsCreated":   120.0,
							"numConnectionsDestroyed": 80.0,
							"waitingThreadCount":      2.0,
						},
					},
				},
			},
		},
	}

	client, err := NewClient(Config{JMXURL: "service:jmx:iiop://localhost:2809"}, nopLogger{}, WithBridge(bridge))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	pools, err := client.FetchJDBCPools(context.Background(), 5)
	if err != nil {
		t.Fatalf("fetch jdbc pools failed: %v", err)
	}

	if len(pools) != 1 {
		t.Fatalf("expected 1 pool, got %d", len(pools))
	}
	p := pools[0]
	if p.Name != "DefaultDS" || p.PoolSize != 40 || p.NumConnectionsUsed != 5 {
		t.Fatalf("unexpected pool contents: %+v", p)
	}
	if p.AvgWaitTime != 1.5 || p.AvgInUseTime != 3.2 {
		t.Fatalf("unexpected timing values: %+v", p)
	}
}

func TestClientFetchJMSDestinations(t *testing.T) {
	bridge := &fakeBridge{
		responses: map[string]*jmxbridge.Response{
			"JMS": {
				Status: "OK",
				Data: map[string]interface{}{
					"jmsDestinations": []interface{}{
						map[string]interface{}{
							"name":                 "Queue1",
							"type":                 "queue",
							"messagesCurrentCount": 12.0,
							"messagesPendingCount": 3.0,
							"messagesAddedCount":   40.0,
							"consumerCount":        5.0,
						},
					},
				},
			},
		},
	}

	client, err := NewClient(Config{JMXURL: "service:jmx:iiop://localhost:2809"}, nopLogger{}, WithBridge(bridge))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	dests, err := client.FetchJMSDestinations(context.Background(), 5)
	if err != nil {
		t.Fatalf("fetch jms destinations failed: %v", err)
	}

	if len(dests) != 1 {
		t.Fatalf("expected 1 destination, got %d", len(dests))
	}
	d := dests[0]
	if d.Name != "Queue1" || d.Type != "queue" || d.MessagesCurrentCount != 12 {
		t.Fatalf("unexpected destination contents: %+v", d)
	}
	if d.MessagesPendingCount != 3 || d.MessagesAddedCount != 40 || d.ConsumerCount != 5 {
		t.Fatalf("unexpected message/consumer values: %+v", d)
	}
}

func TestClientFetchApplications(t *testing.T) {
	bridge := &fakeBridge{
		responses: map[string]*jmxbridge.Response{
			"APPLICATIONS": {
				Status: "OK",
				Data: map[string]interface{}{
					"applications": []interface{}{
						map[string]interface{}{
							"name":                   "sample-app",
							"module":                 "moduleA",
							"requestCount":           120.0,
							"averageResponseTime":    0.24,
							"activeSessions":         5.0,
							"liveSessions":           7.0,
							"sessionCreates":         60.0,
							"sessionInvalidates":     10.0,
							"transactionsCommitted":  40.0,
							"transactionsRolledBack": 2.0,
						},
					},
				},
			},
		},
	}

	client, err := NewClient(Config{JMXURL: "service:jmx:iiop://localhost:2809"}, nopLogger{}, WithBridge(bridge))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	metrics, err := client.FetchApplications(context.Background(), 10, true, true)
	if err != nil {
		t.Fatalf("fetch applications failed: %v", err)
	}

	if len(metrics) != 1 {
		t.Fatalf("expected 1 application, got %d", len(metrics))
	}
	app := metrics[0]
	if app.Name != "sample-app" || app.Module != "moduleA" || app.Requests != 120 {
		t.Fatalf("unexpected application metric: %+v", app)
	}
	if app.ActiveSessions != 5 || app.TransactionsCommitted != 40 {
		t.Fatalf("unexpected session/transaction metrics: %+v", app)
	}
}
