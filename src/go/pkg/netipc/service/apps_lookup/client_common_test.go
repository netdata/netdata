package apps_lookup

import (
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

func appsLookupTestConfig() ClientConfig {
	return ClientConfig{
		SupportedProfiles:             protocol.ProfileBaseline,
		MaxRequestPayloadBytes:        4096,
		MaxRequestBatchItems:          4,
		MaxResponsePayloadBytes:       4096,
		AuthToken:                     0xA11CE,
		MaxLogicalLookupItems:         16,
		MaxLogicalLookupSubcalls:      8,
		MaxLogicalLookupResponseBytes: 65536,
	}
}

func waitAppsLookupReady(t *testing.T, client *Client) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		client.Refresh()
		if client.Ready() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("apps lookup client did not become ready; status=%+v", client.Status())
}

func TestAppsLookupPublicWrapper(t *testing.T) {
	runDir := t.TempDir()
	service := fmt.Sprintf("go_apps_wrapper_%d", time.Now().UnixNano())
	handler := Handler{Handle: func(req *protocol.AppsLookupRequestView, builder *protocol.AppsLookupBuilder) bool {
		for i := uint32(0); i < req.ItemCount; i++ {
			pid, err := req.Item(i)
			if err != nil {
				return false
			}
			if err := builder.Add(
				protocol.PidLookupKnown,
				protocol.AppsCgroupHostRoot,
				0,
				pid,
				0,
				0,
				0,
				[]byte("proc"),
				nil,
				nil,
				nil,
			); err != nil {
				return false
			}
		}
		return true
	}}

	serverConfig := ServerConfig(appsLookupTestConfig())
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

	client := NewClient(runDir, service, appsLookupTestConfig())
	defer client.Close()
	client.SetCallTimeout(500)
	client.Abort()
	client.ClearAbort()
	waitAppsLookupReady(t, client)
	if client.Status().State != StateReady {
		t.Fatalf("status after ready = %+v", client.Status())
	}

	view, err := client.Call([]uint32{11})
	if err != nil {
		t.Fatalf("call apps lookup: %v", err)
	}
	item, err := view.Item(0)
	if err != nil {
		t.Fatalf("apps item: %v", err)
	}
	if item.Pid != 11 || item.Comm.String() != "proc" {
		t.Fatalf("apps item = %+v", item)
	}

	view, err = client.CallWithTimeout([]uint32{12}, 500)
	if err != nil {
		t.Fatalf("call apps lookup with timeout: %v", err)
	}
	item, err = view.Item(0)
	if err != nil || item.Pid != 12 {
		t.Fatalf("apps timeout item = %+v err=%v", item, err)
	}
}
