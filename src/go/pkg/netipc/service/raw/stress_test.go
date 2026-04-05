//go:build unix

package raw

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	"github.com/netdata/netdata/go/plugins/pkg/netipc/transport/posix"
)

// simpleHash: djb2 matching the C implementation
func simpleHash(s string) uint32 {
	var hash uint32 = 5381
	for _, c := range []byte(s) {
		hash = ((hash << 5) + hash) + uint32(c)
	}
	return hash
}

// largeHandler builds a snapshot with N items (realistic container names).
func largeHandler(n int) DispatchHandler {
	return SnapshotDispatch(func(request *protocol.CgroupsRequest, builder *protocol.CgroupsBuilder) bool {
		if request.LayoutVersion != 1 || request.Flags != 0 {
			return false
		}
		builder.SetHeader(1, 42)

		for i := 0; i < n; i++ {
			name := fmt.Sprintf("container-%04d", i)
			path := fmt.Sprintf("/sys/fs/cgroup/docker/%04d", i)
			hash := simpleHash(name)
			enabled := uint32(1)
			if i%5 == 0 {
				enabled = 0
			}
			if err := builder.Add(hash, 0x10, enabled,
				[]byte(name), []byte(path)); err != nil {
				return false
			}
		}
		return true
	}, uint32(n))
}

// verifySnapshot checks all items in a snapshot view match expected data.
func verifySnapshot(t *testing.T, view *protocol.CgroupsResponseView, n int) bool {
	t.Helper()
	if int(view.ItemCount) != n {
		t.Errorf("expected %d items, got %d", n, view.ItemCount)
		return false
	}
	if view.SystemdEnabled != 1 {
		t.Errorf("expected systemd_enabled=1, got %d", view.SystemdEnabled)
		return false
	}
	if view.Generation != 42 {
		t.Errorf("expected generation=42, got %d", view.Generation)
		return false
	}

	// Spot-check first, middle, last
	indices := []int{0, n / 2, n - 1}
	for _, idx := range indices {
		item, err := view.Item(uint32(idx))
		if err != nil {
			t.Errorf("item %d decode error: %v", idx, err)
			return false
		}
		expectedName := fmt.Sprintf("container-%04d", idx)
		expectedPath := fmt.Sprintf("/sys/fs/cgroup/docker/%04d", idx)
		expectedHash := simpleHash(expectedName)
		expectedEnabled := uint32(1)
		if idx%5 == 0 {
			expectedEnabled = 0
		}

		if item.Hash != expectedHash {
			t.Errorf("item %d: hash=%d, expected=%d", idx, item.Hash, expectedHash)
			return false
		}
		if item.Name.String() != expectedName {
			t.Errorf("item %d: name=%q, expected=%q", idx, item.Name.String(), expectedName)
			return false
		}
		if item.Path.String() != expectedPath {
			t.Errorf("item %d: path=%q, expected=%q", idx, item.Path.String(), expectedPath)
			return false
		}
		if item.Enabled != expectedEnabled {
			t.Errorf("item %d: enabled=%d, expected=%d", idx, item.Enabled, expectedEnabled)
			return false
		}
		if item.Options != 0x10 {
			t.Errorf("item %d: options=0x%x, expected=0x10", idx, item.Options)
			return false
		}
	}
	return true
}

// startLargeServer creates a server with a large response buffer.
// startLargeServer creates a server with large response buffer and explicit
// packet_size to enable chunked transport for payloads > 64KB.
func startLargeServer(service string, handler DispatchHandler, maxResp uint32) *testServer {
	ensureRunDir()
	cleanupAll(service)

	cfg := testServerConfig()
	cfg.MaxResponsePayloadBytes = maxResp
	cfg.PacketSize = 65536 // force smaller packets for reliable chunking

	s := NewServer(testRunDir, service, cfg, protocol.MethodCgroupsSnapshot, handler)
	doneCh := make(chan struct{})

	go func() {
		defer close(doneCh)
		s.Run()
	}()

	waitUnixServerReady(service)
	return &testServer{server: s, doneCh: doneCh}
}

// startServerWithWorkers creates a server with explicit worker count.
func startServerWithWorkers(service string, handler DispatchHandler, workers int) *testServer {
	ensureRunDir()
	cleanupAll(service)

	s := NewServerWithWorkers(
		testRunDir,
		service,
		testServerConfig(),
		protocol.MethodCgroupsSnapshot,
		handler,
		workers,
	)
	doneCh := make(chan struct{})

	go func() {
		defer close(doneCh)
		s.Run()
	}()

	waitUnixServerReady(service)
	return &testServer{server: s, doneCh: doneCh}
}

