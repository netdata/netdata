//go:build cgo
// +build cgo

package jmx

import (
	"context"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/websphere/common"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/websphere/jmx/contexts"
	jmxproto "github.com/netdata/netdata/go/plugins/plugin/ibm.d/protocols/websphere/jmx"
)

type fakeClient struct {
	jvm             *jmxproto.JVMStats
	threadPools     []jmxproto.ThreadPool
	jdbcPools       []jmxproto.JDBCPool
	jcaPools        []jmxproto.JCAPool
	jmsDestinations []jmxproto.JMSDestination
	applications    []jmxproto.ApplicationMetric
	jvmErr          error
	poolErr         error
	jdbcErr         error
	jcaErr          error
	jmsErr          error
}

func (f *fakeClient) Start(context.Context) error { return nil }
func (f *fakeClient) Shutdown()                   {}

func (f *fakeClient) FetchJVM(context.Context) (*jmxproto.JVMStats, error) {
	if f.jvmErr != nil {
		return nil, f.jvmErr
	}
	return f.jvm, nil
}

func (f *fakeClient) FetchThreadPools(context.Context, int) ([]jmxproto.ThreadPool, error) {
	if f.poolErr != nil {
		return nil, f.poolErr
	}
	return f.threadPools, nil
}

func (f *fakeClient) FetchJDBCPools(context.Context, int) ([]jmxproto.JDBCPool, error) {
	if f.jdbcErr != nil {
		return nil, f.jdbcErr
	}
	return f.jdbcPools, nil
}

func (f *fakeClient) FetchJMSDestinations(context.Context, int) ([]jmxproto.JMSDestination, error) {
	if f.jmsErr != nil {
		return nil, f.jmsErr
	}
	return f.jmsDestinations, nil
}

func (f *fakeClient) FetchJCAPools(context.Context, int) ([]jmxproto.JCAPool, error) {
	if f.jcaErr != nil {
		return nil, f.jcaErr
	}
	return f.jcaPools, nil
}

func (f *fakeClient) FetchApplications(context.Context, int, bool, bool) ([]jmxproto.ApplicationMetric, error) {
	return f.applications, nil
}

func newTestCollector(t *testing.T, client jmxClient) *Collector {
	t.Helper()

	cfg := defaultConfig()
	collector := &Collector{
		Config:   cfg,
		client:   client,
		identity: common.Identity{},
	}

	collector.Collector.Config = cfg.Config
	if err := collector.Collector.Init(context.Background()); err != nil {
		t.Fatalf("framework init failed: %v", err)
	}
	collector.RegisterContexts(contexts.GetAllContexts()...)
	collector.SetImpl(collector)

	return collector
}

func TestCollectorCollectOnceExportsJVMMetrics(t *testing.T) {
	client := &fakeClient{
		jvm: &jmxproto.JVMStats{
			Heap: struct {
				Used      float64
				Committed float64
				Max       float64
			}{
				Used:      512,
				Committed: 1024,
				Max:       2048,
			},
			NonHeap: struct {
				Used      float64
				Committed float64
			}{
				Used:      128,
				Committed: 256,
			},
			GC: struct {
				Count float64
				Time  float64
			}{
				Count: 12,
				Time:  345,
			},
			Threads: struct {
				Count   float64
				Daemon  float64
				Peak    float64
				Started float64
			}{
				Count:   44,
				Daemon:  30,
				Peak:    60,
				Started: 100,
			},
			Classes: struct {
				Loaded   float64
				Unloaded float64
			}{
				Loaded:   5000,
				Unloaded: 200,
			},
			CPU:    struct{ ProcessUsage float64 }{ProcessUsage: 42},
			Uptime: 900,
		},
	}

	collector := newTestCollector(t, client)
	collector.Config.CollectThreadPoolMetrics = confopt.AutoBoolDisabled

	metrics := collector.Collect(context.Background())

	expects := map[string]int64{
		"websphere_jmx.jvm_heap_memory.used":         512,
		"websphere_jmx.jvm_heap_memory.committed":    1024,
		"websphere_jmx.jvm_heap_memory.max":          2048,
		"websphere_jmx.jvm_heap_usage.usage":         25,
		"websphere_jmx.jvm_nonheap_memory.used":      128,
		"websphere_jmx.jvm_nonheap_memory.committed": 256,
		"websphere_jmx.jvm_gc_count.collections":     12,
		"websphere_jmx.jvm_gc_time.time":             345,
		"websphere_jmx.jvm_threads.total":            44,
		"websphere_jmx.jvm_threads.daemon":           30,
		"websphere_jmx.jvm_thread_states.peak":       60,
		"websphere_jmx.jvm_thread_states.started":    100,
		"websphere_jmx.jvm_classes.loaded":           5000,
		"websphere_jmx.jvm_classes.unloaded":         200,
		"websphere_jmx.jvm_process_cpu_usage.cpu":    42,
		"websphere_jmx.jvm_uptime.uptime":            900,
	}

	for key, want := range expects {
		if got, ok := metrics[key]; !ok {
			t.Errorf("expected metric %s to be present", key)
		} else if got != want {
			t.Errorf("metric %s mismatch: want %d got %d", key, want, got)
		}
	}
}

