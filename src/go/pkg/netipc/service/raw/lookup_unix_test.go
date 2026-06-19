//go:build unix

package raw

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	"github.com/netdata/netdata/go/plugins/pkg/netipc/transport/posix"
)

func testCgroupsLookupHandler(req *protocol.CgroupsLookupRequestView, builder *protocol.CgroupsLookupBuilder) bool {
	for i := uint32(0); i < req.ItemCount; i++ {
		path, err := req.Item(i)
		if err != nil {
			return false
		}
		if path.String() == "/known" {
			if err := builder.Add(
				protocol.CgroupLookupKnown,
				protocol.OrchestratorK8s,
				path.Bytes(),
				[]byte("pod-a"),
				[]struct{ Key, Value []byte }{
					{Key: []byte("namespace"), Value: []byte("default")},
				},
			); err != nil {
				return false
			}
		} else {
			if err := builder.Add(
				protocol.CgroupLookupUnknownRetryLater,
				0,
				path.Bytes(),
				nil,
				nil,
			); err != nil {
				return false
			}
		}
	}
	return true
}

func testAppsLookupHandler(req *protocol.AppsLookupRequestView, builder *protocol.AppsLookupBuilder) bool {
	for i := uint32(0); i < req.ItemCount; i++ {
		pid, err := req.Item(i)
		if err != nil {
			return false
		}
		switch pid {
		case 1234:
			if err := builder.Add(
				protocol.PidLookupKnown,
				protocol.AppsCgroupKnown,
				protocol.OrchestratorDocker,
				pid,
				1,
				1000,
				42,
				[]byte("nginx"),
				[]byte("/docker/abc"),
				[]byte("container-a"),
				[]struct{ Key, Value []byte }{
					{Key: []byte("image"), Value: []byte("nginx:latest")},
				},
			); err != nil {
				return false
			}
		case 0:
			if err := builder.Add(
				protocol.PidLookupKnown,
				protocol.AppsCgroupHostRoot,
				0,
				pid,
				0,
				0,
				0,
				[]byte("swapper"),
				nil,
				nil,
				nil,
			); err != nil {
				return false
			}
		default:
			if err := builder.Add(
				protocol.PidLookupUnknown,
				protocol.AppsCgroupKnown,
				0,
				pid,
				0,
				protocol.NipcUIDUnset,
				0,
				nil,
				nil,
				nil,
				nil,
			); err != nil {
				return false
			}
		}
	}
	return true
}

func startLookupTestServer(service string, method uint16, handler DispatchHandler) *testServer {
	return startLookupTestServerWithWorkers(service, method, handler, 8)
}

func startLookupTestServerWithWorkers(service string, method uint16, handler DispatchHandler, workers int) *testServer {
	return startLookupTestServerWithConfig(service, method, handler, testServerConfig(), workers)
}

func startLookupTestServerWithConfig(service string, method uint16, handler DispatchHandler, config posix.ServerConfig, workers int) *testServer {
	ensureRunDir()
	cleanupAll(service)

	s := NewServerWithWorkers(testRunDir, service, config, method, handler, workers)
	doneCh := make(chan struct{})

	go func() {
		defer close(doneCh)
		s.Run()
	}()

	waitUnixServerReady(service)
	return &testServer{server: s, doneCh: doneCh}
}

func verifyCgroupsLookupView(t *testing.T, view *protocol.CgroupsLookupResponseView) {
	t.Helper()
	if err := checkCgroupsLookupView(view); err != nil {
		t.Fatal(err)
	}
}

func checkCgroupsLookupView(view *protocol.CgroupsLookupResponseView) error {
	if view.ItemCount != 2 {
		return fmt.Errorf("item count = %d, want 2", view.ItemCount)
	}
	item0, err := view.Item(0)
	if err != nil {
		return fmt.Errorf("item 0: %w", err)
	}
	if item0.Status != protocol.CgroupLookupKnown ||
		item0.Orchestrator != protocol.OrchestratorK8s ||
		item0.Path.String() != "/known" ||
		item0.Name.String() != "pod-a" ||
		item0.LabelCount != 1 {
		return fmt.Errorf("bad item 0: %+v", item0)
	}
	item1, err := view.Item(1)
	if err != nil {
		return fmt.Errorf("item 1: %w", err)
	}
	if item1.Status != protocol.CgroupLookupUnknownRetryLater || item1.Path.String() != "/missing" {
		return fmt.Errorf("bad item 1: %+v", item1)
	}
	return nil
}

func verifyAppsLookupView(t *testing.T, view *protocol.AppsLookupResponseView) {
	t.Helper()
	if err := checkAppsLookupView(view); err != nil {
		t.Fatal(err)
	}
}

func checkAppsLookupView(view *protocol.AppsLookupResponseView) error {
	if view.ItemCount != 3 {
		return fmt.Errorf("item count = %d, want 3", view.ItemCount)
	}
	item0, err := view.Item(0)
	if err != nil {
		return fmt.Errorf("item 0: %w", err)
	}
	if item0.Pid != 1234 ||
		item0.Status != protocol.PidLookupKnown ||
		item0.CgroupStatus != protocol.AppsCgroupKnown ||
		item0.Comm.String() != "nginx" ||
		item0.CgroupPath.String() != "/docker/abc" ||
		item0.LabelCount != 1 {
		return fmt.Errorf("bad item 0: %+v", item0)
	}
	item1, err := view.Item(1)
	if err != nil {
		return fmt.Errorf("item 1: %w", err)
	}
	if item1.Pid != 0 || item1.CgroupStatus != protocol.AppsCgroupHostRoot || item1.CgroupPath.Len() != 0 {
		return fmt.Errorf("bad item 1: %+v", item1)
	}
	item2, err := view.Item(2)
	if err != nil {
		return fmt.Errorf("item 2: %w", err)
	}
	if item2.Pid != 9999 || item2.Status != protocol.PidLookupUnknown || item2.Uid != protocol.NipcUIDUnset {
		return fmt.Errorf("bad item 2: %+v", item2)
	}
	return nil
}

func TestCgroupsLookupCall(t *testing.T) {
	svc := "go_svc_cgroups_lookup"
	ts := startLookupTestServer(svc, protocol.MethodCgroupsLookup, CgroupsLookupDispatch(testCgroupsLookupHandler))
	defer ts.stop()

	client := NewCgroupsLookupClient(testRunDir, svc, testClientConfig())
	client.Refresh()
	if !client.Ready() {
		t.Fatal("client not ready")
	}

	view, err := client.CallCgroupsLookup([][]byte{[]byte("/known"), []byte("/missing")})
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	verifyCgroupsLookupView(t, view)
}

func TestAppsLookupCall(t *testing.T) {
	svc := "go_svc_apps_lookup"
	ts := startLookupTestServer(svc, protocol.MethodAppsLookup, AppsLookupDispatch(testAppsLookupHandler))
	defer ts.stop()

	client := NewAppsLookupClient(testRunDir, svc, testClientConfig())
	client.Refresh()
	if !client.Ready() {
		t.Fatal("client not ready")
	}

	view, err := client.CallAppsLookup([]uint32{1234, 0, 9999})
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	verifyAppsLookupView(t, view)
}

func TestLookupZeroItemCalls(t *testing.T) {
	t.Run("apps", func(t *testing.T) {
		svc := uniqueUnixService("go_svc_apps_lookup_zero")
		ts := startLookupTestServer(svc, protocol.MethodAppsLookup, AppsLookupDispatch(testAppsLookupHandler))
		defer ts.stop()

		client := NewAppsLookupClient(testRunDir, svc, testClientConfig())
		defer client.Close()
		refreshUnixClientReady(t, client)

		view, err := client.CallAppsLookup(nil)
		if err != nil {
			t.Fatalf("zero-item apps lookup: %v", err)
		}
		if view.ItemCount != 0 || view.Generation != 0 {
			t.Fatalf("zero-item apps response = count %d generation %d, want 0/0", view.ItemCount, view.Generation)
		}
	})

	t.Run("cgroups", func(t *testing.T) {
		svc := uniqueUnixService("go_svc_cgroups_lookup_zero")
		ts := startLookupTestServer(svc, protocol.MethodCgroupsLookup, CgroupsLookupDispatch(testCgroupsLookupHandler))
		defer ts.stop()

		client := NewCgroupsLookupClient(testRunDir, svc, testClientConfig())
		defer client.Close()
		refreshUnixClientReady(t, client)

		view, err := client.CallCgroupsLookup(nil)
		if err != nil {
			t.Fatalf("zero-item cgroups lookup: %v", err)
		}
		if view.ItemCount != 0 || view.Generation != 0 {
			t.Fatalf("zero-item cgroups response = count %d generation %d, want 0/0", view.ItemCount, view.Generation)
		}
	})
}

func TestCgroupsLookupTransparentPayloadExceededRetry(t *testing.T) {
	svc := uniqueUnixService("go_svc_cgroups_lookup_scale")
	cfg := testServerConfig()
	cfg.MaxResponsePayloadBytes = 256
	var calls atomic.Uint32
	ts := startLookupTestServerWithConfig(svc, protocol.MethodCgroupsLookup, CgroupsLookupDispatch(
		func(req *protocol.CgroupsLookupRequestView, builder *protocol.CgroupsLookupBuilder) bool {
			calls.Add(1)
			builder.SetGeneration(7)
			for i := uint32(0); i < req.ItemCount; i++ {
				path, err := req.Item(i)
				if err != nil {
					return false
				}
				name := []byte("ok")
				var labels []struct{ Key, Value []byte }
				if path.String() == "/huge" {
					name = bytes.Repeat([]byte("x"), 512)
				} else if path.String() == "/huge-label" {
					labels = []struct{ Key, Value []byte }{
						{Key: []byte("huge"), Value: bytes.Repeat([]byte("l"), 512)},
					}
				}
				if err := builder.Add(protocol.CgroupLookupKnown, protocol.OrchestratorK8s, path.Bytes(), name, labels); err != nil {
					return false
				}
			}
			return true
		},
	), cfg, 8)
	defer ts.stop()

	client := NewCgroupsLookupClient(testRunDir, svc, testClientConfig())
	defer client.Close()
	client.Refresh()
	if !client.Ready() {
		t.Fatal("client not ready")
	}

	view, err := client.CallCgroupsLookup([][]byte{[]byte("/a"), []byte("/huge"), []byte("/huge-label"), []byte("/b")})
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	if calls.Load() < 2 {
		t.Fatalf("handler calls = %d, want at least 2", calls.Load())
	}
	if view.ItemCount != 4 || view.Generation != 7 {
		t.Fatalf("header = count %d generation %d", view.ItemCount, view.Generation)
	}
	item0, _ := view.Item(0)
	item1, _ := view.Item(1)
	item2, _ := view.Item(2)
	item3, _ := view.Item(3)
	if item0.Status != protocol.CgroupLookupKnown || item0.Path.String() != "/a" {
		t.Fatalf("item0 = %+v", item0)
	}
	if item1.Status != protocol.CgroupLookupOversizedItem || item1.Path.String() != "/huge" {
		t.Fatalf("item1 = %+v", item1)
	}
	if item2.Status != protocol.CgroupLookupOversizedItem || item2.Path.String() != "/huge-label" {
		t.Fatalf("item2 = %+v", item2)
	}
	if item3.Status != protocol.CgroupLookupKnown || item3.Path.String() != "/b" || item3.Name.String() != "ok" {
		t.Fatalf("item3 = %+v", item3)
	}
}

