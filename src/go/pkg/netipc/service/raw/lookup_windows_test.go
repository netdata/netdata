//go:build windows

package raw

import (
	"bytes"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	windows "github.com/netdata/netdata/go/plugins/pkg/netipc/transport/windows"
)

func winTestCgroupsLookupHandler(req *protocol.CgroupsLookupRequestView, builder *protocol.CgroupsLookupBuilder) bool {
	for i := uint32(0); i < req.ItemCount; i++ {
		path, err := req.Item(i)
		if err != nil {
			return false
		}
		if path.String() == "/known" {
			if err := builder.Add(protocol.CgroupLookupKnown, protocol.OrchestratorK8s, path.Bytes(), []byte("pod-a"), nil); err != nil {
				return false
			}
			continue
		}
		if err := builder.Add(protocol.CgroupLookupUnknownRetryLater, 0, path.Bytes(), nil, nil); err != nil {
			return false
		}
	}
	return true
}

func winTestAppsLookupHandler(req *protocol.AppsLookupRequestView, builder *protocol.AppsLookupBuilder) bool {
	for i := uint32(0); i < req.ItemCount; i++ {
		pid, err := req.Item(i)
		if err != nil {
			return false
		}
		if pid != 1234 {
			if err := builder.Add(protocol.PidLookupUnknown, 0, 0, pid, 0, protocol.NipcUIDUnset, 0, nil, nil, nil, nil); err != nil {
				return false
			}
			continue
		}
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
			nil,
		); err != nil {
			return false
		}
	}
	return true
}