func TestCollectorCollectsThreadPools(t *testing.T) {
	client := &fakeClient{
		jvm: &jmxproto.JVMStats{},
		threadPools: []jmxproto.ThreadPool{
			{Name: "Default", PoolSize: 50, ActiveCount: 5, MaximumPoolSize: 75},
			{Name: "WebContainer", PoolSize: 80, ActiveCount: 12, MaximumPoolSize: 100},
		},
	}

	collector := newTestCollector(t, client)

	metrics := collector.Collect(context.Background())

	expected := map[string]int64{
		"websphere_jmx.threadpool_size.default.size":          50,
		"websphere_jmx.threadpool_size.default.max":           75,
		"websphere_jmx.threadpool_active.default.active":      5,
		"websphere_jmx.threadpool_size.webcontainer.size":     80,
		"websphere_jmx.threadpool_size.webcontainer.max":      100,
		"websphere_jmx.threadpool_active.webcontainer.active": 12,
	}

	for key, want := range expected {
		if got, ok := metrics[key]; !ok {
			t.Fatalf("expected metric %s to be present", key)
		} else if got != want {
			t.Errorf("metric %s mismatch: want %d got %d", key, want, got)
		}
	}
}

func TestCollectorCollectsJDBCPools(t *testing.T) {
	client := &fakeClient{
		jvm: &jmxproto.JVMStats{},
		jdbcPools: []jmxproto.JDBCPool{
			{
				Name:                    "DefaultDS",
				PoolSize:                40,
				NumConnectionsUsed:      8,
				NumConnectionsFree:      12,
				AvgWaitTime:             1.5,
				AvgInUseTime:            2.5,
				NumConnectionsCreated:   100,
				NumConnectionsDestroyed: 90,
				WaitingThreadCount:      3,
			},
		},
	}

	collector := newTestCollector(t, client)
	collector.Config.CollectThreadPoolMetrics = confopt.AutoBoolDisabled

	metrics := collector.Collect(context.Background())

	expected := map[string]int64{
		"websphere_jmx.jdbc_pool_size.defaultds.size":               40,
		"websphere_jmx.jdbc_pool_usage.defaultds.active":            8,
		"websphere_jmx.jdbc_pool_usage.defaultds.free":              12,
		"websphere_jmx.jdbc_pool_wait_time.defaultds.wait":          1500,
		"websphere_jmx.jdbc_pool_use_time.defaultds.use":            2500,
		"websphere_jmx.jdbc_pool_connections.defaultds.created":     100,
		"websphere_jmx.jdbc_pool_connections.defaultds.destroyed":   90,
		"websphere_jmx.jdbc_pool_waiting_threads.defaultds.waiting": 3,
	}

	for key, want := range expected {
		if got, ok := metrics[key]; !ok {
			t.Fatalf("expected metric %s to be present", key)
		} else if got != want {
			t.Errorf("metric %s mismatch: want %d got %d", key, want, got)
		}
	}
}

func TestCollectorCollectsJMSDestinations(t *testing.T) {
	client := &fakeClient{
		jvm: &jmxproto.JVMStats{},
		jmsDestinations: []jmxproto.JMSDestination{
			{
				Name:                 "Queue1",
				Type:                 "queue",
				MessagesCurrentCount: 12,
				MessagesPendingCount: 4,
				MessagesAddedCount:   80,
				ConsumerCount:        6,
			},
		},
	}

	collector := newTestCollector(t, client)
	collector.Config.CollectThreadPoolMetrics = confopt.AutoBoolDisabled
	collector.Config.CollectJDBCMetrics = confopt.AutoBoolDisabled

	metrics := collector.Collect(context.Background())

	expected := map[string]int64{
		"websphere_jmx.jms_messages_current.queue1_queue.current": 12,
		"websphere_jmx.jms_messages_pending.queue1_queue.pending": 4,
		"websphere_jmx.jms_messages_total.queue1_queue.total":     80,
		"websphere_jmx.jms_consumers.queue1_queue.consumers":      6,
	}

	for key, want := range expected {
		if got, ok := metrics[key]; !ok {
			t.Fatalf("expected metric %s to be present", key)
		} else if got != want {
			t.Errorf("metric %s mismatch: want %d got %d", key, want, got)
		}
	}
}