func TestAppsLookupTransparentPayloadExceededRetry(t *testing.T) {
	svc := uniqueUnixService("go_svc_apps_lookup_scale")
	cfg := testServerConfig()
	cfg.MaxResponsePayloadBytes = 320
	var calls atomic.Uint32
	ts := startLookupTestServerWithConfig(svc, protocol.MethodAppsLookup, AppsLookupDispatch(
		func(req *protocol.AppsLookupRequestView, builder *protocol.AppsLookupBuilder) bool {
			calls.Add(1)
			builder.SetGeneration(9)
			for i := uint32(0); i < req.ItemCount; i++ {
				pid, err := req.Item(i)
				if err != nil {
					return false
				}
				cgroupPath := []byte("/ok")
				var labels []struct{ Key, Value []byte }
				if pid == 22 {
					cgroupPath = append([]byte("/"), bytes.Repeat([]byte("x"), 1024)...)
				} else if pid == 44 {
					labels = []struct{ Key, Value []byte }{
						{Key: []byte("huge"), Value: bytes.Repeat([]byte("l"), 512)},
					}
				}
				if err := builder.Add(
					protocol.PidLookupKnown,
					protocol.AppsCgroupKnown,
					0,
					pid,
					0,
					0,
					1,
					[]byte("ok"),
					cgroupPath,
					[]byte("name"),
					labels,
				); err != nil {
					return false
				}
			}
			return true
		},
	), cfg, 8)
	defer ts.stop()

	client := NewAppsLookupClient(testRunDir, svc, testClientConfig())
	defer client.Close()
	client.Refresh()
	if !client.Ready() {
		t.Fatal("client not ready")
	}

	view, err := client.CallAppsLookup([]uint32{11, 22, 44, 33})
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	if calls.Load() < 2 {
		t.Fatalf("handler calls = %d, want at least 2", calls.Load())
	}
	if view.ItemCount != 4 || view.Generation != 9 {
		t.Fatalf("header = count %d generation %d", view.ItemCount, view.Generation)
	}
	item0, _ := view.Item(0)
	item1, _ := view.Item(1)
	item2, _ := view.Item(2)
	item3, _ := view.Item(3)
	if item0.Status != protocol.PidLookupKnown || item0.Pid != 11 || item0.Comm.String() != "ok" {
		t.Fatalf("item0 = %+v", item0)
	}
	if item1.Status != protocol.PidLookupOversizedItem || item1.Pid != 22 {
		t.Fatalf("item1 = %+v", item1)
	}
	if item2.Status != protocol.PidLookupOversizedItem || item2.Pid != 44 {
		t.Fatalf("item2 = %+v", item2)
	}
	if item3.Status != protocol.PidLookupKnown || item3.Pid != 33 || item3.Comm.String() != "ok" {
		t.Fatalf("item3 = %+v", item3)
	}
}

func runUnixAppsLookupRequestBoundary(t *testing.T, name string, requestCap, expectedMaxItems, minCalls uint32) {
	t.Helper()

	svc := uniqueUnixService("go_svc_apps_lookup_request_split_" + name)
	scfg := testServerConfig()
	scfg.MaxRequestPayloadBytes = requestCap
	scfg.MaxResponsePayloadBytes = 4096
	var calls atomic.Uint32
	var maxSeen atomic.Uint32
	ts := startLookupTestServerWithConfig(svc, protocol.MethodAppsLookup, AppsLookupDispatch(
		func(req *protocol.AppsLookupRequestView, builder *protocol.AppsLookupBuilder) bool {
			calls.Add(1)
			for {
				current := maxSeen.Load()
				if req.ItemCount <= current || maxSeen.CompareAndSwap(current, req.ItemCount) {
					break
				}
			}
			builder.SetGeneration(11)
			for i := uint32(0); i < req.ItemCount; i++ {
				pid, err := req.Item(i)
				if err != nil {
					return false
				}
				if err := builder.Add(protocol.PidLookupUnknown, 0, 0, pid, 0, protocol.NipcUIDUnset, 0, nil, nil, nil, nil); err != nil {
					return false
				}
			}
			return true
		},
	), scfg, 8)
	defer ts.stop()

	ccfg := testClientConfig()
	ccfg.MaxRequestPayloadBytes = requestCap
	client := NewAppsLookupClient(testRunDir, svc, ccfg)
	defer client.Close()
	client.Refresh()
	if !client.Ready() {
		t.Fatal("client not ready")
	}

	pids := []uint32{4, 1, 4, 7, 1, 9, 7}
	view, err := client.CallAppsLookup(pids)
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	if view.ItemCount != 7 || view.Generation != 11 {
		t.Fatalf("header = count %d generation %d", view.ItemCount, view.Generation)
	}
	for i, want := range pids {
		item, err := view.Item(uint32(i))
		if err != nil {
			t.Fatalf("item %d decode: %v", i, err)
		}
		if item.Status != protocol.PidLookupUnknown || item.Pid != want {
			t.Fatalf("item %d = %+v, want pid %d unknown", i, item, want)
		}
	}
	if calls.Load() < minCalls {
		t.Fatalf("handler calls = %d, want at least %d", calls.Load(), minCalls)
	}
	if maxSeen.Load() != expectedMaxItems {
		t.Fatalf("max request items = %d, want %d", maxSeen.Load(), expectedMaxItems)
	}
}

func runUnixCgroupsLookupRequestBoundary(t *testing.T, name string, requestCap, expectedMaxItems, minCalls uint32) {
	t.Helper()

	svc := uniqueUnixService("go_svc_cgroups_lookup_request_split_" + name)
	scfg := testServerConfig()
	scfg.MaxRequestPayloadBytes = requestCap
	scfg.MaxResponsePayloadBytes = 4096
	var calls atomic.Uint32
	var maxSeen atomic.Uint32
	ts := startLookupTestServerWithConfig(svc, protocol.MethodCgroupsLookup, CgroupsLookupDispatch(
		func(req *protocol.CgroupsLookupRequestView, builder *protocol.CgroupsLookupBuilder) bool {
			calls.Add(1)
			for {
				current := maxSeen.Load()
				if req.ItemCount <= current || maxSeen.CompareAndSwap(current, req.ItemCount) {
					break
				}
			}
			builder.SetGeneration(12)
			for i := uint32(0); i < req.ItemCount; i++ {
				path, err := req.Item(i)
				if err != nil {
					return false
				}
				if err := builder.Add(protocol.CgroupLookupKnown, protocol.OrchestratorK8s, path.Bytes(), []byte("ok"), nil); err != nil {
					return false
				}
			}
			return true
		},
	), scfg, 8)
	defer ts.stop()

	ccfg := testClientConfig()
	ccfg.MaxRequestPayloadBytes = requestCap
	client := NewCgroupsLookupClient(testRunDir, svc, ccfg)
	defer client.Close()
	client.Refresh()
	if !client.Ready() {
		t.Fatal("client not ready")
	}

	paths := [][]byte{
		[]byte("/bbbbbb"),
		[]byte("/aaaaaa"),
		[]byte("/bbbbbb"),
		[]byte("/cccccc"),
		[]byte("/aaaaaa"),
	}
	view, err := client.CallCgroupsLookup(paths)
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	if view.ItemCount != 5 || view.Generation != 12 {
		t.Fatalf("header = count %d generation %d", view.ItemCount, view.Generation)
	}
	for i, want := range paths {
		item, err := view.Item(uint32(i))
		if err != nil {
			t.Fatalf("item %d decode: %v", i, err)
		}
		if item.Status != protocol.CgroupLookupKnown || item.Path.String() != string(want) {
			t.Fatalf("item %d = %+v, want path %q known", i, item, want)
		}
	}
	if calls.Load() < minCalls {
		t.Fatalf("handler calls = %d, want at least %d", calls.Load(), minCalls)
	}
	if maxSeen.Load() != expectedMaxItems {
		t.Fatalf("max request items = %d, want %d", maxSeen.Load(), expectedMaxItems)
	}
}

