//go:build linux || (freebsd && (amd64 || 386))

package raw

// SHM robustness reproduction tests (SOW-0040). SHM exists only on Linux.

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

func shmFiles(service string) []string {
	matches, _ := filepath.Glob(filepath.Join(testRunDir, service+"-*.ipcshm"))
	return matches
}

// K1: SHM server must release a session once its client leaves.

func TestRobustShmSessionsReleasedAfterCleanClose(t *testing.T) {
	svc := robustService("go_rob_shm_clean")
	rs := startRobustServer(svc, testUnixShmServerConfig(), protocol.MethodIncrement, robustIncrementDispatch(0), 2)
	defer cleanupAll(svc)

	// Each worker must exit and remove its region once its client closes;
	// a leaked session fails waitSessionExits loudly.
	var failures []string
	for i := 0; i < 5; i++ {
		if err := servedOnce(svc, testUnixShmClientConfig()); err != nil {
			failures = append(failures, fmt.Sprintf("client %d: %v", i, err))
		}
		waitSessionExits(rs.server, 1)
	}
	leftover := shmFiles(svc)
	rs.stopWithin(5 * time.Second)

	if len(failures) > 0 {
		t.Errorf("with 2 workers, sequential SHM clients must all be served: %v", failures)
	}
	if len(leftover) > 0 {
		t.Errorf("SHM regions leaked: %v", leftover)
	}
}

func TestRobustShmSessionsReleasedAfterClientCrash(t *testing.T) {
	svc := robustService("go_rob_shm_crash")
	rs := startRobustServer(svc, testUnixShmServerConfig(), protocol.MethodIncrement, robustIncrementDispatch(0), 2)
	defer cleanupAll(svc)

	for i := 0; i < 2; i++ {
		cmd := spawnHelper(t, "shm_client", svc)
		_ = cmd.Process.Kill() // SIGKILL
		_ = cmd.Wait()
	}
	// Both workers must notice their crashed clients and free the slots.
	waitSessionExits(rs.server, 2)
	served := servedOnce(svc, testUnixShmClientConfig())
	waitSessionExits(rs.server, 1)
	leftover := shmFiles(svc)
	rs.stopWithin(5 * time.Second)

	if served != nil {
		t.Errorf("after 2 crashed SHM clients on 2 workers, a new client must be served: %v", served)
	}
	if len(leftover) > 0 {
		t.Errorf("SHM regions leaked: %v", leftover)
	}
}

// N5: an SHM client must notice a crashed server instead of waiting out its
// whole timeout.

func TestRobustShmClientNoticesCrashedServer(t *testing.T) {
	clientNoticesCrashedServer(t, true)
}

// A server at worker capacity completes the handshake, then closes the
// session and removes the prepared SHM region. The client must not spend its
// attach retry budget (5 s) on a region that will never appear, and must keep
// SHM enabled for its next connect.
func TestRobustShmAttachStopsWhenServerClosesSession(t *testing.T) {
	svc := robustService("go_rob_attach_refused")
	rs := startRobustServer(svc, testUnixShmServerConfig(), protocol.MethodIncrement, robustIncrementDispatch(0), 1)
	defer cleanupAll(svc)

	holder := NewIncrementClient(testRunDir, svc, testUnixShmClientConfig())
	holder.Refresh()
	if !holder.Ready() {
		holder.Close()
		rs.stopWithin(5 * time.Second)
		t.Fatal("first client must hold the only worker")
	}

	refused := NewIncrementClient(testRunDir, svc, testUnixShmClientConfig())
	defer refused.Close()
	start := time.Now()
	refused.Refresh()
	elapsed := time.Since(start)

	holder.Close()
	waitSessionExits(rs.server, 1)
	refused.Refresh()
	ready, usesShm := refused.Ready(), refused.shm != nil
	refused.Close()
	rs.stopWithin(5 * time.Second)

	if elapsed >= time.Second {
		t.Errorf("refused connect must return within 1s (attach retry budget is 5s): %v", elapsed)
	}
	if !ready {
		t.Error("refused client must connect once the worker is free")
	}
	if !usesShm {
		t.Error("refused client must still use SHM")
	}
}
