package main

import (
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
		tcpDimCleanupCalls: socketDelta(snap.CallsTCPSendmsg, prev.CallsTCPSendmsg),
		tcpDimCleanupKbits: -kbDelta(snap.BytesTCPSendmsg, prev.BytesTCPSendmsg),
		tcpDimCleanupErr:   socketDelta(snap.ErrorTCPSendmsg, prev.ErrorTCPSendmsg),
		tcpDimSendmsgCalls: socketDelta(snap.CallsTCPCleanupRbuf, prev.CallsTCPCleanupRbuf),
		tcpDimSendmsgKbits: kbDelta(snap.BytesTCPCleanupRbuf, prev.BytesTCPCleanupRbuf),
		tcpDimSendmsgErr:   socketDelta(snap.ErrorTCPCleanupRbuf, prev.ErrorTCPCleanupRbuf),
		tcpCloseCalls:      socketDelta(snap.CallsTCPClose, prev.CallsTCPClose),
		tcpRetransmit:      socketDelta(snap.TCPRetransmit, prev.TCPRetransmit),
		tcpV4Conn:          socketDelta(snap.CallsTCPConnectIPv4, prev.CallsTCPConnectIPv4),
		tcpV6Conn:          socketDelta(snap.CallsTCPConnectIPv6, prev.CallsTCPConnectIPv6),
		udpRecvCalls:       socketDelta(snap.CallsUDPRecvmsg, prev.CallsUDPRecvmsg),
		udpSendCalls:       socketDelta(snap.CallsUDPSendmsg, prev.CallsUDPSendmsg),
		udpRecvKbits:       kbDelta(snap.BytesUDPRecvmsg, prev.BytesUDPRecvmsg),
		udpSendKbits:       -kbDelta(snap.BytesUDPSendmsg, prev.BytesUDPSendmsg),
		udpRecvErr:         socketDelta(snap.ErrorUDPRecvmsg, prev.ErrorUDPRecvmsg),
		udpSendErr:         socketDelta(snap.ErrorUDPSendmsg, prev.ErrorUDPSendmsg),
		inboundTCP:         socketDelta(snap.InboundConnTCP, prev.InboundConnTCP),
		inboundUDP:         socketDelta(snap.InboundConnUDP, prev.InboundConnUDP),
	}, true
}

// ---- rate-limited stderr ---------------------------------------------------
// (rateLimitedStderr and logPluginErr live in error_log.go; both socket and
// dns collectors use them to throttle stderr noise from persistent failures.)

// ---- collection loop -------------------------------------------------------

// runSocketGlobalCollector is the socket plugin collection loop.
// It runs in its own goroutine alongside cachestat.
// store may be nil when there are no apps/cgroups consumers for socket data.
// shouldPublish must be true when cachestat is not publishing to SHM; in that
// case the socket collector opens the shared segment and publishes each cycle
// so that cgroup.plugin can read socket data independently of cachestat.
func runSocketGlobalCollector(handle *SocketLegacyHandle, stop <-chan struct{}, updateEvery int, store *cachestatSharedMemoryStore, fnStore *socketFunctionStore, shouldPublish bool) {
	if handle == nil || handle.Runtime == nil {
		return
	}

	if updateEvery <= 0 {
		updateEvery = socketDefaultUpdateEvery
	}

	state := &socketGlobalState{}

	// Lazily-opened SHM publisher, used only when socket is the designated
	// publisher (cachestat is not running or has no apps/cgroups consumers).
	var shmPublisher *SharedPidMemoryPublisher
	if shouldPublish && store != nil {
		p, perr := NewSharedPidMemoryPublisher(socketDefaultPIDTableSize)
		if perr != nil {
			logPluginErr("socket.shm_open", "socket", "shared memory open", perr)
		} else {
			shmPublisher = p
			defer shmPublisher.Close()
		}
	}

	clearSocketApps := func() {
		if store == nil {
			return
		}

		// Snapshot failures mean this cycle has no trustworthy per-PID socket
		// data. Clear prior values so consumers see zeros, not stale activity.
		store.UpdateSocketApps(nil)
		if shmPublisher != nil {
			if err := store.Publish(shmPublisher); err != nil {
				logPluginErr("socket.publish", "socket", "shared memory publish", err)
			}
		}
	}

	collectAndPublish := func() {
		// Mark the socket module active before any early return so the SOCKET
		// SHM flag is set even when Snapshot or SnapshotPerPID fails.  Without
		// this, a transient BPF error prevents socket_ok from becoming true and
		// cgroup charts are never created for the cycle.
		if store != nil {
			store.MarkSocketActive()
		}

		snap, err := handle.Runtime.Snapshot(handle.MapsPerCore)
		if err != nil {
			logPluginErr("socket.snapshot", "socket", "snapshot", err)
			clearSocketApps()
			return
		}
		if publish, ok := state.Update(snap); ok {
			fnStore.update(publish)
		}

		if store != nil {
			pidEntries, pidErr := handle.Runtime.SnapshotPerPID()
			if pidErr != nil {
				logPluginErr("socket.snapshot_per_pid", "socket", "per-PID snapshot", pidErr)
				clearSocketApps()
			} else {
				store.UpdateSocketApps(pidEntries)
				if shmPublisher != nil {
					if err := store.Publish(shmPublisher); err != nil {
						logPluginErr("socket.publish", "socket", "shared memory publish", err)
					}
				}
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