func TestLookupProactiveRequestSplit(t *testing.T) {
	for _, tc := range []struct {
		name             string
		requestCap       uint32
		expectedMaxItems uint32
		minCalls         uint32
	}{
		{name: "cap_minus_1", requestCap: 63, expectedMaxItems: 2, minCalls: 4},
		{name: "cap_exact", requestCap: 64, expectedMaxItems: 3, minCalls: 3},
		{name: "cap_plus_1", requestCap: 65, expectedMaxItems: 3, minCalls: 3},
	} {
		t.Run("apps "+tc.name, func(t *testing.T) {
			runUnixAppsLookupRequestBoundary(t, tc.name, tc.requestCap, tc.expectedMaxItems, tc.minCalls)
		})
	}

	for _, tc := range []struct {
		name             string
		requestCap       uint32
		expectedMaxItems uint32
		minCalls         uint32
	}{
		{name: "cap_minus_1", requestCap: 47, expectedMaxItems: 1, minCalls: 5},
		{name: "cap_exact", requestCap: 48, expectedMaxItems: 2, minCalls: 3},
		{name: "cap_plus_1", requestCap: 49, expectedMaxItems: 2, minCalls: 3},
	} {
		t.Run("cgroups "+tc.name, func(t *testing.T) {
			runUnixCgroupsLookupRequestBoundary(t, tc.name, tc.requestCap, tc.expectedMaxItems, tc.minCalls)
		})
	}

	t.Run("cgroups oversized request key", func(t *testing.T) {
		svc := uniqueUnixService("go_svc_cgroups_lookup_oversized_request_key")
		scfg := testServerConfig()
		scfg.MaxRequestPayloadBytes = 48
		scfg.MaxResponsePayloadBytes = 4096
		var calls atomic.Uint32
		var maxSeen atomic.Uint32
		ts := startLookupTestServerWithConfig(svc, protocol.MethodCgroupsLookup, CgroupsLookupDispatch(
			func(req *protocol.CgroupsLookupRequestView, builder *protocol.CgroupsLookupBuilder) bool {
				calls.Add(1)
				for {
					current := maxSeen.Load()
					if req.ItemCount <= current || maxSeen.CompareAndSwap(current, req.ItemCount) {
						break
					}
				}
				builder.SetGeneration(12)
				for i := uint32(0); i < req.ItemCount; i++ {
					path, err := req.Item(i)
					if err != nil {
						return false
					}
					if err := builder.Add(protocol.CgroupLookupKnown, protocol.OrchestratorK8s, path.Bytes(), []byte("ok"), nil); err != nil {
						return false
					}
				}
				return true
			},
		), scfg, 8)
		defer ts.stop()

		ccfg := testClientConfig()
		ccfg.MaxRequestPayloadBytes = 48
		client := NewCgroupsLookupClient(testRunDir, svc, ccfg)
		defer client.Close()
		client.Refresh()
		if !client.Ready() {
			t.Fatal("client not ready")
		}

		paths := [][]byte{
			[]byte("/request-key-too-large-for-configured-cap"),
			[]byte("/ok"),
		}
		view, err := client.CallCgroupsLookup(paths)
		if err != nil {
			t.Fatalf("call failed: %v", err)
		}
		if view.ItemCount != 2 || view.Generation != 12 {
			t.Fatalf("header = count %d generation %d", view.ItemCount, view.Generation)
		}
		item0, _ := view.Item(0)
		if item0.Status != protocol.CgroupLookupOversizedItem || item0.Path.String() != string(paths[0]) {
			t.Fatalf("item0 = %+v", item0)
		}
		item1, _ := view.Item(1)
		if item1.Status != protocol.CgroupLookupKnown || item1.Path.String() != "/ok" || item1.Name.String() != "ok" {
			t.Fatalf("item1 = %+v", item1)
		}
		if calls.Load() != 1 {
			t.Fatalf("handler calls = %d, want 1", calls.Load())
		}
		if maxSeen.Load() != 1 {
			t.Fatalf("max request items = %d, want 1", maxSeen.Load())
		}
	})
}

func TestLookupLargeLogicalCalls(t *testing.T) {
	t.Run("apps 8192", func(t *testing.T) {
		runUnixLargeAppsLookup(t, "go_svc_apps_lookup_large_8192", lookupTopologyScaleItems)
	})
	t.Run("apps 32768", func(t *testing.T) {
		runUnixLargeAppsLookup(t, "go_svc_apps_lookup_large_32768", lookupHPCScaleItems)
	})
	t.Run("cgroups 8192", func(t *testing.T) {
		runUnixLargeCgroupsLookup(t, "go_svc_cgroups_lookup_large_8192", lookupTopologyScaleItems)
	})
	t.Run("cgroups 32768", func(t *testing.T) {
		runUnixLargeCgroupsLookup(t, "go_svc_cgroups_lookup_large_32768", lookupHPCScaleItems)
	})
}

func runUnixLargeAppsLookup(t *testing.T, service string, itemCount int) {
	t.Helper()

	pids := largeLookupPids(itemCount)
	cfg := testServerConfig()
	cfg.MaxRequestPayloadBytes = lookupScaleRequestPayloadBytes
	cfg.MaxResponsePayloadBytes = responseBufSize
	var calls atomic.Uint32
	var maxSeen atomic.Uint32
	ts := startLookupTestServerWithConfig(
		service,
		protocol.MethodAppsLookup,
		AppsLookupDispatch(largeAppsLookupHandler(&calls, &maxSeen)),
		cfg,
		8,
	)
	defer ts.stop()

	ccfg := testClientConfig()
	ccfg.MaxRequestPayloadBytes = lookupScaleRequestPayloadBytes
	client := NewAppsLookupClient(testRunDir, service, ccfg)
	defer client.Close()
	client.SetCallTimeout(lookupScaleCallTimeoutMs)
	refreshUnixClientReady(t, client)

	view, err := client.CallAppsLookup(pids)
	if err != nil {
		t.Fatalf("apps large lookup failed: %v", err)
	}
	verifyLargeAppsLookupResponse(t, view, pids)
	if calls.Load() <= 1 {
		t.Fatalf("handler calls = %d, want multiple request fragments", calls.Load())
	}
	if maxSeen.Load() >= largeLookupU32(itemCount) {
		t.Fatalf("max fragment = %d, want less than logical item count %d", maxSeen.Load(), itemCount)
	}
}

func runUnixLargeCgroupsLookup(t *testing.T, service string, itemCount int) {
	t.Helper()

	paths := largeLookupPaths(itemCount)
	cfg := testServerConfig()
	cfg.MaxRequestPayloadBytes = lookupScaleRequestPayloadBytes
	cfg.MaxResponsePayloadBytes = responseBufSize
	var calls atomic.Uint32
	var maxSeen atomic.Uint32
	ts := startLookupTestServerWithConfig(
		service,
		protocol.MethodCgroupsLookup,
		CgroupsLookupDispatch(largeCgroupsLookupHandler(&calls, &maxSeen)),
		cfg,
		8,
	)
	defer ts.stop()

	ccfg := testClientConfig()
	ccfg.MaxRequestPayloadBytes = lookupScaleRequestPayloadBytes
	client := NewCgroupsLookupClient(testRunDir, service, ccfg)
	defer client.Close()
	client.SetCallTimeout(lookupScaleCallTimeoutMs)
	refreshUnixClientReady(t, client)

	view, err := client.CallCgroupsLookup(paths)
	if err != nil {
		t.Fatalf("cgroups large lookup failed: %v", err)
	}
	verifyLargeCgroupsLookupResponse(t, view, paths)
	if calls.Load() <= 1 {
		t.Fatalf("handler calls = %d, want multiple request fragments", calls.Load())
	}
	if maxSeen.Load() >= largeLookupU32(itemCount) {
		t.Fatalf("max fragment = %d, want less than logical item count %d", maxSeen.Load(), itemCount)
	}
}

func TestLookupLargeResponseSplit(t *testing.T) {
	t.Run("apps", func(t *testing.T) {
		runUnixLargeAppsResponseSplit(t, "go_svc_apps_lookup_large_response_split")
	})
	t.Run("cgroups", func(t *testing.T) {
		runUnixLargeCgroupsResponseSplit(t, "go_svc_cgroups_lookup_large_response_split")
	})
}

func runUnixLargeAppsResponseSplit(t *testing.T, service string) {
	t.Helper()

	pids := largeLookupPids(lookupResponseSplitScaleItems)
	cfg := testServerConfig()
	cfg.MaxRequestPayloadBytes = lookupResponseSplitRequestPayloadBytes
	cfg.MaxResponsePayloadBytes = lookupResponseSplitPayloadBytes
	var calls atomic.Uint32
	var maxSeen atomic.Uint32
	ts := startLookupTestServerWithConfig(
		service,
		protocol.MethodAppsLookup,
		AppsLookupDispatch(responseSplitAppsLookupHandler(&calls, &maxSeen)),
		cfg,
		8,
	)
	defer ts.stop()

	ccfg := testClientConfig()
	ccfg.MaxRequestPayloadBytes = lookupResponseSplitRequestPayloadBytes
	ccfg.MaxResponsePayloadBytes = lookupResponseSplitPayloadBytes
	client := NewAppsLookupClient(testRunDir, service, ccfg)
	defer client.Close()
	client.SetCallTimeout(lookupScaleCallTimeoutMs)
	refreshUnixClientReady(t, client)

	view, err := client.CallAppsLookup(pids)
	if err != nil {
		t.Fatalf("apps response-split lookup failed: %v", err)
	}
	verifyResponseSplitAppsLookupResponse(t, view, pids)
	if calls.Load() <= lookupResponseSplitMinCalls {
		t.Fatalf("handler calls = %d, want response-split retries", calls.Load())
	}
	if maxSeen.Load() != lookupResponseSplitScaleItems {
		t.Fatalf("max request items = %d, want one full %d-item request", maxSeen.Load(), lookupResponseSplitScaleItems)
	}
}

func runUnixLargeCgroupsResponseSplit(t *testing.T, service string) {
	t.Helper()

	paths := largeLookupPaths(lookupResponseSplitScaleItems)
	cfg := testServerConfig()
	cfg.MaxRequestPayloadBytes = lookupResponseSplitRequestPayloadBytes
	cfg.MaxResponsePayloadBytes = lookupResponseSplitPayloadBytes
	var calls atomic.Uint32
	var maxSeen atomic.Uint32
	ts := startLookupTestServerWithConfig(
		service,
		protocol.MethodCgroupsLookup,
		CgroupsLookupDispatch(responseSplitCgroupsLookupHandler(&calls, &maxSeen)),
		cfg,
		8,
	)
	defer ts.stop()

	ccfg := testClientConfig()
	ccfg.MaxRequestPayloadBytes = lookupResponseSplitRequestPayloadBytes
	ccfg.MaxResponsePayloadBytes = lookupResponseSplitPayloadBytes
	client := NewCgroupsLookupClient(testRunDir, service, ccfg)
	defer client.Close()
	client.SetCallTimeout(lookupScaleCallTimeoutMs)
	refreshUnixClientReady(t, client)

	view, err := client.CallCgroupsLookup(paths)
	if err != nil {
		t.Fatalf("cgroups response-split lookup failed: %v", err)
	}
	verifyResponseSplitCgroupsLookupResponse(t, view, paths)
	if calls.Load() <= lookupResponseSplitMinCalls {
		t.Fatalf("handler calls = %d, want response-split retries", calls.Load())
	}
	if maxSeen.Load() != lookupResponseSplitScaleItems {
		t.Fatalf("max request items = %d, want one full %d-item request", maxSeen.Load(), lookupResponseSplitScaleItems)
	}
}