func TestStress1000Items(t *testing.T) {
	const N = 1000
	const bufSize = 300 * N

	svc := "go_stress_1k"
	ts := startLargeServer(svc, largeHandler(N), bufSize)
	defer ts.stop()

	ccfg := testClientConfig()
	ccfg.MaxResponsePayloadBytes = bufSize
	ccfg.PacketSize = 65536

	client := NewSnapshotClient(testRunDir, svc, ccfg)
	client.Refresh()
	if !client.Ready() {
		t.Fatal("client not ready")
	}

	start := time.Now()
	view, err := client.CallSnapshot()
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}

	t.Logf("1000 items: %v", elapsed)

	// Verify ALL items
	if int(view.ItemCount) != N {
		t.Fatalf("expected %d items, got %d", N, view.ItemCount)
	}
	for i := 0; i < N; i++ {
		item, ierr := view.Item(uint32(i))
		if ierr != nil {
			t.Fatalf("item %d decode error: %v", i, ierr)
		}
		expectedName := fmt.Sprintf("container-%04d", i)
		expectedPath := fmt.Sprintf("/sys/fs/cgroup/docker/%04d", i)
		expectedHash := simpleHash(expectedName)
		if item.Hash != expectedHash {
			t.Fatalf("item %d: hash mismatch", i)
		}
		if item.Name.String() != expectedName {
			t.Fatalf("item %d: name=%q expected=%q", i, item.Name.String(), expectedName)
		}
		if item.Path.String() != expectedPath {
			t.Fatalf("item %d: path=%q expected=%q", i, item.Path.String(), expectedPath)
		}
	}

	client.Close()
	cleanupAll(svc)
}

func TestStress5000Items(t *testing.T) {
	const N = 5000
	const bufSize = 300 * N

	svc := "go_stress_5k"
	ts := startLargeServer(svc, largeHandler(N), bufSize)
	defer ts.stop()

	ccfg := testClientConfig()
	ccfg.MaxResponsePayloadBytes = bufSize
	ccfg.PacketSize = 65536

	client := NewSnapshotClient(testRunDir, svc, ccfg)
	client.Refresh()
	if !client.Ready() {
		t.Fatal("client not ready")
	}

	start := time.Now()
	view, err := client.CallSnapshot()
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}

	t.Logf("5000 items: %v", elapsed)

	if int(view.ItemCount) != N {
		t.Fatalf("expected %d items, got %d", N, view.ItemCount)
	}

	// Verify spot checks
	if !verifySnapshot(t, view, N) {
		t.Fatal("snapshot verification failed")
	}

	client.Close()
	cleanupAll(svc)
}

