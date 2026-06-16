package cgroups_lookup

import (
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

func cgroupsLookupTestConfig() ClientConfig {
	return ClientConfig{
		SupportedProfiles:             protocol.ProfileBaseline,
		MaxRequestPayloadBytes:        4096,
		MaxRequestBatchItems:          4,
		MaxResponsePayloadBytes:       4096,
		AuthToken:                     0xC600,
		MaxLogicalLookupItems:         16,
		MaxLogicalLookupSubcalls:      8,
		MaxLogicalLookupResponseBytes: 65536,
	}
}

func waitCgroupsLookupReady(t *testing.T, client *Client) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		client.Refresh()
		if client.Ready() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("cgroups lookup client did not become ready; status=%+v", client.Status())
}

func TestCgroupsLookupPublicWrapper(t *testing.T) {
	runDir := t.TempDir()
	service := fmt.Sprintf("go_cgroups_wrapper_%d", time.Now().UnixNano())
	handler := Handler{Handle: func(req *protocol.CgroupsLookupRequestView, builder *protocol.CgroupsLookupBuilder) bool {
		for i := uint32(0); i < req.ItemCount; i++ {
			path, err := req.Item(i)
			if err != nil {
				return false
			}
			if err := builder.Add(
				protocol.CgroupLookupKnown,
				protocol.OrchestratorK8s,
				path.Bytes(),
				[]byte("pod"),
				nil,
			); err != nil {
				return false
			}
		}
		return true
	}}

	serverConfig := ServerConfig(cgroupsLookupTestConfig())
	plainServer := NewServer(runDir, service+"_plain", serverConfig, handler)
	if plainServer == nil || plainServer.inner == nil {
		t.Fatal("NewServer returned nil")
	}
	plainServer.Stop()

	server := NewServerWithWorkers(runDir, service, serverConfig, handler, 2)
	done := make(chan error, 1)
	go func() {
		done <- server.Run()
	}()
	defer func() {
		server.Stop()
		if err := <-done; err != nil {
			t.Fatalf("server run: %v", err)
		}
	}()

	client := NewClient(runDir, service, cgroupsLookupTestConfig())
	defer client.Close()
	client.SetCallTimeout(500)
	client.Abort()
	client.ClearAbort()
	waitCgroupsLookupReady(t, client)
	if client.Status().State != StateReady {
		t.Fatalf("status after ready = %+v", client.Status())
	}

	view, err := client.Call([][]byte{[]byte("/a")})
	if err != nil {
		t.Fatalf("call cgroups lookup: %v", err)
	}
	item, err := view.Item(0)
	if err != nil {
		t.Fatalf("cgroups item: %v", err)
	}
	if item.Path.String() != "/a" || item.Name.String() != "pod" {
		t.Fatalf("cgroups item = %+v", item)
	}

	view, err = client.CallWithTimeout([][]byte{[]byte("/b")}, 500)
	if err != nil {
		t.Fatalf("call cgroups lookup with timeout: %v", err)
	}
	item, err = view.Item(0)
	if err != nil || item.Path.String() != "/b" {
		t.Fatalf("cgroups timeout item = %+v err=%v", item, err)
	}
}