func TestLookupLogicalLimits(t *testing.T) {
	t.Run("apps item limit", func(t *testing.T) {
		client := NewAppsLookupClient(testRunDir, "unused", testClientConfig())
		client.SetLookupLogicalConfig(LookupLogicalConfig{MaxItems: 2})
		if _, err := client.CallAppsLookup([]uint32{1, 2, 3}); err == nil {
			t.Fatal("expected logical item limit failure")
		}
	})

	t.Run("cgroups item limit", func(t *testing.T) {
		client := NewCgroupsLookupClient(testRunDir, "unused", testClientConfig())
		client.SetLookupLogicalConfig(LookupLogicalConfig{MaxItems: 1})
		if _, err := client.CallCgroupsLookup([][]byte{[]byte("/a"), []byte("/b")}); err == nil {
			t.Fatal("expected logical item limit failure")
		}
	})

	t.Run("apps response byte limit", func(t *testing.T) {
		svc := uniqueUnixService("go_svc_apps_lookup_response_limit")
		ts := startLookupTestServer(svc, protocol.MethodAppsLookup, AppsLookupDispatch(testAppsLookupHandler))
		defer ts.stop()

		client := NewAppsLookupClient(testRunDir, svc, testClientConfig())
		defer client.Close()
		refreshUnixClientReady(t, client)
		client.SetLookupLogicalConfig(LookupLogicalConfig{MaxResponseBytes: protocol.AppsLookupRespHdr + protocol.LookupDirEntrySize})
		if _, err := client.CallAppsLookup([]uint32{1234}); !errors.Is(err, protocol.ErrOverflow) {
			t.Fatalf("expected apps logical response overflow, got %v", err)
		}
	})

	t.Run("cgroups response byte limit", func(t *testing.T) {
		svc := uniqueUnixService("go_svc_cgroups_lookup_response_limit")
		ts := startLookupTestServer(svc, protocol.MethodCgroupsLookup, CgroupsLookupDispatch(testCgroupsLookupHandler))
		defer ts.stop()

		client := NewCgroupsLookupClient(testRunDir, svc, testClientConfig())
		defer client.Close()
		refreshUnixClientReady(t, client)
		client.SetLookupLogicalConfig(LookupLogicalConfig{MaxResponseBytes: protocol.CgroupsLookupRespHdr + protocol.LookupDirEntrySize})
		if _, err := client.CallCgroupsLookup([][]byte{[]byte("/known")}); !errors.Is(err, protocol.ErrOverflow) {
			t.Fatalf("expected cgroups logical response overflow, got %v", err)
		}
	})

	t.Run("apps subcall limit does not reconnect", func(t *testing.T) {
		svc := uniqueUnixService("go_svc_apps_lookup_logical_subcall_limit")
		cfg := testServerConfig()
		cfg.MaxRequestPayloadBytes = 48
		ts := startLookupTestServerWithConfig(svc, protocol.MethodAppsLookup, AppsLookupDispatch(testAppsLookupHandler), cfg, 8)
		defer ts.stop()

		ccfg := testClientConfig()
		ccfg.MaxRequestPayloadBytes = 48
		client := NewAppsLookupClient(testRunDir, svc, ccfg)
		refreshUnixClientReady(t, client)
		client.SetLookupLogicalConfig(LookupLogicalConfig{MaxSubcalls: 1})
		if _, err := client.CallAppsLookup([]uint32{1, 2, 3}); !errors.Is(err, protocol.ErrOverflow) {
			t.Fatalf("expected logical subcall overflow, got %v", err)
		}
		if got := client.Status().ReconnectCount; got != 0 {
			t.Fatalf("logical subcall limit reconnected %d times", got)
		}
		client.Close()
	})

	t.Run("cgroups subcall limit does not reconnect", func(t *testing.T) {
		svc := uniqueUnixService("go_svc_cgroups_lookup_logical_subcall_limit")
		cfg := testServerConfig()
		cfg.MaxRequestPayloadBytes = 64
		ts := startLookupTestServerWithConfig(svc, protocol.MethodCgroupsLookup, CgroupsLookupDispatch(testCgroupsLookupHandler), cfg, 8)
		defer ts.stop()

		ccfg := testClientConfig()
		ccfg.MaxRequestPayloadBytes = 64
		client := NewCgroupsLookupClient(testRunDir, svc, ccfg)
		refreshUnixClientReady(t, client)
		client.SetLookupLogicalConfig(LookupLogicalConfig{MaxSubcalls: 1})
		if _, err := client.CallCgroupsLookup([][]byte{[]byte("/known"), []byte("/missing"), []byte("/other")}); !errors.Is(err, protocol.ErrOverflow) {
			t.Fatalf("expected cgroups logical subcall overflow, got %v", err)
		}
		if got := client.Status().ReconnectCount; got != 0 {
			t.Fatalf("logical subcall limit reconnected %d times", got)
		}
		client.Close()
	})

	t.Run("cgroups oversized request key requires ready client", func(t *testing.T) {
		ccfg := testClientConfig()
		ccfg.MaxRequestPayloadBytes = 48
		client := NewCgroupsLookupClient(testRunDir, "unused", ccfg)
		_, err := client.CallCgroupsLookup([][]byte{[]byte("/request-key-too-large-for-configured-cap")})
		if !errors.Is(err, protocol.ErrBadLayout) {
			t.Fatalf("expected not-ready failure before local oversized response, got %v", err)
		}
		client.Close()
	})
}

func TestLookupRequestCapacityReconnectPaths(t *testing.T) {
	svc := uniqueUnixService("go_svc_lookup_request_capacity")
	cfg := testServerConfig()
	cfg.MaxRequestPayloadBytes = 256
	ts := startLookupTestServerWithConfig(svc, protocol.MethodAppsLookup, AppsLookupDispatch(testAppsLookupHandler), cfg, 8)
	defer ts.stop()

	ccfg := testClientConfig()
	ccfg.MaxRequestPayloadBytes = 256
	client := NewAppsLookupClient(testRunDir, svc, ccfg)
	defer client.Close()
	refreshUnixClientReady(t, client)
	if client.session == nil {
		t.Fatal("client session should be ready")
	}

	client.session.MaxRequestPayloadBytes = 64
	if err := client.ensureLookupRequestCapacity(128); err != nil {
		t.Fatalf("capacity reconnect should succeed: %v", err)
	}
	if client.state != StateReady {
		t.Fatalf("client state after capacity reconnect = %d, want ready", client.state)
	}
	if client.reconnectCount == 0 {
		t.Fatal("capacity reconnect did not increment reconnect count")
	}

	client.session.MaxRequestPayloadBytes = 256
	if err := client.ensureLookupRequestCapacity(128); err != nil {
		t.Fatalf("already-large session capacity = %v", err)
	}

	ts.stop()
	cleanupAll(svc)
	client.session.MaxRequestPayloadBytes = 64
	if err := client.ensureLookupRequestCapacity(128); !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("capacity reconnect to stopped service = %v, want ErrOverflow", err)
	}
	if client.state == StateReady {
		t.Fatal("client should not stay ready after failed capacity reconnect")
	}

	missing := NewAppsLookupClient(testRunDir, uniqueUnixService("go_svc_lookup_request_capacity_missing"), ccfg)
	defer missing.Close()
	missing.state = StateReady
	if err := missing.ensureLookupRequestCapacity(128); err != nil {
		t.Fatalf("sessionless configured capacity should not reconnect: %v", err)
	}
}

func TestLookupCallsReconnectForRequestCapacityGrowth(t *testing.T) {
	t.Run("apps", func(t *testing.T) {
		svc := uniqueUnixService("go_svc_apps_lookup_call_capacity")
		cfg := testServerConfig()
		cfg.MaxRequestPayloadBytes = 256
		ts := startLookupTestServerWithConfig(svc, protocol.MethodAppsLookup, AppsLookupDispatch(testAppsLookupHandler), cfg, 8)
		defer ts.stop()

		ccfg := testClientConfig()
		ccfg.MaxRequestPayloadBytes = 256
		client := NewAppsLookupClient(testRunDir, svc, ccfg)
		defer client.Close()
		refreshUnixClientReady(t, client)
		if client.session == nil {
			t.Fatal("client session should be ready")
		}
		client.session.MaxRequestPayloadBytes = 8

		view, err := client.CallAppsLookup([]uint32{1234})
		if err != nil {
			t.Fatalf("apps lookup capacity retry failed: %v", err)
		}
		if view.ItemCount != 1 {
			t.Fatalf("apps item count = %d, want 1", view.ItemCount)
		}
		if client.Status().ReconnectCount == 0 {
			t.Fatal("apps lookup capacity retry did not reconnect")
		}
	})

	t.Run("cgroups", func(t *testing.T) {
		svc := uniqueUnixService("go_svc_cgroups_lookup_call_capacity")
		cfg := testServerConfig()
		cfg.MaxRequestPayloadBytes = 256
		ts := startLookupTestServerWithConfig(svc, protocol.MethodCgroupsLookup, CgroupsLookupDispatch(testCgroupsLookupHandler), cfg, 8)
		defer ts.stop()

		ccfg := testClientConfig()
		ccfg.MaxRequestPayloadBytes = 256
		client := NewCgroupsLookupClient(testRunDir, svc, ccfg)
		defer client.Close()
		refreshUnixClientReady(t, client)
		if client.session == nil {
			t.Fatal("client session should be ready")
		}
		client.session.MaxRequestPayloadBytes = 8

		view, err := client.CallCgroupsLookup([][]byte{[]byte("/known")})
		if err != nil {
			t.Fatalf("cgroups lookup capacity retry failed: %v", err)
		}
		if view.ItemCount != 1 {
			t.Fatalf("cgroups item count = %d, want 1", view.ItemCount)
		}
		if client.Status().ReconnectCount == 0 {
			t.Fatal("cgroups lookup capacity retry did not reconnect")
		}
	})
}