func TestCollectorCollectsApplications(t *testing.T) {
	client := &fakeClient{
		jvm: &jmxproto.JVMStats{},
		applications: []jmxproto.ApplicationMetric{
			{
				Name:                   "sample-app",
				Module:                 "moduleA",
				Requests:               120,
				ResponseTime:           0.24,
				ActiveSessions:         5,
				LiveSessions:           7,
				SessionCreates:         60,
				SessionInvalidates:     10,
				TransactionsCommitted:  40,
				TransactionsRolledback: 2,
			},
		},
	}

	collector := newTestCollector(t, client)
	collector.Config.CollectThreadPoolMetrics = confopt.AutoBoolDisabled
	collector.Config.CollectJDBCMetrics = confopt.AutoBoolDisabled
	collector.Config.CollectJMSMetrics = confopt.AutoBoolDisabled
	collector.Config.CollectWebAppMetrics = confopt.AutoBoolEnabled
	collector.Config.CollectSessionMetrics = confopt.AutoBoolEnabled
	collector.Config.CollectTransactionMetrics = confopt.AutoBoolEnabled

	metrics := collector.Collect(context.Background())

	expected := map[string]int64{
		"websphere_jmx.app_requests.sample_app_modulea.requests":           120,
		"websphere_jmx.app_response_time.sample_app_modulea.response_time": 240,
		"websphere_jmx.app_sessions_active.sample_app_modulea.active":      5,
		"websphere_jmx.app_sessions_live.sample_app_modulea.live":          7,
		"websphere_jmx.app_session_events.sample_app_modulea.creates":      60,
		"websphere_jmx.app_session_events.sample_app_modulea.invalidates":  10,
		"websphere_jmx.app_transactions.sample_app_modulea.committed":      40,
		"websphere_jmx.app_transactions.sample_app_modulea.rolledback":     2,
	}

	for key, want := range expected {
		if got, ok := metrics[key]; !ok {
			t.Fatalf("expected metric %s to be present", key)
		} else if got != want {
			t.Errorf("metric %s mismatch: want %d got %d", key, want, got)
		}
	}
}
func TestCollectorCollectsJCAPools(t *testing.T) {
	client := &fakeClient{
		jvm: &jmxproto.JVMStats{},
		jcaPools: []jmxproto.JCAPool{
			{
				Name:                    "JCAPool1",
				PoolSize:                20,
				NumConnectionsUsed:      6,
				NumConnectionsFree:      4,
				AvgWaitTime:             1.2,
				AvgInUseTime:            3.4,
				NumConnectionsCreated:   30,
				NumConnectionsDestroyed: 12,
				WaitingThreadCount:      2,
			},
		},
	}

	collector := newTestCollector(t, client)
	collector.Config.CollectThreadPoolMetrics = confopt.AutoBoolDisabled
	collector.Config.CollectJDBCMetrics = confopt.AutoBoolDisabled
	collector.Config.CollectJMSMetrics = confopt.AutoBoolDisabled
	collector.Config.CollectWebAppMetrics = confopt.AutoBoolDisabled

	metrics := collector.Collect(context.Background())

	expected := map[string]int64{
		"websphere_jmx.jca_pool_size.jcapool1.size":               20,
		"websphere_jmx.jca_pool_usage.jcapool1.active":            6,
		"websphere_jmx.jca_pool_usage.jcapool1.free":              4,
		"websphere_jmx.jca_pool_wait_time.jcapool1.wait":          1200,
		"websphere_jmx.jca_pool_use_time.jcapool1.use":            3400,
		"websphere_jmx.jca_pool_connections.jcapool1.created":     30,
		"websphere_jmx.jca_pool_connections.jcapool1.destroyed":   12,
		"websphere_jmx.jca_pool_waiting_threads.jcapool1.waiting": 2,
	}

	for key, want := range expected {
		if got, ok := metrics[key]; !ok {
			t.Fatalf("expected metric %s to be present", key)
		} else if got != want {
			t.Errorf("metric %s mismatch: want %d got %d", key, want, got)
		}
	}
}
