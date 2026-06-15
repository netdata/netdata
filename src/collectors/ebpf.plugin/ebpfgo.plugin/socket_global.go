package main

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

// socketGlobalPublish holds the per-cycle delta values for the network-protocols function.
//
// TCP function/bandwidth/error charts use dimension IDs "tcp_cleanup_rbuf" and
// "tcp_sendmsg" in the same cross-mapped order as the C plugin: the field that
// feeds "tcp_cleanup_rbuf" carries tcp_sendmsg data and vice-versa.  This is a
// historic naming quirk preserved for chart-ID compatibility.
type socketGlobalPublish struct {
	// Feeds dimension "tcp_cleanup_rbuf" in tcp_functions, bandwidth, error.
	tcpDimCleanupCalls uint64 // delta of CallsTCPSendmsg
	tcpDimCleanupKbits int64  // -bytesToKbits(BytesTCPSendmsg)  [negative = sent]
	tcpDimCleanupErr   uint64 // delta of ErrorTCPSendmsg

	// Feeds dimension "tcp_sendmsg" in tcp_functions, bandwidth, error.
	tcpDimSendmsgCalls uint64 // delta of CallsTCPCleanupRbuf
	tcpDimSendmsgKbits int64  // +bytesToKbits(BytesTCPCleanupRbuf) [positive = recv]
	tcpDimSendmsgErr   uint64 // delta of ErrorTCPCleanupRbuf

	tcpCloseCalls uint64 // delta of CallsTCPClose
	tcpRetransmit uint64 // delta of TCPRetransmit
	tcpV4Conn     uint64 // delta of CallsTCPConnectIPv4
	tcpV6Conn     uint64 // delta of CallsTCPConnectIPv6
	udpRecvCalls  uint64 // delta of CallsUDPRecvmsg
	udpSendCalls  uint64 // delta of CallsUDPSendmsg
	udpRecvKbits  int64  // +bytesToKbits(BytesUDPRecvmsg)
	udpSendKbits  int64  // -bytesToKbits(BytesUDPSendmsg)     [negative = sent]
	udpRecvErr    uint64 // delta of ErrorUDPRecvmsg
	udpSendErr    uint64 // delta of ErrorUDPSendmsg
	inboundTCP    uint64 // delta of InboundConnTCP
	inboundUDP    uint64 // delta of InboundConnUDP
}

type socketGlobalState struct {
	initialized bool
	prev        libbpfloader.SocketSnapshot
}

// socketDelta returns abs(current - prev).  On the first non-zero read (prev==0)
// it returns 0 to avoid a false spike from the accumulated counter history.
func socketDelta(current, prev uint64) uint64 {
	if current == prev || prev == 0 {
		return 0
	}
	if current > prev {
		return current - prev
	}
	return prev - current
}

// bytesToKbits converts byte count to kilobits (×8 ÷ 1000), matching the C plugin.
func bytesToKbits(b uint64) int64 {
	return int64(b * 8 / 1000)
}