func TestLookupTimeoutDuringFollowupSubcall(t *testing.T) {
	t.Run("apps", func(t *testing.T) {
		svc := uniqueUnixService("go_svc_apps_lookup_followup_timeout")
		cfg := testServerConfig()
		cfg.MaxResponsePayloadBytes = 320
		var calls atomic.Uint32
		ts := startLookupTestServerWithConfig(svc, protocol.MethodAppsLookup, AppsLookupDispatch(
			func(req *protocol.AppsLookupRequestView, builder *protocol.AppsLookupBuilder) bool {
				if calls.Add(1) == 2 {
					time.Sleep(150 * time.Millisecond)
				}
				builder.SetGeneration(9)
				for i := uint32(0); i < req.ItemCount; i++ {
					pid, err := req.Item(i)
					if err != nil {
						return false
					}
					cgroupPath := []byte("/ok")
					if pid == 22 {
						cgroupPath = append([]byte("/"), bytes.Repeat([]byte("x"), 1024)...)
					}
					if err := builder.Add(protocol.PidLookupKnown, protocol.AppsCgroupKnown, 0, pid, 0, 0, 1, []byte("ok"), cgroupPath, []byte("name"), nil); err != nil {
						return false
					}
				}
				return true
			},
		), cfg, 8)
		defer ts.stop()

		client := NewAppsLookupClient(testRunDir, svc, testClientConfig())
		defer client.Close()
		refreshUnixClientReady(t, client)

		_, err := client.CallAppsLookupWithTimeout([]uint32{11, 22, 33}, 30)
		if !errors.Is(err, protocol.ErrTimeout) {
			t.Fatalf("apps follow-up timeout error = %v, want ErrTimeout", err)
		}
		if calls.Load() < 2 {
			t.Fatalf("apps follow-up timeout calls = %d, want >= 2", calls.Load())
		}
	})

	t.Run("cgroups", func(t *testing.T) {
		svc := uniqueUnixService("go_svc_cgroups_lookup_followup_timeout")
		cfg := testServerConfig()
		cfg.MaxResponsePayloadBytes = 160
		var calls atomic.Uint32
		ts := startLookupTestServerWithConfig(svc, protocol.MethodCgroupsLookup, CgroupsLookupDispatch(
			func(req *protocol.CgroupsLookupRequestView, builder *protocol.CgroupsLookupBuilder) bool {
				if calls.Add(1) == 2 {
					time.Sleep(150 * time.Millisecond)
				}
				builder.SetGeneration(7)
				for i := uint32(0); i < req.ItemCount; i++ {
					path, err := req.Item(i)
					if err != nil {
						return false
					}
					name := []byte("ok")
					if path.String() == "/huge" {
						name = bytes.Repeat([]byte("x"), 512)
					}
					if err := builder.Add(protocol.CgroupLookupKnown, protocol.OrchestratorK8s, path.Bytes(), name, nil); err != nil {
						return false
					}
				}
				return true
			},
		), cfg, 8)
		defer ts.stop()

		client := NewCgroupsLookupClient(testRunDir, svc, testClientConfig())
		defer client.Close()
		refreshUnixClientReady(t, client)

		_, err := client.CallCgroupsLookupWithTimeout([][]byte{[]byte("/a"), []byte("/huge"), []byte("/b")}, 30)
		if !errors.Is(err, protocol.ErrTimeout) {
			t.Fatalf("cgroups follow-up timeout error = %v, want ErrTimeout", err)
		}
		if calls.Load() < 2 {
			t.Fatalf("cgroups follow-up timeout calls = %d, want >= 2", calls.Load())
		}
	})
}

