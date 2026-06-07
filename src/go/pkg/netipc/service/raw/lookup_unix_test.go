//go:build unix

package raw

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
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
	ensureRunDir()
	cleanupAll(service)

	s := NewServerWithWorkers(testRunDir, service, testServerConfig(), method, handler, workers)
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
