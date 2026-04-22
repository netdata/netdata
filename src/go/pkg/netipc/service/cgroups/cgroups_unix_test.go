//go:build unix

package cgroups

import (
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

const (
	testRunDirUnix   = "/tmp/nipc_go_cgroups_public"
	testAuthToken    = uint64(0xDEADBEEFCAFEBABE)
	testResponseSize = 65536
)

var unixServiceCounter atomic.Uint64

func uniqueUnixService(prefix string) string {
	return fmt.Sprintf("%s_%d_%d_%d", prefix, os.Getpid(), unixServiceCounter.Add(1), time.Now().UnixNano())
}

func ensureUnixRunDir(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll(testRunDirUnix, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
}

func cleanupUnix(service string) {
	_ = os.Remove(fmt.Sprintf("%s/%s.sock", testRunDirUnix, service))
}

func testUnixServerConfig() ServerConfig {
	return ServerConfig{
		SupportedProfiles:       protocol.ProfileBaseline,
		PreferredProfiles:       protocol.ProfileBaseline,
		MaxRequestBatchItems:    1,
		MaxResponsePayloadBytes: testResponseSize,
		AuthToken:               testAuthToken,
	}
}

func testUnixClientConfig() ClientConfig {
	return ClientConfig{
		SupportedProfiles:       protocol.ProfileBaseline,
		PreferredProfiles:       protocol.ProfileBaseline,
		MaxRequestBatchItems:    1,
		MaxResponsePayloadBytes: testResponseSize,
		AuthToken:               testAuthToken,
	}
}

func testUnixHandler() Handler {
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

type unixTestServer struct {
	server *Server
	done   chan struct{}
}

func startUnixTestServer(t *testing.T, service string) *unixTestServer {
	t.Helper()
	ensureUnixRunDir(t)
	cleanupUnix(service)

	s := NewServer(testRunDirUnix, service, testUnixServerConfig(), testUnixHandler())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = s.Run()
	}()

	time.Sleep(50 * time.Millisecond)
	return &unixTestServer{server: s, done: done}
}

func (ts *unixTestServer) stop() {
	ts.server.Stop()
	select {
	case <-ts.done:
	case <-time.After(2 * time.Second):
	}
}

func connectReadyUnix(t *testing.T, client *Client) {
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

func TestSnapshotRoundTripUnix(t *testing.T) {
	service := uniqueUnixService("snapshot")
	ts := startUnixTestServer(t, service)
	defer ts.stop()

	client := NewClient(testRunDirUnix, service, testUnixClientConfig())
	defer client.Close()
	connectReadyUnix(t, client)

	view, err := client.CallSnapshot()
	if err != nil {
		t.Fatalf("CallSnapshot failed: %v", err)
	}
	if view.ItemCount != 3 {
		t.Fatalf("ItemCount = %d, want 3", view.ItemCount)
	}
	if view.SystemdEnabled != 1 {
		t.Fatalf("SystemdEnabled = %d, want 1", view.SystemdEnabled)
	}
	if view.Generation != 42 {
		t.Fatalf("Generation = %d, want 42", view.Generation)
	}
	item0, err := view.Item(0)
	if err != nil {
		t.Fatalf("Item(0): %v", err)
	}
	if item0.Hash != 1001 || item0.Name.String() != "docker-abc123" {
		t.Fatalf("unexpected item0: %+v", item0)
	}
}

func TestCacheRoundTripUnix(t *testing.T) {
	service := uniqueUnixService("cache")
	ts := startUnixTestServer(t, service)
	defer ts.stop()

	cache := NewCache(testRunDirUnix, service, testUnixClientConfig())
	defer cache.Close()

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
	if !ok {
		t.Fatal("lookup failed")
	}
	if item.Path != "/sys/fs/cgroup/docker/abc123" {
		t.Fatalf("unexpected path: %q", item.Path)
	}

	status := cache.Status()
	if !status.Populated || status.ItemCount != 3 || status.Generation != 42 {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestClientNotReadyReturnsErrorUnix(t *testing.T) {
	service := uniqueUnixService("not_ready")
	cleanupUnix(service)

	client := NewClient(testRunDirUnix, service, testUnixClientConfig())
	defer client.Close()

	if _, err := client.CallSnapshot(); err != protocol.ErrBadLayout {
		t.Fatalf("CallSnapshot err = %v, want %v", err, protocol.ErrBadLayout)
	}
}