func TestLookupAbortDuringFollowupSubcall(t *testing.T) {
	t.Run("apps", func(t *testing.T) {
		svc := uniqueUnixService("go_svc_apps_lookup_followup_abort")
		cfg := testServerConfig()
		cfg.MaxResponsePayloadBytes = 320
		var calls atomic.Uint32
		secondCall := make(chan struct{}, 1)
		ts := startLookupTestServerWithConfig(svc, protocol.MethodAppsLookup, AppsLookupDispatch(
			func(req *protocol.AppsLookupRequestView, builder *protocol.AppsLookupBuilder) bool {
				if calls.Add(1) == 2 {
					secondCall <- struct{}{}
					time.Sleep(250 * time.Millisecond)
				}
				builder.SetGeneration(9)
				for i := uint32(0); i < req.ItemCount; i++ {
					pid, err := req.Item(i)
					if err != nil {
						return false
					}
					cgroupPath := []byte("/ok")
					if pid == 22 {
						cgroupPath = append([]byte("/"), bytes.Repeat([]byte("x"), 1024)...)
					}
					if err := builder.Add(protocol.PidLookupKnown, protocol.AppsCgroupKnown, 0, pid, 0, 0, 1, []byte("ok"), cgroupPath, []byte("name"), nil); err != nil {
						return false
					}
				}
				return true
			},
		), cfg, 8)
		defer ts.stop()

		client := NewAppsLookupClient(testRunDir, svc, testClientConfig())
		defer client.Close()
		refreshUnixClientReady(t, client)

		errCh := make(chan error, 1)
		go func() {
			_, err := client.CallAppsLookupWithTimeout([]uint32{11, 22, 33}, 5000)
			errCh <- err
		}()

		select {
		case <-secondCall:
		case <-time.After(2 * time.Second):
			t.Fatal("apps follow-up subcall did not start")
		}
		client.Abort()

		select {
		case err := <-errCh:
			if !errors.Is(err, protocol.ErrAborted) {
				t.Fatalf("apps follow-up abort error = %v, want ErrAborted", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("apps follow-up abort did not return")
		}
	})

	t.Run("cgroups", func(t *testing.T) {
		svc := uniqueUnixService("go_svc_cgroups_lookup_followup_abort")
		cfg := testServerConfig()
		cfg.MaxResponsePayloadBytes = 160
		var calls atomic.Uint32
		secondCall := make(chan struct{}, 1)
		ts := startLookupTestServerWithConfig(svc, protocol.MethodCgroupsLookup, CgroupsLookupDispatch(
			func(req *protocol.CgroupsLookupRequestView, builder *protocol.CgroupsLookupBuilder) bool {
				if calls.Add(1) == 2 {
					secondCall <- struct{}{}
					time.Sleep(250 * time.Millisecond)
				}
				builder.SetGeneration(7)
				for i := uint32(0); i < req.ItemCount; i++ {
					path, err := req.Item(i)
					if err != nil {
						return false
					}
					name := []byte("ok")
					if path.String() == "/huge" {
						name = bytes.Repeat([]byte("x"), 512)
					}
					if err := builder.Add(protocol.CgroupLookupKnown, protocol.OrchestratorK8s, path.Bytes(), name, nil); err != nil {
						return false
					}
				}
				return true
			},
		), cfg, 8)
		defer ts.stop()

		client := NewCgroupsLookupClient(testRunDir, svc, testClientConfig())
		defer client.Close()
		refreshUnixClientReady(t, client)

		errCh := make(chan error, 1)
		go func() {
			_, err := client.CallCgroupsLookupWithTimeout([][]byte{[]byte("/a"), []byte("/huge"), []byte("/b")}, 5000)
			errCh <- err
		}()

		select {
		case <-secondCall:
		case <-time.After(2 * time.Second):
			t.Fatal("cgroups follow-up subcall did not start")
		}
		client.Abort()

		select {
		case err := <-errCh:
			if !errors.Is(err, protocol.ErrAborted) {
				t.Fatalf("cgroups follow-up abort error = %v, want ErrAborted", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("cgroups follow-up abort did not return")
		}
	})
}

func TestCgroupsLookupRejectsMixedGenerationRetry(t *testing.T) {
	svc := uniqueUnixService("go_svc_cgroups_lookup_generation")
	cfg := testServerConfig()
	cfg.MaxResponsePayloadBytes = 160
	var calls atomic.Uint32
	ts := startLookupTestServerWithConfig(svc, protocol.MethodCgroupsLookup, CgroupsLookupDispatch(
		func(req *protocol.CgroupsLookupRequestView, builder *protocol.CgroupsLookupBuilder) bool {
			generation := uint64(calls.Add(1))
			builder.SetGeneration(generation)
			for i := uint32(0); i < req.ItemCount; i++ {
				path, err := req.Item(i)
				if err != nil {
					return false
				}
				name := []byte("ok")
				if path.String() == "/huge" {
					name = bytes.Repeat([]byte("x"), 512)
				}
				if err := builder.Add(protocol.CgroupLookupKnown, protocol.OrchestratorK8s, path.Bytes(), name, nil); err != nil {
					return false
				}
			}
			return true
		},
	), cfg, 8)
	defer ts.stop()

	client := NewCgroupsLookupClient(testRunDir, svc, testClientConfig())
	defer client.Close()
	client.Refresh()
	if !client.Ready() {
		t.Fatal("client not ready")
	}
	if _, err := client.CallCgroupsLookup([][]byte{[]byte("/a"), []byte("/huge"), []byte("/b")}); err == nil {
		t.Fatal("expected mixed generation lookup to fail")
	}
	if calls.Load() < 2 {
		t.Fatalf("handler calls = %d, want at least 2", calls.Load())
	}
}

func TestAppsLookupRejectsMixedGenerationRetry(t *testing.T) {
	svc := uniqueUnixService("go_svc_apps_lookup_generation")
	cfg := testServerConfig()
	cfg.MaxResponsePayloadBytes = 320
	var calls atomic.Uint32
	ts := startLookupTestServerWithConfig(svc, protocol.MethodAppsLookup, AppsLookupDispatch(
		func(req *protocol.AppsLookupRequestView, builder *protocol.AppsLookupBuilder) bool {
			builder.SetGeneration(uint64(calls.Add(1)))
			for i := uint32(0); i < req.ItemCount; i++ {
				pid, err := req.Item(i)
				if err != nil {
					return false
				}
				cgroupPath := []byte("/ok")
				if pid == 22 {
					cgroupPath = append([]byte("/"), bytes.Repeat([]byte("x"), 1024)...)
				}
				if err := builder.Add(
					protocol.PidLookupKnown,
					protocol.AppsCgroupKnown,
					0,
					pid,
					0,
					0,
					1,
					[]byte("ok"),
					cgroupPath,
					[]byte("name"),
					nil,
				); err != nil {
					return false
				}
			}
			return true
		},
	), cfg, 8)
	defer ts.stop()

	client := NewAppsLookupClient(testRunDir, svc, testClientConfig())
	defer client.Close()
	client.Refresh()
	if !client.Ready() {
		t.Fatal("client not ready")
	}
	if _, err := client.CallAppsLookup([]uint32{11, 22, 33}); err == nil {
		t.Fatal("expected mixed generation lookup to fail")
	}
	if calls.Load() < 2 {
		t.Fatalf("handler calls = %d, want at least 2", calls.Load())
	}
}

func TestLookupHandlerFailures(t *testing.T) {
	t.Run("cgroups", func(t *testing.T) {
		svc := uniqueUnixService("go_svc_cgroups_lookup_fail")
		ts := startLookupTestServer(svc, protocol.MethodCgroupsLookup, CgroupsLookupDispatch(
			func(*protocol.CgroupsLookupRequestView, *protocol.CgroupsLookupBuilder) bool { return false },
		))
		defer ts.stop()

		client := NewCgroupsLookupClient(testRunDir, svc, testClientConfig())
		defer client.Close()
		client.Refresh()
		if !client.Ready() {
			t.Fatal("client not ready")
		}
		if _, err := client.CallCgroupsLookup([][]byte{[]byte("/known")}); err == nil {
			t.Fatal("expected cgroups lookup handler failure")
		}
	})

	t.Run("apps", func(t *testing.T) {
		svc := uniqueUnixService("go_svc_apps_lookup_fail")
		ts := startLookupTestServer(svc, protocol.MethodAppsLookup, AppsLookupDispatch(
			func(*protocol.AppsLookupRequestView, *protocol.AppsLookupBuilder) bool { return false },
		))
		defer ts.stop()

		client := NewAppsLookupClient(testRunDir, svc, testClientConfig())
		defer client.Close()
		client.Refresh()
		if !client.Ready() {
			t.Fatal("client not ready")
		}
		if _, err := client.CallAppsLookup([]uint32{1234}); err == nil {
			t.Fatal("expected apps lookup handler failure")
		}
	})
}

func TestLookupRejectsMalformedServerResponses(t *testing.T) {
	runApps := func(t *testing.T, name string, payload func([]byte) []byte, pids []uint32, want error) {
		t.Helper()
		svc := uniqueUnixService("go_svc_apps_lookup_bad_" + name)
		ts := startLookupTestServer(svc, protocol.MethodAppsLookup, func(_ []byte, responseBuf []byte) (int, error) {
			data := payload(responseBuf)
			return copy(responseBuf, data), nil
		})
		defer ts.stop()

		client := NewAppsLookupClient(testRunDir, svc, testClientConfig())
		defer client.Close()
		refreshUnixClientReady(t, client)
		if _, err := client.CallAppsLookup(pids); !errors.Is(err, want) {
			t.Fatalf("apps malformed response %s error = %v, want %v", name, err, want)
		}
	}

	runCgroups := func(t *testing.T, name string, payload func([]byte) []byte, paths [][]byte, want error) {
		t.Helper()
		svc := uniqueUnixService("go_svc_cgroups_lookup_bad_" + name)
		ts := startLookupTestServer(svc, protocol.MethodCgroupsLookup, func(_ []byte, responseBuf []byte) (int, error) {
			data := payload(responseBuf)
			return copy(responseBuf, data), nil
		})
		defer ts.stop()

		client := NewCgroupsLookupClient(testRunDir, svc, testClientConfig())
		defer client.Close()
		refreshUnixClientReady(t, client)
		if _, err := client.CallCgroupsLookup(paths); !errors.Is(err, want) {
			t.Fatalf("cgroups malformed response %s error = %v, want %v", name, err, want)
		}
	}

	t.Run("apps truncated response", func(t *testing.T) {
		runApps(t, "truncated", func([]byte) []byte {
			return []byte{1, 2, 3}
		}, []uint32{1234}, protocol.ErrTruncated)
	})

	t.Run("apps bad item count", func(t *testing.T) {
		runApps(t, "count", func(buf []byte) []byte {
			builder := protocol.NewAppsLookupBuilder(buf, 0, 1)
			return buf[:builder.Finish()]
		}, []uint32{1234}, protocol.ErrBadItemCount)
	})

	t.Run("apps wrong echoed pid", func(t *testing.T) {
		runApps(t, "pid", func(buf []byte) []byte {
			builder := protocol.NewAppsLookupBuilder(buf, 1, 1)
			if err := builder.Add(protocol.PidLookupUnknown, 0, 0, 9999, 0, protocol.NipcUIDUnset, 0, nil, nil, nil, nil); err != nil {
				t.Fatalf("build apps wrong pid response: %v", err)
			}
			return buf[:builder.Finish()]
		}, []uint32{1234}, protocol.ErrBadLayout)
	})

	t.Run("apps reordered response items", func(t *testing.T) {
		runApps(t, "reordered", func(buf []byte) []byte {
			builder := protocol.NewAppsLookupBuilder(buf, 2, 1)
			for _, pid := range []uint32{2, 1} {
				if err := builder.Add(protocol.PidLookupUnknown, 0, 0, pid, 0, protocol.NipcUIDUnset, 0, nil, nil, nil, nil); err != nil {
					t.Fatalf("build apps reordered response: %v", err)
				}
			}
			return buf[:builder.Finish()]
		}, []uint32{1, 2}, protocol.ErrBadLayout)
	})

	t.Run("apps duplicate response items", func(t *testing.T) {
		runApps(t, "duplicate", func(buf []byte) []byte {
			builder := protocol.NewAppsLookupBuilder(buf, 2, 1)
			for _, pid := range []uint32{1, 1} {
				if err := builder.Add(protocol.PidLookupUnknown, 0, 0, pid, 0, protocol.NipcUIDUnset, 0, nil, nil, nil, nil); err != nil {
					t.Fatalf("build apps duplicate response: %v", err)
				}
			}
			return buf[:builder.Finish()]
		}, []uint32{1, 2}, protocol.ErrBadLayout)
	})

	t.Run("apps invalid status enum", func(t *testing.T) {
		runApps(t, "bad_status", func(buf []byte) []byte {
			builder := protocol.NewAppsLookupBuilder(buf, 1, 1)
			if err := builder.Add(
				protocol.PidLookupKnown,
				protocol.AppsCgroupKnown,
				protocol.OrchestratorDocker,
				1234, 1, 1000, 42,
				[]byte("comm"), []byte("/cg"), []byte("name"),
				[]struct{ Key, Value []byte }{{Key: []byte("role"), Value: []byte("api")}},
			); err != nil {
				t.Fatalf("build apps invalid-status base response: %v", err)
			}
			payload := buf[:builder.Finish()]
			patchLookupResponseItemU16(t, payload, protocol.AppsLookupRespHdr, 1, 0, 2, 0xffff)
			return payload
		}, []uint32{1234}, protocol.ErrBadLayout)
	})

	t.Run("apps invalid status-dependent fields", func(t *testing.T) {
		runApps(t, "bad_status_fields", func(buf []byte) []byte {
			builder := protocol.NewAppsLookupBuilder(buf, 1, 1)
			if err := builder.Add(
				protocol.PidLookupKnown,
				protocol.AppsCgroupKnown,
				protocol.OrchestratorDocker,
				1234, 1, 1000, 42,
				[]byte("comm"), []byte("/cg"), []byte("name"),
				[]struct{ Key, Value []byte }{{Key: []byte("role"), Value: []byte("api")}},
			); err != nil {
				t.Fatalf("build apps invalid-status-fields base response: %v", err)
			}
			payload := buf[:builder.Finish()]
			patchLookupResponseItemU16(t, payload, protocol.AppsLookupRespHdr, 1, 0, 2, protocol.PidLookupUnknown)
			return payload
		}, []uint32{1234}, protocol.ErrBadLayout)
	})

	t.Run("apps invalid label table layout", func(t *testing.T) {
		runApps(t, "bad_label_table", func(buf []byte) []byte {
			builder := protocol.NewAppsLookupBuilder(buf, 1, 1)
			if err := builder.Add(
				protocol.PidLookupKnown,
				protocol.AppsCgroupKnown,
				protocol.OrchestratorDocker,
				1234, 1, 1000, 42,
				[]byte("comm"), []byte("/cg"), []byte("name"),
				[]struct{ Key, Value []byte }{{Key: []byte("role"), Value: []byte("api")}},
			); err != nil {
				t.Fatalf("build apps invalid-label-table base response: %v", err)
			}
			payload := buf[:builder.Finish()]
			patchLookupResponseItemU16(t, payload, protocol.AppsLookupRespHdr, 1, 0, 56, 2)
			return payload
		}, []uint32{1234}, protocol.ErrOutOfBounds)
	})

	t.Run("apps bad payload exceeded suffix", func(t *testing.T) {
		runApps(t, "suffix", func(buf []byte) []byte {
			builder := protocol.NewAppsLookupBuilder(buf, 2, 1)
			if err := builder.Add(protocol.PidLookupPayloadExceeded, 0, 0, 1, 0, protocol.NipcUIDUnset, 0, nil, nil, nil, nil); err != nil {
				t.Fatalf("build apps payload-exceeded item: %v", err)
			}
			if err := builder.Add(protocol.PidLookupUnknown, 0, 0, 2, 0, protocol.NipcUIDUnset, 0, nil, nil, nil, nil); err != nil {
				t.Fatalf("build apps bad suffix item: %v", err)
			}
			return buf[:builder.Finish()]
		}, []uint32{1, 2}, protocol.ErrBadLayout)
	})

	t.Run("apps payload exceeded at first item", func(t *testing.T) {
		runApps(t, "first_payload_exceeded", func(buf []byte) []byte {
			builder := protocol.NewAppsLookupBuilder(buf, 2, 1)
			for _, pid := range []uint32{1, 2} {
				if err := builder.Add(protocol.PidLookupPayloadExceeded, 0, 0, pid, 0, protocol.NipcUIDUnset, 0, nil, nil, nil, nil); err != nil {
					t.Fatalf("build apps payload-exceeded suffix: %v", err)
				}
			}
			return buf[:builder.Finish()]
		}, []uint32{1, 2}, protocol.ErrOverflow)
	})

	t.Run("cgroups truncated response", func(t *testing.T) {
		runCgroups(t, "truncated", func([]byte) []byte {
			return []byte{1, 2, 3}
		}, [][]byte{[]byte("/a")}, protocol.ErrTruncated)
	})

	t.Run("cgroups bad item count", func(t *testing.T) {
		runCgroups(t, "count", func(buf []byte) []byte {
			builder := protocol.NewCgroupsLookupBuilder(buf, 0, 1)
			return buf[:builder.Finish()]
		}, [][]byte{[]byte("/a")}, protocol.ErrBadItemCount)
	})

	t.Run("cgroups wrong echoed path", func(t *testing.T) {
		runCgroups(t, "path", func(buf []byte) []byte {
			builder := protocol.NewCgroupsLookupBuilder(buf, 1, 1)
			if err := builder.Add(protocol.CgroupLookupUnknownRetryLater, 0, []byte("/other"), nil, nil); err != nil {
				t.Fatalf("build cgroups wrong path response: %v", err)
			}
			return buf[:builder.Finish()]
		}, [][]byte{[]byte("/a")}, protocol.ErrBadLayout)
	})

	t.Run("cgroups reordered response items", func(t *testing.T) {
		runCgroups(t, "reordered", func(buf []byte) []byte {
			builder := protocol.NewCgroupsLookupBuilder(buf, 2, 1)
			for _, path := range [][]byte{[]byte("/b"), []byte("/a")} {
				if err := builder.Add(protocol.CgroupLookupUnknownRetryLater, 0, path, nil, nil); err != nil {
					t.Fatalf("build cgroups reordered response: %v", err)
				}
			}
			return buf[:builder.Finish()]
		}, [][]byte{[]byte("/a"), []byte("/b")}, protocol.ErrBadLayout)
	})

	t.Run("cgroups duplicate response items", func(t *testing.T) {
		runCgroups(t, "duplicate", func(buf []byte) []byte {
			builder := protocol.NewCgroupsLookupBuilder(buf, 2, 1)
			for _, path := range [][]byte{[]byte("/a"), []byte("/a")} {
				if err := builder.Add(protocol.CgroupLookupUnknownRetryLater, 0, path, nil, nil); err != nil {
					t.Fatalf("build cgroups duplicate response: %v", err)
				}
			}
			return buf[:builder.Finish()]
		}, [][]byte{[]byte("/a"), []byte("/b")}, protocol.ErrBadLayout)
	})

	t.Run("cgroups invalid status enum", func(t *testing.T) {
		runCgroups(t, "bad_status", func(buf []byte) []byte {
			builder := protocol.NewCgroupsLookupBuilder(buf, 1, 1)
			if err := builder.Add(
				protocol.CgroupLookupKnown,
				protocol.OrchestratorK8s,
				[]byte("/a"),
				[]byte("name"),
				[]struct{ Key, Value []byte }{{Key: []byte("role"), Value: []byte("db")}},
			); err != nil {
				t.Fatalf("build cgroups invalid-status base response: %v", err)
			}
			payload := buf[:builder.Finish()]
			patchLookupResponseItemU16(t, payload, protocol.CgroupsLookupRespHdr, 1, 0, 2, 0xffff)
			return payload
		}, [][]byte{[]byte("/a")}, protocol.ErrBadLayout)
	})

	t.Run("cgroups invalid status-dependent fields", func(t *testing.T) {
		runCgroups(t, "bad_status_fields", func(buf []byte) []byte {
			builder := protocol.NewCgroupsLookupBuilder(buf, 1, 1)
			if err := builder.Add(
				protocol.CgroupLookupKnown,
				protocol.OrchestratorK8s,
				[]byte("/a"),
				[]byte("name"),
				[]struct{ Key, Value []byte }{{Key: []byte("role"), Value: []byte("db")}},
			); err != nil {
				t.Fatalf("build cgroups invalid-status-fields base response: %v", err)
			}
			payload := buf[:builder.Finish()]
			patchLookupResponseItemU16(t, payload, protocol.CgroupsLookupRespHdr, 1, 0, 2, protocol.CgroupLookupUnknownRetryLater)
			return payload
		}, [][]byte{[]byte("/a")}, protocol.ErrBadLayout)
	})

	t.Run("cgroups invalid label table layout", func(t *testing.T) {
		runCgroups(t, "bad_label_table", func(buf []byte) []byte {
			builder := protocol.NewCgroupsLookupBuilder(buf, 1, 1)
			if err := builder.Add(
				protocol.CgroupLookupKnown,
				protocol.OrchestratorK8s,
				[]byte("/a"),
				[]byte("name"),
				[]struct{ Key, Value []byte }{{Key: []byte("role"), Value: []byte("db")}},
			); err != nil {
				t.Fatalf("build cgroups invalid-label-table base response: %v", err)
			}
			payload := buf[:builder.Finish()]
			patchLookupResponseItemU16(t, payload, protocol.CgroupsLookupRespHdr, 1, 0, 24, 2)
			return payload
		}, [][]byte{[]byte("/a")}, protocol.ErrOutOfBounds)
	})

	t.Run("cgroups bad payload exceeded suffix", func(t *testing.T) {
		runCgroups(t, "suffix", func(buf []byte) []byte {
			builder := protocol.NewCgroupsLookupBuilder(buf, 2, 1)
			if err := builder.Add(protocol.CgroupLookupPayloadExceeded, 0, []byte("/a"), nil, nil); err != nil {
				t.Fatalf("build cgroups payload-exceeded item: %v", err)
			}
			if err := builder.Add(protocol.CgroupLookupUnknownRetryLater, 0, []byte("/b"), nil, nil); err != nil {
				t.Fatalf("build cgroups bad suffix item: %v", err)
			}
			return buf[:builder.Finish()]
		}, [][]byte{[]byte("/a"), []byte("/b")}, protocol.ErrBadLayout)
	})

	t.Run("cgroups payload exceeded at first item", func(t *testing.T) {
		runCgroups(t, "first_payload_exceeded", func(buf []byte) []byte {
			builder := protocol.NewCgroupsLookupBuilder(buf, 2, 1)
			for _, path := range [][]byte{[]byte("/a"), []byte("/b")} {
				if err := builder.Add(protocol.CgroupLookupPayloadExceeded, 0, path, nil, nil); err != nil {
					t.Fatalf("build cgroups payload-exceeded suffix: %v", err)
				}
			}
			return buf[:builder.Finish()]
		}, [][]byte{[]byte("/a"), []byte("/b")}, protocol.ErrOverflow)
	})
}

func TestLookupRejectsMalformedFollowupResponse(t *testing.T) {
	t.Run("apps", func(t *testing.T) {
		svc := uniqueUnixService("go_svc_apps_lookup_bad_followup")
		var calls atomic.Uint32
		ts := startLookupTestServer(svc, protocol.MethodAppsLookup, AppsLookupDispatch(
			func(req *protocol.AppsLookupRequestView, builder *protocol.AppsLookupBuilder) bool {
				call := calls.Add(1)
				builder.SetGeneration(88)
				for i := uint32(0); i < req.ItemCount; i++ {
					pid, err := req.Item(i)
					if err != nil {
						return false
					}
					if call == 1 && i > 0 {
						if err := builder.Add(protocol.PidLookupPayloadExceeded, 0, 0, pid, 0, protocol.NipcUIDUnset, 0, nil, nil, nil, nil); err != nil {
							return false
						}
						continue
					}
					if call > 1 && i == 0 {
						if pid == 0 {
							pid = 1
						} else {
							pid = 0
						}
					}
					if err := builder.Add(protocol.PidLookupUnknown, 0, 0, pid, 0, protocol.NipcUIDUnset, 0, nil, nil, nil, nil); err != nil {
						return false
					}
				}
				return true
			},
		))
		defer ts.stop()

		client := NewAppsLookupClient(testRunDir, svc, testClientConfig())
		defer client.Close()
		refreshUnixClientReady(t, client)

		if _, err := client.CallAppsLookup([]uint32{11, 22, 33}); !errors.Is(err, protocol.ErrBadLayout) {
			t.Fatalf("apps malformed follow-up error = %v, want ErrBadLayout", err)
		}
		if calls.Load() < 2 {
			t.Fatalf("apps malformed follow-up calls = %d, want >= 2", calls.Load())
		}
	})

	t.Run("cgroups", func(t *testing.T) {
		svc := uniqueUnixService("go_svc_cgroups_lookup_bad_followup")
		var calls atomic.Uint32
		ts := startLookupTestServer(svc, protocol.MethodCgroupsLookup, CgroupsLookupDispatch(
			func(req *protocol.CgroupsLookupRequestView, builder *protocol.CgroupsLookupBuilder) bool {
				call := calls.Add(1)
				builder.SetGeneration(77)
				for i := uint32(0); i < req.ItemCount; i++ {
					path, err := req.Item(i)
					if err != nil {
						return false
					}
					if call == 1 && i > 0 {
						if err := builder.Add(protocol.CgroupLookupPayloadExceeded, 0, path.Bytes(), nil, nil); err != nil {
							return false
						}
						continue
					}
					echoPath := path.Bytes()
					if call > 1 && i == 0 {
						echoPath = []byte("/wrong")
					}
					if err := builder.Add(protocol.CgroupLookupUnknownRetryLater, 0, echoPath, nil, nil); err != nil {
						return false
					}
				}
				return true
			},
		))
		defer ts.stop()

		client := NewCgroupsLookupClient(testRunDir, svc, testClientConfig())
		defer client.Close()
		refreshUnixClientReady(t, client)

		if _, err := client.CallCgroupsLookup([][]byte{[]byte("/a"), []byte("/b"), []byte("/c")}); !errors.Is(err, protocol.ErrBadLayout) {
			t.Fatalf("cgroups malformed follow-up error = %v, want ErrBadLayout", err)
		}
		if calls.Load() < 2 {
			t.Fatalf("cgroups malformed follow-up calls = %d, want >= 2", calls.Load())
		}
	})
}

func unixAppsLookupPartialResponse(payload []byte) ([]byte, uint32, error) {
	req, err := protocol.DecodeAppsLookupRequest(payload)
	if err != nil {
		return nil, 0, err
	}
	resp := make([]byte, 1024)
	builder := protocol.NewAppsLookupBuilder(resp, req.ItemCount, 88)
	for i := uint32(0); i < req.ItemCount; i++ {
		pid, err := req.Item(i)
		if err != nil {
			return nil, 0, err
		}
		status := protocol.PidLookupUnknown
		if i > 0 {
			status = protocol.PidLookupPayloadExceeded
		}
		if err := builder.Add(status, 0, 0, pid, 0, protocol.NipcUIDUnset, 0, nil, nil, nil, nil); err != nil {
			return nil, 0, err
		}
	}
	n := builder.Finish()
	if n == 0 {
		return nil, 0, builder.Error()
	}
	return resp[:n], req.ItemCount, nil
}

func unixCgroupsLookupPartialResponse(payload []byte) ([]byte, uint32, error) {
	req, err := protocol.DecodeCgroupsLookupRequest(payload)
	if err != nil {
		return nil, 0, err
	}
	resp := make([]byte, 1024)
	builder := protocol.NewCgroupsLookupBuilder(resp, req.ItemCount, 77)
	for i := uint32(0); i < req.ItemCount; i++ {
		path, err := req.Item(i)
		if err != nil {
			return nil, 0, err
		}
		status := protocol.CgroupLookupKnown
		orchestrator := protocol.OrchestratorK8s
		name := []byte("ok")
		if i > 0 {
			status = protocol.CgroupLookupPayloadExceeded
			orchestrator = 0
			name = nil
		}
		if err := builder.Add(status, orchestrator, path.Bytes(), name, nil); err != nil {
			return nil, 0, err
		}
	}
	n := builder.Finish()
	if n == 0 {
		return nil, 0, builder.Error()
	}
	return resp[:n], req.ItemCount, nil
}

func TestLookupEndpointGoneAfterPartialProgress(t *testing.T) {
	t.Run("apps", func(t *testing.T) {
		svc := uniqueUnixService("go_unix_apps_lookup_partial_disconnect")
		srv := startRawPosixSessionServer(t, svc, testServerConfig(),
			func(session *posix.Session, hdr protocol.Header, payload []byte) error {
				if hdr.Kind != protocol.KindRequest || hdr.Code != protocol.MethodAppsLookup {
					return fmt.Errorf("unexpected request header: %+v", hdr)
				}
				respPayload, _, err := unixAppsLookupPartialResponse(payload)
				if err != nil {
					return err
				}
				respHdr := protocol.Header{
					Kind:            protocol.KindResponse,
					Code:            protocol.MethodAppsLookup,
					ItemCount:       1,
					MessageID:       hdr.MessageID,
					TransportStatus: protocol.StatusOK,
				}
				return session.Send(&respHdr, respPayload)
			})

		client := NewAppsLookupClient(testRunDir, svc, testClientConfig())
		defer client.Close()
		refreshUnixClientReady(t, client)

		if _, err := client.CallAppsLookupWithTimeout([]uint32{11, 22, 33}, 1000); err == nil {
			t.Fatal("apps lookup should fail when endpoint disappears after partial progress")
		}
		srv.wait(t)
		cleanupAll(svc)
	})

	t.Run("cgroups", func(t *testing.T) {
		svc := uniqueUnixService("go_unix_cgroups_lookup_partial_disconnect")
		srv := startRawPosixSessionServer(t, svc, testServerConfig(),
			func(session *posix.Session, hdr protocol.Header, payload []byte) error {
				if hdr.Kind != protocol.KindRequest || hdr.Code != protocol.MethodCgroupsLookup {
					return fmt.Errorf("unexpected request header: %+v", hdr)
				}
				respPayload, _, err := unixCgroupsLookupPartialResponse(payload)
				if err != nil {
					return err
				}
				respHdr := protocol.Header{
					Kind:            protocol.KindResponse,
					Code:            protocol.MethodCgroupsLookup,
					ItemCount:       1,
					MessageID:       hdr.MessageID,
					TransportStatus: protocol.StatusOK,
				}
				return session.Send(&respHdr, respPayload)
			})

		client := NewCgroupsLookupClient(testRunDir, svc, testClientConfig())
		defer client.Close()
		refreshUnixClientReady(t, client)

		paths := [][]byte{[]byte("/a"), []byte("/b"), []byte("/c")}
		if _, err := client.CallCgroupsLookupWithTimeout(paths, 1000); err == nil {
			t.Fatal("cgroups lookup should fail when endpoint disappears after partial progress")
		}
		srv.wait(t)
		cleanupAll(svc)
	})
}

func TestLookupEndpointGoneBeforeFirstSubcall(t *testing.T) {
	t.Run("apps", func(t *testing.T) {
		svc := uniqueUnixService("go_unix_apps_lookup_gone_before_call")
		ts := startLookupTestServer(svc, protocol.MethodAppsLookup, AppsLookupDispatch(testAppsLookupHandler))

		client := NewAppsLookupClient(testRunDir, svc, testClientConfig())
		defer client.Close()
		refreshUnixClientReady(t, client)
		ts.stop()

		if _, err := client.CallAppsLookupWithTimeout([]uint32{11, 22}, 1000); err == nil {
			t.Fatal("apps lookup should fail when endpoint disappears before first subcall")
		}
		cleanupAll(svc)
	})

	t.Run("cgroups", func(t *testing.T) {
		svc := uniqueUnixService("go_unix_cgroups_lookup_gone_before_call")
		ts := startLookupTestServer(svc, protocol.MethodCgroupsLookup, CgroupsLookupDispatch(testCgroupsLookupHandler))

		client := NewCgroupsLookupClient(testRunDir, svc, testClientConfig())
		defer client.Close()
		refreshUnixClientReady(t, client)
		ts.stop()

		if _, err := client.CallCgroupsLookupWithTimeout([][]byte{[]byte("/a"), []byte("/b")}, 1000); err == nil {
			t.Fatal("cgroups lookup should fail when endpoint disappears before first subcall")
		}
		cleanupAll(svc)
	})
}

func TestLookupEndpointAbsentBeforeCall(t *testing.T) {
	t.Run("apps", func(t *testing.T) {
		svc := uniqueUnixService("go_unix_apps_lookup_absent")
		cleanupAll(svc)
		defer cleanupAll(svc)

		client := NewAppsLookupClient(testRunDir, svc, testClientConfig())
		defer client.Close()
		if !client.Refresh() {
			t.Fatal("apps absent refresh did not change state")
		}
		if client.state != StateNotFound {
			t.Fatalf("apps absent state = %d, want StateNotFound", client.state)
		}
		if _, err := client.CallAppsLookupWithTimeout([]uint32{11, 22}, 1000); !errors.Is(err, protocol.ErrBadLayout) {
			t.Fatalf("apps absent lookup error = %v, want ErrBadLayout", err)
		}
	})

	t.Run("cgroups", func(t *testing.T) {
		svc := uniqueUnixService("go_unix_cgroups_lookup_absent")
		cleanupAll(svc)
		defer cleanupAll(svc)

		client := NewCgroupsLookupClient(testRunDir, svc, testClientConfig())
		defer client.Close()
		if !client.Refresh() {
			t.Fatal("cgroups absent refresh did not change state")
		}
		if client.state != StateNotFound {
			t.Fatalf("cgroups absent state = %d, want StateNotFound", client.state)
		}
		paths := [][]byte{[]byte("/a"), []byte("/b")}
		if _, err := client.CallCgroupsLookupWithTimeout(paths, 1000); !errors.Is(err, protocol.ErrBadLayout) {
			t.Fatalf("cgroups absent lookup error = %v, want ErrBadLayout", err)
		}
	})
}

func TestCgroupsLookupRejectsInvalidRequestPathOnReadyClient(t *testing.T) {
	svc := uniqueUnixService("go_svc_cgroups_lookup_bad_request_path")
	ts := startLookupTestServer(svc, protocol.MethodCgroupsLookup, CgroupsLookupDispatch(testCgroupsLookupHandler))
	defer ts.stop()

	client := NewCgroupsLookupClient(testRunDir, svc, testClientConfig())
	defer client.Close()
	refreshUnixClientReady(t, client)

	if _, err := client.CallCgroupsLookup([][]byte{[]byte("bad\x00path")}); !errors.Is(err, protocol.ErrBadLayout) {
		t.Fatalf("invalid cgroups request path error = %v, want ErrBadLayout", err)
	}
}

func TestCgroupsLookupRetryOnFailure(t *testing.T) {
	svc := uniqueUnixService("go_svc_cgroups_lookup_retry")
	ts1 := startLookupTestServer(svc, protocol.MethodCgroupsLookup, CgroupsLookupDispatch(testCgroupsLookupHandler))

	client := NewCgroupsLookupClient(testRunDir, svc, testClientConfig())
	defer client.Close()
	client.Refresh()
	if !client.Ready() {
		t.Fatal("client not ready")
	}

	view, err := client.CallCgroupsLookup([][]byte{[]byte("/known"), []byte("/missing")})
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	verifyCgroupsLookupView(t, view)

	ts1.stop()
	cleanupAll(svc)
	time.Sleep(50 * time.Millisecond)

	ts2 := startLookupTestServer(svc, protocol.MethodCgroupsLookup, CgroupsLookupDispatch(testCgroupsLookupHandler))
	defer ts2.stop()

	view, err = client.CallCgroupsLookup([][]byte{[]byte("/known"), []byte("/missing")})
	if err != nil {
		t.Fatalf("retry call failed: %v", err)
	}
	verifyCgroupsLookupView(t, view)
	if client.Status().ReconnectCount < 1 {
		t.Fatalf("expected reconnect_count >= 1, got %d", client.Status().ReconnectCount)
	}
}

func TestLookupConcurrentClients(t *testing.T) {
	t.Run("cgroups", func(t *testing.T) {
		svc := uniqueUnixService("go_svc_cgroups_lookup_concurrent")
		ts := startLookupTestServerWithWorkers(
			svc,
			protocol.MethodCgroupsLookup,
			CgroupsLookupDispatch(testCgroupsLookupHandler),
			16,
		)
		defer ts.stop()

		const clients = 16
		const callsPerClient = 10
		var wg sync.WaitGroup
		errs := make(chan error, clients*callsPerClient)
		for range clients {
			wg.Go(func() {
				client := NewCgroupsLookupClient(testRunDir, svc, testClientConfig())
				defer client.Close()
				for range 200 {
					client.Refresh()
					if client.Ready() {
						break
					}
					time.Sleep(5 * time.Millisecond)
				}
				if !client.Ready() {
					errs <- fmt.Errorf("client not ready")
					return
				}
				for range callsPerClient {
					view, err := client.CallCgroupsLookup([][]byte{[]byte("/known"), []byte("/missing")})
					if err != nil {
						errs <- err
						return
					}
					if err := checkCgroupsLookupView(view); err != nil {
						errs <- err
						return
					}
				}
			})
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Fatalf("concurrent cgroups lookup failed: %v", err)
		}
	})

	t.Run("apps", func(t *testing.T) {
		svc := uniqueUnixService("go_svc_apps_lookup_concurrent")
		ts := startLookupTestServerWithWorkers(
			svc,
			protocol.MethodAppsLookup,
			AppsLookupDispatch(testAppsLookupHandler),
			16,
		)
		defer ts.stop()

		const clients = 16
		const callsPerClient = 10
		var wg sync.WaitGroup
		errs := make(chan error, clients*callsPerClient)
		for range clients {
			wg.Go(func() {
				client := NewAppsLookupClient(testRunDir, svc, testClientConfig())
				defer client.Close()
				for range 200 {
					client.Refresh()
					if client.Ready() {
						break
					}
					time.Sleep(5 * time.Millisecond)
				}
				if !client.Ready() {
					errs <- fmt.Errorf("client not ready")
					return
				}
				for range callsPerClient {
					view, err := client.CallAppsLookup([]uint32{1234, 0, 9999})
					if err != nil {
						errs <- err
						return
					}
					if err := checkAppsLookupView(view); err != nil {
						errs <- err
						return
					}
				}
			})
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Fatalf("concurrent apps lookup failed: %v", err)
		}
	})
}
