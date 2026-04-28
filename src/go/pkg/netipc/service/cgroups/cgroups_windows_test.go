//go:build windows

package cgroups

import (
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

const testWinRunDir = `C:\Temp\nipc_go_cgroups_public`

const (
	testWinAuthToken    = uint64(0xDEADBEEFCAFEBABE)
	testWinResponseSize = 65536
)

var winServiceCounter atomic.Uint64

func uniqueWinService(prefix string) string {
	return fmt.Sprintf("%s_%d_%d", prefix, os.Getpid(), winServiceCounter.Add(1))
}

func ensureWinRunDir(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll(testWinRunDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
}

func testWinServerConfig() ServerConfig {
	return ServerConfig{
		SupportedProfiles:       protocol.ProfileBaseline,
		PreferredProfiles:       protocol.ProfileBaseline,
		MaxRequestBatchItems:    1,
		MaxResponsePayloadBytes: testWinResponseSize,
		AuthToken:               testWinAuthToken,
	}
}

func testWinClientConfig() ClientConfig {
	return ClientConfig{
		SupportedProfiles:       protocol.ProfileBaseline,
		PreferredProfiles:       protocol.ProfileBaseline,
		MaxRequestBatchItems:    1,
		MaxResponsePayloadBytes: testWinResponseSize,
		AuthToken:               testWinAuthToken,
	}
}

func testWinHandler() Handler {
	return Handler{
		Handle: func(req *protocol.CgroupsRequest, builder *protocol.CgroupsBuilder) bool {
			if req.LayoutVersion != 1 || req.Flags != 0 {
				return false
			}
			builder.SetHeader(1, 42)
			items := []struct {
				hash, options, enabled uint32
				name, path             []byte
			}{
				{1001, 0, 1, []byte("docker-abc123"), []byte("/sys/fs/cgroup/docker/abc123")},
				{2002, 0, 1, []byte("k8s-pod-xyz"), []byte("/sys/fs/cgroup/kubepods/xyz")},
				{3003, 0, 0, []byte("systemd-user"), []byte("/sys/fs/cgroup/user.slice/user-1000")},
			}
			for _, item := range items {
				if err := builder.Add(item.hash, item.options, item.enabled, item.name, item.path); err != nil {
					return false
				}
			}
			return true
		},
		SnapshotMaxItems: 3,
	}
}

type winTestServer struct {
	server *Server
	done   chan struct{}
}

func startWinTestServer(t *testing.T, service string) *winTestServer {
	t.Helper()
	ensureWinRunDir(t)
	s := NewServer(testWinRunDir, service, testWinServerConfig(), testWinHandler())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = s.Run()
	}()
	time.Sleep(200 * time.Millisecond)
	return &winTestServer{server: s, done: done}
}

func (ts *winTestServer) stop() {
	ts.server.Stop()
	select {
	case <-ts.done:
	case <-time.After(2 * time.Second):
	}
}

func connectReadyWin(t *testing.T, client *Client) {
	t.Helper()
	for i := 0; i < 200; i++ {
		client.Refresh()
		if client.Ready() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("client did not reach READY state")
}

func TestSnapshotRoundTripWindows(t *testing.T) {
	service := uniqueWinService("snapshot")
	ts := startWinTestServer(t, service)
	defer ts.stop()

	client := NewClient(testWinRunDir, service, testWinClientConfig())
	defer client.Close()
	connectReadyWin(t, client)

	view, err := client.CallSnapshot()
	if err != nil {
		t.Fatalf("CallSnapshot failed: %v", err)
	}
	if view.ItemCount != 3 || view.SystemdEnabled != 1 || view.Generation != 42 {
		t.Fatalf("unexpected snapshot header: %+v", view)
	}
}

func TestCacheRoundTripWindows(t *testing.T) {
	service := uniqueWinService("cache")
	ts := startWinTestServer(t, service)
	defer ts.stop()

	cache := NewCache(testWinRunDir, service, testWinClientConfig())
	defer cache.Close()

	if cache.Ready() {
		t.Fatal("cache unexpectedly ready before refresh")
	}
	status0 := cache.Status()
	if status0.Populated || status0.ItemCount != 0 || status0.RefreshSuccessCount != 0 || status0.RefreshFailureCount != 0 || status0.ConnectionState != StateDisconnected || status0.LastRefreshTs != 0 {
		t.Fatalf("unexpected initial cache status: %+v", status0)
	}

	var updated bool
	for i := 0; i < 200; i++ {
		if cache.Refresh() {
			updated = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !updated {
		t.Fatal("Refresh never succeeded")
	}
	if !cache.Ready() {
		t.Fatal("cache not ready after refresh")
	}
	item, ok := cache.Lookup(1001, "docker-abc123")
	if !ok || item.Path != "/sys/fs/cgroup/docker/abc123" {
		t.Fatalf("unexpected cache item: %+v ok=%v", item, ok)
	}
	status1 := cache.Status()
	if !status1.Populated || status1.ItemCount != 3 || status1.SystemdEnabled != 1 || status1.Generation != 42 || status1.RefreshSuccessCount != 1 || status1.RefreshFailureCount != 0 || status1.ConnectionState != StateReady || status1.LastRefreshTs < 0 {
		t.Fatalf("unexpected refreshed cache status: %+v", status1)
	}
}

func TestClientNotReadyReturnsErrorWindows(t *testing.T) {
	service := uniqueWinService("not_ready")
	client := NewClient(testWinRunDir, service, testWinClientConfig())
	defer client.Close()

	status := client.Status()
	if status.State != StateDisconnected || status.ConnectCount != 0 || status.ReconnectCount != 0 || status.CallCount != 0 || status.ErrorCount != 0 {
		t.Fatalf("unexpected initial client status: %+v", status)
	}

	if _, err := client.CallSnapshot(); err != protocol.ErrBadLayout {
		t.Fatalf("CallSnapshot err = %v, want %v", err, protocol.ErrBadLayout)
	}

	status = client.Status()
	if status.State != StateDisconnected || status.ErrorCount != 1 {
		t.Fatalf("unexpected error-path client status: %+v", status)
	}
}

func TestClientStatusWindows(t *testing.T) {
	service := uniqueWinService("status")
	ts := startWinTestServer(t, service)
	defer ts.stop()

	client := NewClient(testWinRunDir, service, testWinClientConfig())
	defer client.Close()
	connectReadyWin(t, client)

	status0 := client.Status()
	if status0.State != StateReady || status0.ConnectCount != 1 || status0.ReconnectCount != 0 || status0.CallCount != 0 || status0.ErrorCount != 0 {
		t.Fatalf("unexpected ready client status: %+v", status0)
	}

	if _, err := client.CallSnapshot(); err != nil {
		t.Fatalf("CallSnapshot failed: %v", err)
	}

	status1 := client.Status()
	if status1.State != StateReady || status1.ConnectCount != 1 || status1.ReconnectCount != 0 || status1.CallCount != 1 || status1.ErrorCount != 0 {
		t.Fatalf("unexpected post-call client status: %+v", status1)
	}
}

func TestNewServerWithWorkersWindows(t *testing.T) {
	service := uniqueWinService("workers")
	server := NewServerWithWorkers(testWinRunDir, service, testWinServerConfig(), testWinHandler(), 3)
	if server == nil || server.inner == nil {
		t.Fatal("NewServerWithWorkers returned nil")
	}
}