func (s *socketGlobalState) Update(snap libbpfloader.SocketSnapshot) (socketGlobalPublish, bool) {
	if !s.initialized {
		s.prev = snap
		s.initialized = true
		return socketGlobalPublish{}, true
	}

	p := socketGlobalPublish{}

	// TCP function chart (cross-mapped: "tcp_cleanup_rbuf" dim ← sendmsg data).
	p.tcpDimCleanupCalls = socketDelta(snap.CallsTCPSendmsg, s.prev.CallsTCPSendmsg)
	p.tcpDimSendmsgCalls = socketDelta(snap.CallsTCPCleanupRbuf, s.prev.CallsTCPCleanupRbuf)
	p.tcpCloseCalls = socketDelta(snap.CallsTCPClose, s.prev.CallsTCPClose)

	// TCP bandwidth chart (same cross-map: cleanup_rbuf dim = sent kbits negative).
	p.tcpDimCleanupKbits = -bytesToKbits(socketDelta(snap.BytesTCPSendmsg, s.prev.BytesTCPSendmsg))
	p.tcpDimSendmsgKbits = bytesToKbits(socketDelta(snap.BytesTCPCleanupRbuf, s.prev.BytesTCPCleanupRbuf))

	// TCP error chart (same cross-map).
	p.tcpDimCleanupErr = socketDelta(snap.ErrorTCPSendmsg, s.prev.ErrorTCPSendmsg)
	p.tcpDimSendmsgErr = socketDelta(snap.ErrorTCPCleanupRbuf, s.prev.ErrorTCPCleanupRbuf)

	p.tcpRetransmit = socketDelta(snap.TCPRetransmit, s.prev.TCPRetransmit)
	p.tcpV4Conn = socketDelta(snap.CallsTCPConnectIPv4, s.prev.CallsTCPConnectIPv4)
	p.tcpV6Conn = socketDelta(snap.CallsTCPConnectIPv6, s.prev.CallsTCPConnectIPv6)

	p.udpRecvCalls = socketDelta(snap.CallsUDPRecvmsg, s.prev.CallsUDPRecvmsg)
	p.udpSendCalls = socketDelta(snap.CallsUDPSendmsg, s.prev.CallsUDPSendmsg)
	p.udpRecvKbits = bytesToKbits(socketDelta(snap.BytesUDPRecvmsg, s.prev.BytesUDPRecvmsg))
	p.udpSendKbits = -bytesToKbits(socketDelta(snap.BytesUDPSendmsg, s.prev.BytesUDPSendmsg))
	p.udpRecvErr = socketDelta(snap.ErrorUDPRecvmsg, s.prev.ErrorUDPRecvmsg)
	p.udpSendErr = socketDelta(snap.ErrorUDPSendmsg, s.prev.ErrorUDPSendmsg)

	p.inboundTCP = socketDelta(snap.InboundConnTCP, s.prev.InboundConnTCP)
	p.inboundUDP = socketDelta(snap.InboundConnUDP, s.prev.InboundConnUDP)

	s.prev = snap
	return p, true
}

// ---- rate-limited stderr ---------------------------------------------------

const socketErrorLogInterval = 60 * time.Second

var (
	socketErrorMu      sync.Mutex
	socketErrorLastLog = map[string]time.Time{}
)

func socketRateLimitedStderr(site, msg string) {
	socketErrorMu.Lock()
	defer socketErrorMu.Unlock()
	now := time.Now()
	if last, ok := socketErrorLastLog[site]; ok && now.Sub(last) < socketErrorLogInterval {
		return
	}
	socketErrorLastLog[site] = now
	fmt.Fprint(os.Stderr, msg)
}

// ---- collection loop -------------------------------------------------------

// runSocketGlobalCollector is the socket plugin collection loop.
// It runs in its own goroutine alongside cachestat.
// store may be nil when there are no apps/cgroups consumers for socket data.
func runSocketGlobalCollector(handle *SocketLegacyHandle, stop <-chan struct{}, updateEvery int, store *cachestatSharedMemoryStore, fnStore *socketFunctionStore) {
	if handle == nil || handle.Runtime == nil {
		return
	}

	if updateEvery <= 0 {
		updateEvery = socketDefaultUpdateEvery
	}

	state := &socketGlobalState{}

	collectAndPublish := func() {
		snap, err := handle.Runtime.Snapshot(handle.MapsPerCore)
		if err != nil {
			socketRateLimitedStderr("socket.snapshot",
				fmt.Sprintf("ebpf-go.plugin: socket snapshot failed: %v\n", err))
			return
		}
		if publish, ok := state.Update(snap); ok {
			fnStore.update(publish)
		}

		if store != nil {
			pidEntries, pidErr := handle.Runtime.SnapshotPerPID()
			if pidErr != nil {
				socketRateLimitedStderr("socket.snapshot_per_pid",
					fmt.Sprintf("ebpf-go.plugin: socket per-PID snapshot failed: %v\n", pidErr))
			} else {
				store.UpdateSocketApps(pidEntries)
			}
		}
	}

	collectAndPublish()

	ticker := time.NewTicker(time.Duration(updateEvery) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
		}

		collectAndPublish()
	}
}