func TestStress50Clients(t *testing.T) {
	svc := "go_stress_mc50"
	ensureRunDir()
	cleanupAll(svc)

	ts := startServerWithWorkers(svc, testSnapshotDispatch(), 64)
	defer ts.stop()

	const numClients = 50
	const requestsPerClient = 10

	type result struct {
		clientID  int
		successes int
		failures  int
	}

	results := make(chan result, numClients)

	start := time.Now()

	for i := 0; i < numClients; i++ {
		go func(id int) {
			r := result{clientID: id}
			client := NewSnapshotClient(testRunDir, svc, testClientConfig())
			defer client.Close()

			for retry := 0; retry < 200; retry++ {
				client.Refresh()
				if client.Ready() {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}

			if !client.Ready() {
				r.failures = requestsPerClient
				results <- r
				return
			}

			for j := 0; j < requestsPerClient; j++ {
				view, err := client.CallSnapshot()
				if err != nil || view.ItemCount != 3 {
					r.failures++
					continue
				}
				// Verify content correctness
				item0, ierr := view.Item(0)
				if ierr != nil || item0.Hash != 1001 ||
					item0.Name.String() != "docker-abc123" {
					r.failures++
					continue
				}
				item2, ierr := view.Item(2)
				if ierr != nil || item2.Hash != 3003 ||
					item2.Name.String() != "systemd-user" {
					r.failures++
					continue
				}
				r.successes++
			}
			results <- r
		}(i)
	}

	totalSuccess := 0
	totalFailure := 0
	for i := 0; i < numClients; i++ {
		r := <-results
		totalSuccess += r.successes
		totalFailure += r.failures
	}

	elapsed := time.Since(start)
	expected := numClients * requestsPerClient
	t.Logf("50 clients x 10 req: %d/%d succeeded, %d failures, %v",
		totalSuccess, expected, totalFailure, elapsed)

	if totalSuccess != expected {
		t.Fatalf("expected %d successes, got %d (failures: %d)",
			expected, totalSuccess, totalFailure)
	}
	if totalFailure != 0 {
		t.Fatalf("expected 0 failures, got %d", totalFailure)
	}

	cleanupAll(svc)
}

func TestStressConcurrentCacheClients(t *testing.T) {
	svc := "go_stress_cache10"
	ensureRunDir()
	cleanupAll(svc)

	ts := startServerWithWorkers(svc, testSnapshotDispatch(), 16)
	defer ts.stop()

	const numClients = 10
	const requestsPerClient = 100

	type result struct {
		successes int
		failures  int
	}

	results := make(chan result, numClients)

	start := time.Now()

	for i := 0; i < numClients; i++ {
		go func() {
			r := result{}
			cache := NewCache(testRunDir, svc, testClientConfig())
			defer cache.Close()

			for j := 0; j < requestsPerClient; j++ {
				updated := cache.Refresh()
				if updated || cache.Ready() {
					status := cache.Status()
					if status.ItemCount != 3 {
						r.failures++
						continue
					}
					// Verify a lookup
					item, found := cache.Lookup(1001, "docker-abc123")
					if !found || item.Hash != 1001 ||
						item.Path != "/sys/fs/cgroup/docker/abc123" {
						r.failures++
						continue
					}
					r.successes++
				} else {
					r.failures++
				}
			}
			results <- r
		}()
	}

	totalSuccess := 0
	totalFailure := 0
	for i := 0; i < numClients; i++ {
		r := <-results
		totalSuccess += r.successes
		totalFailure += r.failures
	}

	elapsed := time.Since(start)
	expected := numClients * requestsPerClient
	t.Logf("10 cache clients x 100 req: %d/%d succeeded, %d failures, %v",
		totalSuccess, expected, totalFailure, elapsed)

	if totalSuccess != expected {
		t.Fatalf("expected %d successes, got %d", expected, totalSuccess)
	}
	if totalFailure != 0 {
		t.Fatalf("expected 0 failures, got %d", totalFailure)
	}

	cleanupAll(svc)
}

func TestStressRapidConnectDisconnect(t *testing.T) {
	svc := "go_stress_rapid"
	ensureRunDir()
	cleanupAll(svc)

	ts := startServerWithWorkers(svc, testSnapshotDispatch(), 16)
	defer ts.stop()

	const cycles = 1000
	successes := 0
	failures := 0

	start := time.Now()

	for i := 0; i < cycles; i++ {
		client := NewSnapshotClient(testRunDir, svc, testClientConfig())

		for r := 0; r < 50; r++ {
			client.Refresh()
			if client.Ready() {
				break
			}
			time.Sleep(2 * time.Millisecond)
		}

		if !client.Ready() {
			failures++
			client.Close()
			continue
		}

		view, err := client.CallSnapshot()
		if err != nil || view.ItemCount != 3 {
			failures++
		} else {
			item0, ierr := view.Item(0)
			if ierr != nil || item0.Hash != 1001 {
				failures++
			} else {
				successes++
			}
		}
		client.Close()
	}

	elapsed := time.Since(start)
	t.Logf("1000 rapid cycles: %d ok, %d fail, %v", successes, failures, elapsed)

	if successes != cycles {
		t.Fatalf("expected %d successes, got %d (failures: %d)",
			cycles, successes, failures)
	}

	cleanupAll(svc)
}

func TestStressLongRunning60s(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 60s test in short mode")
	}

	svc := "go_stress_long"
	ensureRunDir()
	cleanupAll(svc)

	ts := startServerWithWorkers(svc, testSnapshotDispatch(), 8)
	defer ts.stop()

	const numClients = 5
	const duration = 60 * time.Second

	var totalRefreshes int64
	var totalErrors int64
	var wg sync.WaitGroup
	stop := make(chan struct{})

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cache := NewCache(testRunDir, svc, testClientConfig())
			defer cache.Close()

			for {
				select {
				case <-stop:
					return
				default:
				}

				updated := cache.Refresh()
				if updated || cache.Ready() {
					status := cache.Status()
					if status.ItemCount != 3 {
						atomic.AddInt64(&totalErrors, 1)
					} else {
						atomic.AddInt64(&totalRefreshes, 1)
					}
				} else {
					atomic.AddInt64(&totalErrors, 1)
				}

				time.Sleep(time.Millisecond)
			}
		}()
	}

	time.Sleep(duration)
	close(stop)
	wg.Wait()

	refreshes := atomic.LoadInt64(&totalRefreshes)
	errors := atomic.LoadInt64(&totalErrors)

	t.Logf("60s run: %d refreshes, %d errors", refreshes, errors)

	if refreshes == 0 {
		t.Fatal("expected some refreshes to succeed")
	}
	if errors != 0 {
		t.Fatalf("expected 0 errors, got %d", errors)
	}

	cleanupAll(svc)
}

