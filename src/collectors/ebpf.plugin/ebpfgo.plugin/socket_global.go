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

// socketDelta returns the increment of a monotonically increasing BPF counter.
// Returns 0 when prev is zero (first read — avoids a spike from accumulated
// counter history) or when current is not greater than prev (counter reset or wrap).
func socketDelta(current, prev uint64) uint64 {
	if prev == 0 || current <= prev {
		return 0
	}
	return current - prev
}

// bytesToKbits converts byte count to kilobits (×8 ÷ 1000), matching the C plugin.
func bytesToKbits(b uint64) int64 {
	return int64(b * 8 / 1000)
}

// kbDelta is the frequent bytesToKbits(socketDelta(...)) combination.
func kbDelta(cur, prev uint64) int64 {
	return bytesToKbits(socketDelta(cur, prev))
}

func (s *socketGlobalState) Update(snap libbpfloader.SocketSnapshot) (socketGlobalPublish, bool) {
	if !s.initialized {
		s.prev = snap
		s.initialized = true
		return socketGlobalPublish{}, true
	}

	prev := s.prev
	s.prev = snap

	// Cross-map: "tcp_cleanup_rbuf" dim ← sendmsg data; "tcp_sendmsg" dim ← cleanup_rbuf data.
	return socketGlobalPublish{
		tcpDimCleanupCalls: socketDelta(snap.CallsTCPSendmsg,     prev.CallsTCPSendmsg),
		tcpDimCleanupKbits: -kbDelta(snap.BytesTCPSendmsg,        prev.BytesTCPSendmsg),
		tcpDimCleanupErr:   socketDelta(snap.ErrorTCPSendmsg,      prev.ErrorTCPSendmsg),
		tcpDimSendmsgCalls: socketDelta(snap.CallsTCPCleanupRbuf,  prev.CallsTCPCleanupRbuf),
		tcpDimSendmsgKbits: kbDelta(snap.BytesTCPCleanupRbuf,     prev.BytesTCPCleanupRbuf),
		tcpDimSendmsgErr:   socketDelta(snap.ErrorTCPCleanupRbuf,  prev.ErrorTCPCleanupRbuf),
		tcpCloseCalls:      socketDelta(snap.CallsTCPClose,         prev.CallsTCPClose),
		tcpRetransmit:      socketDelta(snap.TCPRetransmit,          prev.TCPRetransmit),
		tcpV4Conn:          socketDelta(snap.CallsTCPConnectIPv4,   prev.CallsTCPConnectIPv4),
		tcpV6Conn:          socketDelta(snap.CallsTCPConnectIPv6,   prev.CallsTCPConnectIPv6),
		udpRecvCalls:       socketDelta(snap.CallsUDPRecvmsg,        prev.CallsUDPRecvmsg),
		udpSendCalls:       socketDelta(snap.CallsUDPSendmsg,        prev.CallsUDPSendmsg),
		udpRecvKbits:       kbDelta(snap.BytesUDPRecvmsg,           prev.BytesUDPRecvmsg),
		udpSendKbits:       -kbDelta(snap.BytesUDPSendmsg,          prev.BytesUDPSendmsg),
		udpRecvErr:         socketDelta(snap.ErrorUDPRecvmsg,        prev.ErrorUDPRecvmsg),
		udpSendErr:         socketDelta(snap.ErrorUDPSendmsg,        prev.ErrorUDPSendmsg),
		inboundTCP:         socketDelta(snap.InboundConnTCP,         prev.InboundConnTCP),
		inboundUDP:         socketDelta(snap.InboundConnUDP,         prev.InboundConnUDP),
	}, true
}

// ---- rate-limited stderr ---------------------------------------------------

const socketErrorLogInterval = 60 * time.Second

var (
	socketErrorMu      sync.Mutex
	socketErrorLastLog = map[string]time.Time{}
)

func socketRateLimitedStderr(site, msg string) {
	now := time.Now()
	socketErrorMu.Lock()
	if last, ok := socketErrorLastLog[site]; ok && now.Sub(last) < socketErrorLogInterval {
		socketErrorMu.Unlock()
		return
	}
	socketErrorLastLog[site] = now
	socketErrorMu.Unlock()
	fmt.Fprint(os.Stderr, msg)
}

func socketLogErr(site, what string, err error) {
	socketRateLimitedStderr(site, fmt.Sprintf("ebpf-go.plugin: socket %s failed: %v\n", what, err))
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
			socketLogErr("socket.snapshot", "snapshot", err)
			return
		}
		if publish, ok := state.Update(snap); ok {
			fnStore.update(publish)
		}

		if store != nil {
			pidEntries, pidErr := handle.Runtime.SnapshotPerPID()
			if pidErr != nil {
				socketLogErr("socket.snapshot_per_pid", "per-PID snapshot", pidErr)
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