func TestWinCgroupsLookupTransparentPayloadExceededRetry(t *testing.T) {
	svc := uniqueWinService("go_win_cgroups_lookup_scale")
	cfg := testWinServerConfig()
	cfg.MaxResponsePayloadBytes = 256
	var calls atomic.Uint32

	ts := startTestServerWinWithConfig(svc, cfg, protocol.MethodCgroupsLookup, CgroupsLookupDispatch(
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
	))
	defer ts.stop()

	client := NewCgroupsLookupClient(winTestRunDir, svc, testWinClientConfig())
	defer client.Close()
	waitWinClientReady(t, client)

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

func TestWinAppsLookupTransparentPayloadExceededRetry(t *testing.T) {
	svc := uniqueWinService("go_win_apps_lookup_scale")
	cfg := testWinServerConfig()
	cfg.MaxResponsePayloadBytes = 320
	var calls atomic.Uint32

	ts := startTestServerWinWithConfig(svc, cfg, protocol.MethodAppsLookup, AppsLookupDispatch(
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
	))
	defer ts.stop()

	client := NewAppsLookupClient(winTestRunDir, svc, testWinClientConfig())
	defer client.Close()
	waitWinClientReady(t, client)

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

func TestWinLookupZeroItemCalls(t *testing.T) {
	t.Run("apps", func(t *testing.T) {
		svc := uniqueWinService("go_win_apps_lookup_zero")
		ts := startTestServerWin(svc, protocol.MethodAppsLookup, AppsLookupDispatch(winTestAppsLookupHandler))
		defer ts.stop()

		client := NewAppsLookupClient(winTestRunDir, svc, testWinClientConfig())
		defer client.Close()
		waitWinClientReady(t, client)

		view, err := client.CallAppsLookup(nil)
		if err != nil {
			t.Fatalf("zero-item apps lookup: %v", err)
		}
		if view.ItemCount != 0 || view.Generation != 0 {
			t.Fatalf("zero-item apps response = count %d generation %d, want 0/0", view.ItemCount, view.Generation)
		}
	})

	t.Run("cgroups", func(t *testing.T) {
		svc := uniqueWinService("go_win_cgroups_lookup_zero")
		ts := startTestServerWin(svc, protocol.MethodCgroupsLookup, CgroupsLookupDispatch(winTestCgroupsLookupHandler))
		defer ts.stop()

		client := NewCgroupsLookupClient(winTestRunDir, svc, testWinClientConfig())
		defer client.Close()
		waitWinClientReady(t, client)

		view, err := client.CallCgroupsLookup(nil)
		if err != nil {
			t.Fatalf("zero-item cgroups lookup: %v", err)
		}
		if view.ItemCount != 0 || view.Generation != 0 {
			t.Fatalf("zero-item cgroups response = count %d generation %d, want 0/0", view.ItemCount, view.Generation)
		}
	})
}

func runWinAppsLookupRequestBoundary(t *testing.T, name string, requestCap, expectedMaxItems, minCalls uint32) {
	t.Helper()

	svc := uniqueWinService("go_win_apps_lookup_request_split_" + name)
	scfg := testWinServerConfig()
	scfg.MaxRequestPayloadBytes = requestCap
	scfg.MaxResponsePayloadBytes = 4096
	var calls atomic.Uint32
	var maxSeen atomic.Uint32
	ts := startTestServerWinWithConfig(svc, scfg, protocol.MethodAppsLookup, AppsLookupDispatch(
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
	))
	defer ts.stop()

	ccfg := testWinClientConfig()
	ccfg.MaxRequestPayloadBytes = requestCap
	client := NewAppsLookupClient(winTestRunDir, svc, ccfg)
	defer client.Close()
	waitWinClientReady(t, client)

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

func runWinCgroupsLookupRequestBoundary(t *testing.T, name string, requestCap, expectedMaxItems, minCalls uint32) {
	t.Helper()

	svc := uniqueWinService("go_win_cgroups_lookup_request_split_" + name)
	scfg := testWinServerConfig()
	scfg.MaxRequestPayloadBytes = requestCap
	scfg.MaxResponsePayloadBytes = 4096
	var calls atomic.Uint32
	var maxSeen atomic.Uint32
	ts := startTestServerWinWithConfig(svc, scfg, protocol.MethodCgroupsLookup, CgroupsLookupDispatch(
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
	))
	defer ts.stop()

	ccfg := testWinClientConfig()
	ccfg.MaxRequestPayloadBytes = requestCap
	client := NewCgroupsLookupClient(winTestRunDir, svc, ccfg)
	defer client.Close()
	waitWinClientReady(t, client)

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

func TestWinLookupProactiveRequestSplitAndLimits(t *testing.T) {
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
			runWinAppsLookupRequestBoundary(t, tc.name, tc.requestCap, tc.expectedMaxItems, tc.minCalls)
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
			runWinCgroupsLookupRequestBoundary(t, tc.name, tc.requestCap, tc.expectedMaxItems, tc.minCalls)
		})
	}

	t.Run("cgroups oversized request key", func(t *testing.T) {
		svc := uniqueWinService("go_win_cgroups_lookup_oversized_request_key")
		scfg := testWinServerConfig()
		scfg.MaxRequestPayloadBytes = 48
		scfg.MaxResponsePayloadBytes = 4096
		var calls atomic.Uint32
		var maxSeen atomic.Uint32
		ts := startTestServerWinWithConfig(svc, scfg, protocol.MethodCgroupsLookup, CgroupsLookupDispatch(
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
		))
		defer ts.stop()

		ccfg := testWinClientConfig()
		ccfg.MaxRequestPayloadBytes = 48
		client := NewCgroupsLookupClient(winTestRunDir, svc, ccfg)
		defer client.Close()
		waitWinClientReady(t, client)

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

	t.Run("logical limit", func(t *testing.T) {
		client := NewAppsLookupClient(winTestRunDir, "unused", testWinClientConfig())
		client.SetLookupLogicalConfig(LookupLogicalConfig{MaxItems: 2})
		if _, err := client.CallAppsLookup([]uint32{1, 2, 3}); err == nil {
			t.Fatal("expected logical item limit failure")
		}
	})

	t.Run("apps subcall limit does not reconnect", func(t *testing.T) {
		svc := uniqueWinService("go_win_apps_lookup_logical_subcall_limit")
		scfg := testWinServerConfig()
		scfg.MaxRequestPayloadBytes = 48
		ts := startTestServerWinWithConfig(svc, scfg, protocol.MethodAppsLookup, AppsLookupDispatch(
			func(req *protocol.AppsLookupRequestView, builder *protocol.AppsLookupBuilder) bool {
				builder.SetGeneration(9)
				for i := uint32(0); i < req.ItemCount; i++ {
					pid, err := req.Item(i)
					if err != nil {
						return false
					}
					if err := builder.Add(protocol.PidLookupUnknown, protocol.AppsCgroupKnown, 0, pid, 0, protocol.NipcUIDUnset, 0, nil, nil, nil, nil); err != nil {
						return false
					}
				}
				return true
			},
		))
		defer ts.stop()

		ccfg := testWinClientConfig()
		ccfg.MaxRequestPayloadBytes = 48
		client := NewAppsLookupClient(winTestRunDir, svc, ccfg)
		waitWinClientReady(t, client)
		client.SetLookupLogicalConfig(LookupLogicalConfig{MaxSubcalls: 1})
		if _, err := client.CallAppsLookup([]uint32{1, 2, 3}); !errors.Is(err, protocol.ErrOverflow) {
			t.Fatalf("expected logical subcall overflow, got %v", err)
		}
		if got := client.Status().ReconnectCount; got != 0 {
			t.Fatalf("logical subcall limit reconnected %d times", got)
		}
		client.Close()
	})

	t.Run("cgroups oversized request key requires ready client", func(t *testing.T) {
		ccfg := testWinClientConfig()
		ccfg.MaxRequestPayloadBytes = 48
		client := NewCgroupsLookupClient(winTestRunDir, "unused", ccfg)
		_, err := client.CallCgroupsLookup([][]byte{[]byte("/request-key-too-large-for-configured-cap")})
		if !errors.Is(err, protocol.ErrBadLayout) {
			t.Fatalf("expected not-ready failure before local oversized response, got %v", err)
		}
		client.Close()
	})
}

func TestWinLookupLargeLogicalCalls(t *testing.T) {
	t.Run("apps 8192", func(t *testing.T) {
		runWinLargeAppsLookup(t, "go_win_apps_lookup_large_8192", lookupTopologyScaleItems)
	})
	t.Run("apps 32768", func(t *testing.T) {
		runWinLargeAppsLookup(t, "go_win_apps_lookup_large_32768", lookupHPCScaleItems)
	})
	t.Run("cgroups 8192", func(t *testing.T) {
		runWinLargeCgroupsLookup(t, "go_win_cgroups_lookup_large_8192", lookupTopologyScaleItems)
	})
	t.Run("cgroups 32768", func(t *testing.T) {
		runWinLargeCgroupsLookup(t, "go_win_cgroups_lookup_large_32768", lookupHPCScaleItems)
	})
}

func runWinLargeAppsLookup(t *testing.T, service string, itemCount int) {
	t.Helper()

	pids := largeLookupPids(itemCount)
	cfg := testWinServerConfig()
	cfg.MaxRequestPayloadBytes = lookupScaleRequestPayloadBytes
	cfg.MaxResponsePayloadBytes = winResponseBufSize
	var calls atomic.Uint32
	var maxSeen atomic.Uint32
	ts := startTestServerWinWithConfig(
		service,
		cfg,
		protocol.MethodAppsLookup,
		AppsLookupDispatch(largeAppsLookupHandler(&calls, &maxSeen)),
	)
	defer ts.stop()

	ccfg := testWinClientConfig()
	ccfg.MaxRequestPayloadBytes = lookupScaleRequestPayloadBytes
	client := NewAppsLookupClient(winTestRunDir, service, ccfg)
	defer client.Close()
	client.SetCallTimeout(lookupScaleCallTimeoutMs)
	waitWinClientReady(t, client)

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

func runWinLargeCgroupsLookup(t *testing.T, service string, itemCount int) {
	t.Helper()

	paths := largeLookupPaths(itemCount)
	cfg := testWinServerConfig()
	cfg.MaxRequestPayloadBytes = lookupScaleRequestPayloadBytes
	cfg.MaxResponsePayloadBytes = winResponseBufSize
	var calls atomic.Uint32
	var maxSeen atomic.Uint32
	ts := startTestServerWinWithConfig(
		service,
		cfg,
		protocol.MethodCgroupsLookup,
		CgroupsLookupDispatch(largeCgroupsLookupHandler(&calls, &maxSeen)),
	)
	defer ts.stop()

	ccfg := testWinClientConfig()
	ccfg.MaxRequestPayloadBytes = lookupScaleRequestPayloadBytes
	client := NewCgroupsLookupClient(winTestRunDir, service, ccfg)
	defer client.Close()
	client.SetCallTimeout(lookupScaleCallTimeoutMs)
	waitWinClientReady(t, client)

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

func TestWinLookupLargeResponseSplit(t *testing.T) {
	t.Run("apps", func(t *testing.T) {
		runWinLargeAppsResponseSplit(t, "go_win_apps_lookup_large_response_split")
	})
	t.Run("cgroups", func(t *testing.T) {
		runWinLargeCgroupsResponseSplit(t, "go_win_cgroups_lookup_large_response_split")
	})
}

func runWinLargeAppsResponseSplit(t *testing.T, service string) {
	t.Helper()

	pids := largeLookupPids(lookupResponseSplitScaleItems)
	cfg := testWinServerConfig()
	cfg.MaxRequestPayloadBytes = lookupResponseSplitRequestPayloadBytes
	cfg.MaxResponsePayloadBytes = lookupResponseSplitPayloadBytes
	var calls atomic.Uint32
	var maxSeen atomic.Uint32
	ts := startTestServerWinWithConfig(
		service,
		cfg,
		protocol.MethodAppsLookup,
		AppsLookupDispatch(responseSplitAppsLookupHandler(&calls, &maxSeen)),
	)
	defer ts.stop()

	ccfg := testWinClientConfig()
	ccfg.MaxRequestPayloadBytes = lookupResponseSplitRequestPayloadBytes
	ccfg.MaxResponsePayloadBytes = lookupResponseSplitPayloadBytes
	client := NewAppsLookupClient(winTestRunDir, service, ccfg)
	defer client.Close()
	client.SetCallTimeout(lookupScaleCallTimeoutMs)
	waitWinClientReady(t, client)

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

func runWinLargeCgroupsResponseSplit(t *testing.T, service string) {
	t.Helper()

	paths := largeLookupPaths(lookupResponseSplitScaleItems)
	cfg := testWinServerConfig()
	cfg.MaxRequestPayloadBytes = lookupResponseSplitRequestPayloadBytes
	cfg.MaxResponsePayloadBytes = lookupResponseSplitPayloadBytes
	var calls atomic.Uint32
	var maxSeen atomic.Uint32
	ts := startTestServerWinWithConfig(
		service,
		cfg,
		protocol.MethodCgroupsLookup,
		CgroupsLookupDispatch(responseSplitCgroupsLookupHandler(&calls, &maxSeen)),
	)
	defer ts.stop()

	ccfg := testWinClientConfig()
	ccfg.MaxRequestPayloadBytes = lookupResponseSplitRequestPayloadBytes
	ccfg.MaxResponsePayloadBytes = lookupResponseSplitPayloadBytes
	client := NewCgroupsLookupClient(winTestRunDir, service, ccfg)
	defer client.Close()
	client.SetCallTimeout(lookupScaleCallTimeoutMs)
	waitWinClientReady(t, client)

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

func TestWinLookupRequestCapacityReconnectPaths(t *testing.T) {
	t.Run("reconnect grows capacity", func(t *testing.T) {
		svc := uniqueWinService("go_win_lookup_request_capacity")
		cfg := testWinServerConfig()
		cfg.MaxRequestPayloadBytes = 256
		ts := startTestServerWinWithConfig(svc, cfg, protocol.MethodAppsLookup, AppsLookupDispatch(winTestAppsLookupHandler))
		defer ts.stop()

		ccfg := testWinClientConfig()
		ccfg.MaxRequestPayloadBytes = 256
		client := NewAppsLookupClient(winTestRunDir, svc, ccfg)
		defer client.Close()
		waitWinClientReady(t, client)
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

		missing := NewAppsLookupClient(winTestRunDir, uniqueWinService("go_win_lookup_request_capacity_missing"), ccfg)
		defer missing.Close()
		missing.state = StateReady
		if err := missing.ensureLookupRequestCapacity(128); err != nil {
			t.Fatalf("sessionless configured capacity should not reconnect: %v", err)
		}
	})

	t.Run("reconnect endpoint missing", func(t *testing.T) {
		svc := uniqueWinService("go_win_lookup_request_capacity_gone")
		cfg := testWinServerConfig()
		cfg.MaxRequestPayloadBytes = 256
		ts := startTestServerWinWithConfig(svc, cfg, protocol.MethodAppsLookup, AppsLookupDispatch(winTestAppsLookupHandler))

		ccfg := testWinClientConfig()
		ccfg.MaxRequestPayloadBytes = 256
		client := NewAppsLookupClient(winTestRunDir, svc, ccfg)
		defer client.Close()
		waitWinClientReady(t, client)
		if client.session == nil {
			t.Fatal("client session should be ready")
		}
		client.session.MaxRequestPayloadBytes = 64
		ts.stop()

		if err := client.ensureLookupRequestCapacity(128); !errors.Is(err, protocol.ErrOverflow) {
			t.Fatalf("missing endpoint capacity reconnect error = %v, want ErrOverflow", err)
		}
		if client.errorCount == 0 {
			t.Fatal("missing endpoint capacity reconnect did not record an error")
		}
	})

}

func TestWinLookupCallsReconnectForRequestCapacityGrowth(t *testing.T) {
	t.Run("apps", func(t *testing.T) {
		svc := uniqueWinService("go_win_apps_lookup_call_capacity")
		cfg := testWinServerConfig()
		cfg.MaxRequestPayloadBytes = 256
		ts := startTestServerWinWithConfig(svc, cfg, protocol.MethodAppsLookup, AppsLookupDispatch(winTestAppsLookupHandler))
		defer ts.stop()

		ccfg := testWinClientConfig()
		ccfg.MaxRequestPayloadBytes = 256
		client := NewAppsLookupClient(winTestRunDir, svc, ccfg)
		defer client.Close()
		waitWinClientReady(t, client)
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
		svc := uniqueWinService("go_win_cgroups_lookup_call_capacity")
		cfg := testWinServerConfig()
		cfg.MaxRequestPayloadBytes = 256
		ts := startTestServerWinWithConfig(svc, cfg, protocol.MethodCgroupsLookup, CgroupsLookupDispatch(winTestCgroupsLookupHandler))
		defer ts.stop()

		ccfg := testWinClientConfig()
		ccfg.MaxRequestPayloadBytes = 256
		client := NewCgroupsLookupClient(winTestRunDir, svc, ccfg)
		defer client.Close()
		waitWinClientReady(t, client)
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

func TestWinLookupTimeoutDuringFollowupSubcall(t *testing.T) {
	t.Run("apps", func(t *testing.T) {
		svc := uniqueWinService("go_win_apps_lookup_followup_timeout")
		cfg := testWinServerConfig()
		cfg.MaxResponsePayloadBytes = 320
		var calls atomic.Uint32
		ts := startTestServerWinWithConfig(svc, cfg, protocol.MethodAppsLookup, AppsLookupDispatch(
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
		))
		defer ts.stop()

		client := NewAppsLookupClient(winTestRunDir, svc, testWinClientConfig())
		defer client.Close()
		waitWinClientReady(t, client)

		_, err := client.CallAppsLookupWithTimeout([]uint32{11, 22, 33}, 30)
		if !errors.Is(err, protocol.ErrTimeout) {
			t.Fatalf("apps follow-up timeout error = %v, want ErrTimeout", err)
		}
		if calls.Load() < 2 {
			t.Fatalf("apps follow-up timeout calls = %d, want >= 2", calls.Load())
		}
	})

	t.Run("cgroups", func(t *testing.T) {
		svc := uniqueWinService("go_win_cgroups_lookup_followup_timeout")
		cfg := testWinServerConfig()
		cfg.MaxResponsePayloadBytes = 160
		var calls atomic.Uint32
		ts := startTestServerWinWithConfig(svc, cfg, protocol.MethodCgroupsLookup, CgroupsLookupDispatch(
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
		))
		defer ts.stop()

		client := NewCgroupsLookupClient(winTestRunDir, svc, testWinClientConfig())
		defer client.Close()
		waitWinClientReady(t, client)

		_, err := client.CallCgroupsLookupWithTimeout([][]byte{[]byte("/a"), []byte("/huge"), []byte("/b")}, 30)
		if !errors.Is(err, protocol.ErrTimeout) {
			t.Fatalf("cgroups follow-up timeout error = %v, want ErrTimeout", err)
		}
		if calls.Load() < 2 {
			t.Fatalf("cgroups follow-up timeout calls = %d, want >= 2", calls.Load())
		}
	})
}

func TestWinLookupAbortDuringFollowupSubcall(t *testing.T) {
	t.Run("apps", func(t *testing.T) {
		svc := uniqueWinService("go_win_apps_lookup_followup_abort")
		cfg := testWinServerConfig()
		cfg.MaxResponsePayloadBytes = 320
		var calls atomic.Uint32
		secondCall := make(chan struct{}, 1)
		ts := startTestServerWinWithConfig(svc, cfg, protocol.MethodAppsLookup, AppsLookupDispatch(
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
		))
		defer ts.stop()

		client := NewAppsLookupClient(winTestRunDir, svc, testWinClientConfig())
		defer client.Close()
		waitWinClientReady(t, client)

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
		svc := uniqueWinService("go_win_cgroups_lookup_followup_abort")
		cfg := testWinServerConfig()
		cfg.MaxResponsePayloadBytes = 160
		var calls atomic.Uint32
		secondCall := make(chan struct{}, 1)
		ts := startTestServerWinWithConfig(svc, cfg, protocol.MethodCgroupsLookup, CgroupsLookupDispatch(
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
		))
		defer ts.stop()

		client := NewCgroupsLookupClient(winTestRunDir, svc, testWinClientConfig())
		defer client.Close()
		waitWinClientReady(t, client)

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

func TestWinCgroupsLookupRejectsMixedGenerationRetry(t *testing.T) {
	svc := uniqueWinService("go_win_cgroups_lookup_generation")
	cfg := testWinServerConfig()
	cfg.MaxResponsePayloadBytes = 160
	var calls atomic.Uint32

	ts := startTestServerWinWithConfig(svc, cfg, protocol.MethodCgroupsLookup, CgroupsLookupDispatch(
		func(req *protocol.CgroupsLookupRequestView, builder *protocol.CgroupsLookupBuilder) bool {
			builder.SetGeneration(uint64(calls.Add(1)))
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
	))
	defer ts.stop()

	client := NewCgroupsLookupClient(winTestRunDir, svc, testWinClientConfig())
	defer client.Close()
	waitWinClientReady(t, client)

	if _, err := client.CallCgroupsLookup([][]byte{[]byte("/a"), []byte("/huge"), []byte("/b")}); err == nil {
		t.Fatal("expected mixed generation lookup to fail")
	}
	if calls.Load() < 2 {
		t.Fatalf("handler calls = %d, want at least 2", calls.Load())
	}
}

func TestWinAppsLookupRejectsMixedGenerationRetry(t *testing.T) {
	svc := uniqueWinService("go_win_apps_lookup_generation")
	cfg := testWinServerConfig()
	cfg.MaxResponsePayloadBytes = 320
	var calls atomic.Uint32

	ts := startTestServerWinWithConfig(svc, cfg, protocol.MethodAppsLookup, AppsLookupDispatch(
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
	))
	defer ts.stop()

	client := NewAppsLookupClient(winTestRunDir, svc, testWinClientConfig())
	defer client.Close()
	waitWinClientReady(t, client)

	if _, err := client.CallAppsLookup([]uint32{11, 22, 33}); err == nil {
		t.Fatal("expected mixed generation lookup to fail")
	}
	if calls.Load() < 2 {
		t.Fatalf("handler calls = %d, want at least 2", calls.Load())
	}
}

func TestWinAppsLookupRejectsMalformedTypedResponses(t *testing.T) {
	cases := []struct {
		name    string
		pids    []uint32
		handler DispatchHandler
		want    error
	}{
		{
			name: "truncated payload",
			handler: func([]byte, []byte) (int, error) {
				return 3, nil
			},
			want: protocol.ErrTruncated,
		},
		{
			name: "bad item count",
			handler: func(_ []byte, responseBuf []byte) (int, error) {
				return protocol.NewAppsLookupBuilder(responseBuf, 0, 1).Finish(), nil
			},
			want: protocol.ErrBadItemCount,
		},
		{
			name: "pid mismatch",
			handler: func(_ []byte, responseBuf []byte) (int, error) {
				builder := protocol.NewAppsLookupBuilder(responseBuf, 1, 1)
				if err := builder.Add(protocol.PidLookupUnknown, 0, 0, 9999, 0, protocol.NipcUIDUnset, 0, nil, nil, nil, nil); err != nil {
					return 0, err
				}
				return builder.Finish(), nil
			},
			want: protocol.ErrBadLayout,
		},
		{
			name: "reordered response items",
			pids: []uint32{1, 2},
			handler: func(_ []byte, responseBuf []byte) (int, error) {
				builder := protocol.NewAppsLookupBuilder(responseBuf, 2, 1)
				for _, pid := range []uint32{2, 1} {
					if err := builder.Add(protocol.PidLookupUnknown, 0, 0, pid, 0, protocol.NipcUIDUnset, 0, nil, nil, nil, nil); err != nil {
						return 0, err
					}
				}
				return builder.Finish(), nil
			},
			want: protocol.ErrBadLayout,
		},
		{
			name: "duplicate response items",
			pids: []uint32{1, 2},
			handler: func(_ []byte, responseBuf []byte) (int, error) {
				builder := protocol.NewAppsLookupBuilder(responseBuf, 2, 1)
				for _, pid := range []uint32{1, 1} {
					if err := builder.Add(protocol.PidLookupUnknown, 0, 0, pid, 0, protocol.NipcUIDUnset, 0, nil, nil, nil, nil); err != nil {
						return 0, err
					}
				}
				return builder.Finish(), nil
			},
			want: protocol.ErrBadLayout,
		},
		{
			name: "invalid status enum",
			handler: func(_ []byte, responseBuf []byte) (int, error) {
				builder := protocol.NewAppsLookupBuilder(responseBuf, 1, 1)
				if err := builder.Add(
					protocol.PidLookupKnown,
					protocol.AppsCgroupKnown,
					protocol.OrchestratorDocker,
					1234, 1, 1000, 42,
					[]byte("comm"), []byte("/cg"), []byte("name"),
					[]struct{ Key, Value []byte }{{Key: []byte("role"), Value: []byte("api")}},
				); err != nil {
					return 0, err
				}
				n := builder.Finish()
				patchLookupResponseItemU16(t, responseBuf[:n], protocol.AppsLookupRespHdr, 1, 0, 2, 0xffff)
				return n, nil
			},
			want: protocol.ErrBadLayout,
		},
		{
			name: "invalid status-dependent fields",
			handler: func(_ []byte, responseBuf []byte) (int, error) {
				builder := protocol.NewAppsLookupBuilder(responseBuf, 1, 1)
				if err := builder.Add(
					protocol.PidLookupKnown,
					protocol.AppsCgroupKnown,
					protocol.OrchestratorDocker,
					1234, 1, 1000, 42,
					[]byte("comm"), []byte("/cg"), []byte("name"),
					[]struct{ Key, Value []byte }{{Key: []byte("role"), Value: []byte("api")}},
				); err != nil {
					return 0, err
				}
				n := builder.Finish()
				patchLookupResponseItemU16(t, responseBuf[:n], protocol.AppsLookupRespHdr, 1, 0, 2, protocol.PidLookupUnknown)
				return n, nil
			},
			want: protocol.ErrBadLayout,
		},
		{
			name: "invalid label table layout",
			handler: func(_ []byte, responseBuf []byte) (int, error) {
				builder := protocol.NewAppsLookupBuilder(responseBuf, 1, 1)
				if err := builder.Add(
					protocol.PidLookupKnown,
					protocol.AppsCgroupKnown,
					protocol.OrchestratorDocker,
					1234, 1, 1000, 42,
					[]byte("comm"), []byte("/cg"), []byte("name"),
					[]struct{ Key, Value []byte }{{Key: []byte("role"), Value: []byte("api")}},
				); err != nil {
					return 0, err
				}
				n := builder.Finish()
				patchLookupResponseItemU16(t, responseBuf[:n], protocol.AppsLookupRespHdr, 1, 0, 56, 2)
				return n, nil
			},
			want: protocol.ErrOutOfBounds,
		},
		{
			name: "payload exceeded first item",
			handler: func(_ []byte, responseBuf []byte) (int, error) {
				builder := protocol.NewAppsLookupBuilder(responseBuf, 1, 1)
				if err := builder.Add(protocol.PidLookupPayloadExceeded, 0, 0, 1234, 0, protocol.NipcUIDUnset, 0, nil, nil, nil, nil); err != nil {
					return 0, err
				}
				return builder.Finish(), nil
			},
			want: protocol.ErrOverflow,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := uniqueWinService("go_win_apps_lookup_bad_typed")
			ts := startTestServerWinWithConfig(svc, testWinServerConfig(), protocol.MethodAppsLookup, tc.handler)
			defer ts.stop()

			client := NewAppsLookupClient(winTestRunDir, svc, testWinClientConfig())
			defer client.Close()
			waitWinClientReady(t, client)

			pids := tc.pids
			if pids == nil {
				pids = []uint32{1234}
			}
			if _, err := client.CallAppsLookup(pids); !errors.Is(err, tc.want) {
				t.Fatalf("CallAppsLookup error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestWinCgroupsLookupRejectsMalformedTypedResponses(t *testing.T) {
	cases := []struct {
		name    string
		paths   [][]byte
		handler DispatchHandler
		want    error
	}{
		{
			name: "truncated payload",
			handler: func([]byte, []byte) (int, error) {
				return 3, nil
			},
			want: protocol.ErrTruncated,
		},
		{
			name: "bad item count",
			handler: func(_ []byte, responseBuf []byte) (int, error) {
				return protocol.NewCgroupsLookupBuilder(responseBuf, 0, 1).Finish(), nil
			},
			want: protocol.ErrBadItemCount,
		},
		{
			name: "path mismatch",
			handler: func(_ []byte, responseBuf []byte) (int, error) {
				builder := protocol.NewCgroupsLookupBuilder(responseBuf, 1, 1)
				if err := builder.Add(protocol.CgroupLookupUnknownRetryLater, 0, []byte("/other"), nil, nil); err != nil {
					return 0, err
				}
				return builder.Finish(), nil
			},
			want: protocol.ErrBadLayout,
		},
		{
			name:  "reordered response items",
			paths: [][]byte{[]byte("/a"), []byte("/b")},
			handler: func(_ []byte, responseBuf []byte) (int, error) {
				builder := protocol.NewCgroupsLookupBuilder(responseBuf, 2, 1)
				for _, path := range [][]byte{[]byte("/b"), []byte("/a")} {
					if err := builder.Add(protocol.CgroupLookupUnknownRetryLater, 0, path, nil, nil); err != nil {
						return 0, err
					}
				}
				return builder.Finish(), nil
			},
			want: protocol.ErrBadLayout,
		},
		{
			name:  "duplicate response items",
			paths: [][]byte{[]byte("/a"), []byte("/b")},
			handler: func(_ []byte, responseBuf []byte) (int, error) {
				builder := protocol.NewCgroupsLookupBuilder(responseBuf, 2, 1)
				for _, path := range [][]byte{[]byte("/a"), []byte("/a")} {
					if err := builder.Add(protocol.CgroupLookupUnknownRetryLater, 0, path, nil, nil); err != nil {
						return 0, err
					}
				}
				return builder.Finish(), nil
			},
			want: protocol.ErrBadLayout,
		},
		{
			name: "invalid status enum",
			handler: func(_ []byte, responseBuf []byte) (int, error) {
				builder := protocol.NewCgroupsLookupBuilder(responseBuf, 1, 1)
				if err := builder.Add(
					protocol.CgroupLookupKnown,
					protocol.OrchestratorK8s,
					[]byte("/expected"),
					[]byte("name"),
					[]struct{ Key, Value []byte }{{Key: []byte("role"), Value: []byte("db")}},
				); err != nil {
					return 0, err
				}
				n := builder.Finish()
				patchLookupResponseItemU16(t, responseBuf[:n], protocol.CgroupsLookupRespHdr, 1, 0, 2, 0xffff)
				return n, nil
			},
			want: protocol.ErrBadLayout,
		},
		{
			name: "invalid status-dependent fields",
			handler: func(_ []byte, responseBuf []byte) (int, error) {
				builder := protocol.NewCgroupsLookupBuilder(responseBuf, 1, 1)
				if err := builder.Add(
					protocol.CgroupLookupKnown,
					protocol.OrchestratorK8s,
					[]byte("/expected"),
					[]byte("name"),
					[]struct{ Key, Value []byte }{{Key: []byte("role"), Value: []byte("db")}},
				); err != nil {
					return 0, err
				}
				n := builder.Finish()
				patchLookupResponseItemU16(t, responseBuf[:n], protocol.CgroupsLookupRespHdr, 1, 0, 2, protocol.CgroupLookupUnknownRetryLater)
				return n, nil
			},
			want: protocol.ErrBadLayout,
		},
		{
			name: "invalid label table layout",
			handler: func(_ []byte, responseBuf []byte) (int, error) {
				builder := protocol.NewCgroupsLookupBuilder(responseBuf, 1, 1)
				if err := builder.Add(
					protocol.CgroupLookupKnown,
					protocol.OrchestratorK8s,
					[]byte("/expected"),
					[]byte("name"),
					[]struct{ Key, Value []byte }{{Key: []byte("role"), Value: []byte("db")}},
				); err != nil {
					return 0, err
				}
				n := builder.Finish()
				patchLookupResponseItemU16(t, responseBuf[:n], protocol.CgroupsLookupRespHdr, 1, 0, 24, 2)
				return n, nil
			},
			want: protocol.ErrOutOfBounds,
		},
		{
			name: "payload exceeded first item",
			handler: func(_ []byte, responseBuf []byte) (int, error) {
				builder := protocol.NewCgroupsLookupBuilder(responseBuf, 1, 1)
				if err := builder.Add(protocol.CgroupLookupPayloadExceeded, 0, []byte("/expected"), nil, nil); err != nil {
					return 0, err
				}
				return builder.Finish(), nil
			},
			want: protocol.ErrOverflow,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := uniqueWinService("go_win_cgroups_lookup_bad_typed")
			ts := startTestServerWinWithConfig(svc, testWinServerConfig(), protocol.MethodCgroupsLookup, tc.handler)
			defer ts.stop()

			client := NewCgroupsLookupClient(winTestRunDir, svc, testWinClientConfig())
			defer client.Close()
			waitWinClientReady(t, client)

			paths := tc.paths
			if paths == nil {
				paths = [][]byte{[]byte("/expected")}
			}
			if _, err := client.CallCgroupsLookup(paths); !errors.Is(err, tc.want) {
				t.Fatalf("CallCgroupsLookup error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestWinLookupRejectsTooSmallNegotiatedRequestCapacity(t *testing.T) {
	t.Run("apps", func(t *testing.T) {
		svc := uniqueWinService("go_win_apps_lookup_tiny_request")
		ts := startTestServerWinWithConfig(svc, testWinServerConfig(), protocol.MethodAppsLookup, AppsLookupDispatch(
			func(req *protocol.AppsLookupRequestView, builder *protocol.AppsLookupBuilder) bool {
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
		))
		defer ts.stop()

		client := NewAppsLookupClient(winTestRunDir, svc, testWinClientConfig())
		defer client.Close()
		waitWinClientReady(t, client)
		client.config.MaxRequestPayloadBytes = 1
		client.session.MaxRequestPayloadBytes = 1

		if _, err := client.CallAppsLookup([]uint32{1234}); !errors.Is(err, protocol.ErrOverflow) {
			t.Fatalf("tiny apps request capacity error = %v, want ErrOverflow", err)
		}
	})

	t.Run("cgroups", func(t *testing.T) {
		svc := uniqueWinService("go_win_cgroups_lookup_tiny_request")
		ts := startTestServerWinWithConfig(svc, testWinServerConfig(), protocol.MethodCgroupsLookup, CgroupsLookupDispatch(
			func(req *protocol.CgroupsLookupRequestView, builder *protocol.CgroupsLookupBuilder) bool {
				for i := uint32(0); i < req.ItemCount; i++ {
					path, err := req.Item(i)
					if err != nil {
						return false
					}
					if err := builder.Add(protocol.CgroupLookupUnknownRetryLater, 0, path.Bytes(), nil, nil); err != nil {
						return false
					}
				}
				return true
			},
		))
		defer ts.stop()

		client := NewCgroupsLookupClient(winTestRunDir, svc, testWinClientConfig())
		defer client.Close()
		waitWinClientReady(t, client)
		client.config.MaxRequestPayloadBytes = 1
		client.session.MaxRequestPayloadBytes = 1

		view, err := client.CallCgroupsLookup([][]byte{[]byte("/x")})
		if err != nil {
			t.Fatalf("tiny cgroups request capacity call failed: %v", err)
		}
		item, err := view.Item(0)
		if err != nil {
			t.Fatalf("tiny cgroups request capacity item: %v", err)
		}
		if item.Status != protocol.CgroupLookupOversizedItem || item.Path.String() != "/x" {
			t.Fatalf("tiny cgroups request capacity item = %+v, want OVERSIZED_ITEM for /x", item)
		}
	})
}

func TestWinLookupRejectsLogicalResponseByteLimit(t *testing.T) {
	t.Run("apps", func(t *testing.T) {
		svc := uniqueWinService("go_win_apps_lookup_response_limit")
		ts := startTestServerWinWithConfig(svc, testWinServerConfig(), protocol.MethodAppsLookup, AppsLookupDispatch(
			func(req *protocol.AppsLookupRequestView, builder *protocol.AppsLookupBuilder) bool {
				pid, err := req.Item(0)
				if err != nil {
					return false
				}
				return builder.Add(protocol.PidLookupUnknown, 0, 0, pid, 0, protocol.NipcUIDUnset, 0, nil, nil, nil, nil) == nil
			},
		))
		defer ts.stop()

		client := NewAppsLookupClient(winTestRunDir, svc, testWinClientConfig())
		defer client.Close()
		waitWinClientReady(t, client)
		client.SetLookupLogicalConfig(LookupLogicalConfig{MaxResponseBytes: 1})

		if _, err := client.CallAppsLookup([]uint32{1234}); !errors.Is(err, protocol.ErrOverflow) {
			t.Fatalf("apps logical response byte limit error = %v, want ErrOverflow", err)
		}
	})

	t.Run("cgroups", func(t *testing.T) {
		svc := uniqueWinService("go_win_cgroups_lookup_response_limit")
		ts := startTestServerWinWithConfig(svc, testWinServerConfig(), protocol.MethodCgroupsLookup, CgroupsLookupDispatch(
			func(req *protocol.CgroupsLookupRequestView, builder *protocol.CgroupsLookupBuilder) bool {
				path, err := req.Item(0)
				if err != nil {
					return false
				}
				return builder.Add(protocol.CgroupLookupUnknownRetryLater, 0, path.Bytes(), nil, nil) == nil
			},
		))
		defer ts.stop()

		client := NewCgroupsLookupClient(winTestRunDir, svc, testWinClientConfig())
		defer client.Close()
		waitWinClientReady(t, client)
		client.SetLookupLogicalConfig(LookupLogicalConfig{MaxResponseBytes: 1})

		if _, err := client.CallCgroupsLookup([][]byte{[]byte("/x")}); !errors.Is(err, protocol.ErrOverflow) {
			t.Fatalf("cgroups logical response byte limit error = %v, want ErrOverflow", err)
		}
	})
}

func TestWinLookupRejectsMalformedPayloadExceededSuffix(t *testing.T) {
	t.Run("apps", func(t *testing.T) {
		svc := uniqueWinService("go_win_apps_lookup_bad_payload_suffix")
		ts := startTestServerWinWithConfig(svc, testWinServerConfig(), protocol.MethodAppsLookup,
			func(_ []byte, responseBuf []byte) (int, error) {
				builder := protocol.NewAppsLookupBuilder(responseBuf, 2, 1)
				if err := builder.Add(protocol.PidLookupPayloadExceeded, 0, 0, 11, 0, protocol.NipcUIDUnset, 0, nil, nil, nil, nil); err != nil {
					return 0, err
				}
				if err := builder.Add(protocol.PidLookupUnknown, 0, 0, 22, 0, protocol.NipcUIDUnset, 0, nil, nil, nil, nil); err != nil {
					return 0, err
				}
				return builder.Finish(), nil
			})
		defer ts.stop()

		client := NewAppsLookupClient(winTestRunDir, svc, testWinClientConfig())
		defer client.Close()
		waitWinClientReady(t, client)

		if _, err := client.CallAppsLookup([]uint32{11, 22}); !errors.Is(err, protocol.ErrBadLayout) {
			t.Fatalf("apps malformed payload-exceeded suffix error = %v, want ErrBadLayout", err)
		}
	})

	t.Run("cgroups", func(t *testing.T) {
		svc := uniqueWinService("go_win_cgroups_lookup_bad_payload_suffix")
		ts := startTestServerWinWithConfig(svc, testWinServerConfig(), protocol.MethodCgroupsLookup,
			func(_ []byte, responseBuf []byte) (int, error) {
				builder := protocol.NewCgroupsLookupBuilder(responseBuf, 2, 1)
				if err := builder.Add(protocol.CgroupLookupPayloadExceeded, 0, []byte("/a"), nil, nil); err != nil {
					return 0, err
				}
				if err := builder.Add(protocol.CgroupLookupUnknownRetryLater, 0, []byte("/b"), nil, nil); err != nil {
					return 0, err
				}
				return builder.Finish(), nil
			})
		defer ts.stop()

		client := NewCgroupsLookupClient(winTestRunDir, svc, testWinClientConfig())
		defer client.Close()
		waitWinClientReady(t, client)

		if _, err := client.CallCgroupsLookup([][]byte{[]byte("/a"), []byte("/b")}); !errors.Is(err, protocol.ErrBadLayout) {
			t.Fatalf("cgroups malformed payload-exceeded suffix error = %v, want ErrBadLayout", err)
		}
	})
}

func TestWinLookupRejectsMalformedFollowupResponse(t *testing.T) {
	t.Run("apps", func(t *testing.T) {
		svc := uniqueWinService("go_win_apps_lookup_bad_followup")
		var calls atomic.Uint32
		ts := startTestServerWinWithConfig(svc, testWinServerConfig(), protocol.MethodAppsLookup, AppsLookupDispatch(
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

		client := NewAppsLookupClient(winTestRunDir, svc, testWinClientConfig())
		defer client.Close()
		waitWinClientReady(t, client)

		if _, err := client.CallAppsLookup([]uint32{11, 22, 33}); !errors.Is(err, protocol.ErrBadLayout) {
			t.Fatalf("apps malformed follow-up error = %v, want ErrBadLayout", err)
		}
		if calls.Load() < 2 {
			t.Fatalf("apps malformed follow-up calls = %d, want >= 2", calls.Load())
		}
	})

	t.Run("cgroups", func(t *testing.T) {
		svc := uniqueWinService("go_win_cgroups_lookup_bad_followup")
		var calls atomic.Uint32
		ts := startTestServerWinWithConfig(svc, testWinServerConfig(), protocol.MethodCgroupsLookup, CgroupsLookupDispatch(
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

		client := NewCgroupsLookupClient(winTestRunDir, svc, testWinClientConfig())
		defer client.Close()
		waitWinClientReady(t, client)

		if _, err := client.CallCgroupsLookup([][]byte{[]byte("/a"), []byte("/b"), []byte("/c")}); !errors.Is(err, protocol.ErrBadLayout) {
			t.Fatalf("cgroups malformed follow-up error = %v, want ErrBadLayout", err)
		}
		if calls.Load() < 2 {
			t.Fatalf("cgroups malformed follow-up calls = %d, want >= 2", calls.Load())
		}
	})
}

func winAppsLookupPartialResponse(payload []byte) ([]byte, uint32, error) {
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

func winCgroupsLookupPartialResponse(payload []byte) ([]byte, uint32, error) {
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

func TestWinLookupEndpointGoneAfterPartialProgress(t *testing.T) {
	t.Run("apps", func(t *testing.T) {
		svc := uniqueWinService("go_win_apps_lookup_partial_disconnect")
		srv := startRawWinSessionServer(t, svc, testWinServerConfig(),
			func(session *windows.Session, hdr protocol.Header, payload []byte) error {
				if hdr.Kind != protocol.KindRequest || hdr.Code != protocol.MethodAppsLookup {
					return errors.New("unexpected apps lookup request header")
				}
				respPayload, _, err := winAppsLookupPartialResponse(payload)
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

		client := NewAppsLookupClient(winTestRunDir, svc, testWinClientConfig())
		defer client.Close()
		waitWinClientReady(t, client)

		if _, err := client.CallAppsLookupWithTimeout([]uint32{11, 22, 33}, 1000); err == nil {
			t.Fatal("apps lookup should fail when endpoint disappears after partial progress")
		}
		srv.wait(t)
	})

	t.Run("cgroups", func(t *testing.T) {
		svc := uniqueWinService("go_win_cgroups_lookup_partial_disconnect")
		srv := startRawWinSessionServer(t, svc, testWinServerConfig(),
			func(session *windows.Session, hdr protocol.Header, payload []byte) error {
				if hdr.Kind != protocol.KindRequest || hdr.Code != protocol.MethodCgroupsLookup {
					return errors.New("unexpected cgroups lookup request header")
				}
				respPayload, _, err := winCgroupsLookupPartialResponse(payload)
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

		client := NewCgroupsLookupClient(winTestRunDir, svc, testWinClientConfig())
		defer client.Close()
		waitWinClientReady(t, client)

		paths := [][]byte{[]byte("/a"), []byte("/b"), []byte("/c")}
		if _, err := client.CallCgroupsLookupWithTimeout(paths, 1000); err == nil {
			t.Fatal("cgroups lookup should fail when endpoint disappears after partial progress")
		}
		srv.wait(t)
	})
}

func TestWinLookupEndpointGoneBeforeFirstSubcall(t *testing.T) {
	t.Run("apps", func(t *testing.T) {
		svc := uniqueWinService("go_win_apps_lookup_gone_before_call")
		ts := startTestServerWin(svc, protocol.MethodAppsLookup, AppsLookupDispatch(winTestAppsLookupHandler))

		client := NewAppsLookupClient(winTestRunDir, svc, testWinClientConfig())
		defer client.Close()
		waitWinClientReady(t, client)
		ts.stop()

		if _, err := client.CallAppsLookupWithTimeout([]uint32{11, 22}, 1000); err == nil {
			t.Fatal("apps lookup should fail when endpoint disappears before first subcall")
		}
	})

	t.Run("cgroups", func(t *testing.T) {
		svc := uniqueWinService("go_win_cgroups_lookup_gone_before_call")
		ts := startTestServerWin(svc, protocol.MethodCgroupsLookup, CgroupsLookupDispatch(winTestCgroupsLookupHandler))

		client := NewCgroupsLookupClient(winTestRunDir, svc, testWinClientConfig())
		defer client.Close()
		waitWinClientReady(t, client)
		ts.stop()

		if _, err := client.CallCgroupsLookupWithTimeout([][]byte{[]byte("/a"), []byte("/b")}, 1000); err == nil {
			t.Fatal("cgroups lookup should fail when endpoint disappears before first subcall")
		}
	})
}

func TestWinLookupEndpointAbsentBeforeCall(t *testing.T) {
	t.Run("apps", func(t *testing.T) {
		svc := uniqueWinService("go_win_apps_lookup_absent")
		client := NewAppsLookupClient(winTestRunDir, svc, testWinClientConfig())
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
		svc := uniqueWinService("go_win_cgroups_lookup_absent")
		client := NewCgroupsLookupClient(winTestRunDir, svc, testWinClientConfig())
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

func TestWinCgroupsLookupRejectsInvalidRequestKeyAfterReady(t *testing.T) {
	svc := uniqueWinService("go_win_cgroups_lookup_invalid_key")
	ts := startTestServerWinWithConfig(svc, testWinServerConfig(), protocol.MethodCgroupsLookup, CgroupsLookupDispatch(
		func(req *protocol.CgroupsLookupRequestView, builder *protocol.CgroupsLookupBuilder) bool {
			path, err := req.Item(0)
			if err != nil {
				return false
			}
			return builder.Add(protocol.CgroupLookupUnknownRetryLater, 0, path.Bytes(), nil, nil) == nil
		},
	))
	defer ts.stop()

	client := NewCgroupsLookupClient(winTestRunDir, svc, testWinClientConfig())
	defer client.Close()
	waitWinClientReady(t, client)

	if _, err := client.CallCgroupsLookup([][]byte{[]byte("bad\x00path")}); !errors.Is(err, protocol.ErrBadLayout) {
		t.Fatalf("invalid cgroups lookup request key error = %v, want ErrBadLayout", err)
	}
}