func TestStressMixedTransport(t *testing.T) {
	svc := "go_stress_mixed"
	ensureRunDir()
	cleanupAll(svc)

	// Server supports both baseline and SHM
	cfg := posix.ServerConfig{
		SupportedProfiles:       protocol.ProfileBaseline | protocol.ProfileSHMHybrid,
		PreferredProfiles:       protocol.ProfileSHMHybrid,
		MaxRequestPayloadBytes:  4096,
		MaxRequestBatchItems:    1,
		MaxResponsePayloadBytes: responseBufSize,
		MaxResponseBatchItems:   1,
		AuthToken:               authToken,
		Backlog:                 16,
	}

	s := NewServer(
		testRunDir,
		svc,
		cfg,
		protocol.MethodCgroupsSnapshot,
		testSnapshotDispatch(),
	)
	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		s.Run()
	}()
	waitUnixServerReady(svc)

	defer func() {
		s.Stop()
		<-doneCh
	}()

	type result struct {
		clientID int
		profile  string
		success  int
		failure  int
	}

	results := make(chan result, 3)

	// Client 0: SHM-capable
	go func() {
		ccfg := testClientConfig()
		ccfg.SupportedProfiles = protocol.ProfileBaseline | protocol.ProfileSHMHybrid
		ccfg.PreferredProfiles = protocol.ProfileSHMHybrid

		r := result{clientID: 0, profile: "SHM"}
		client := NewSnapshotClient(testRunDir, svc, ccfg)
		defer client.Close()

		for retry := 0; retry < 200; retry++ {
			client.Refresh()
			if client.Ready() {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}

		for i := 0; i < 10; i++ {
			view, err := client.CallSnapshot()
			if err == nil && view.ItemCount == 3 {
				item0, ierr := view.Item(0)
				if ierr == nil && item0.Hash == 1001 {
					r.success++
					continue
				}
			}
			r.failure++
		}
		results <- r
	}()

	// Client 1: SHM-capable
	go func() {
		ccfg := testClientConfig()
		ccfg.SupportedProfiles = protocol.ProfileBaseline | protocol.ProfileSHMHybrid
		ccfg.PreferredProfiles = protocol.ProfileSHMHybrid

		r := result{clientID: 1, profile: "SHM"}
		client := NewSnapshotClient(testRunDir, svc, ccfg)
		defer client.Close()

		for retry := 0; retry < 200; retry++ {
			client.Refresh()
			if client.Ready() {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}

		for i := 0; i < 10; i++ {
			view, err := client.CallSnapshot()
			if err == nil && view.ItemCount == 3 {
				r.success++
			} else {
				r.failure++
			}
		}
		results <- r
	}()

	// Client 2: UDS-only (baseline)
	go func() {
		ccfg := testClientConfig()
		ccfg.SupportedProfiles = protocol.ProfileBaseline
		ccfg.PreferredProfiles = 0

		r := result{clientID: 2, profile: "UDS"}
		client := NewSnapshotClient(testRunDir, svc, ccfg)
		defer client.Close()

		for retry := 0; retry < 200; retry++ {
			client.Refresh()
			if client.Ready() {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}

		for i := 0; i < 10; i++ {
			view, err := client.CallSnapshot()
			if err == nil && view.ItemCount == 3 {
				item0, ierr := view.Item(0)
				if ierr == nil && item0.Hash == 1001 {
					r.success++
					continue
				}
			}
			r.failure++
		}
		results <- r
	}()

	totalSuccess := 0
	totalFailure := 0
	for i := 0; i < 3; i++ {
		r := <-results
		t.Logf("client %d (%s): %d ok, %d fail", r.clientID, r.profile, r.success, r.failure)
		totalSuccess += r.success
		totalFailure += r.failure
	}

	if totalSuccess != 30 {
		t.Fatalf("expected 30 successes, got %d (failures: %d)", totalSuccess, totalFailure)
	}
	if totalFailure != 0 {
		t.Fatalf("expected 0 failures, got %d", totalFailure)
	}

	cleanupAll(svc)
}
