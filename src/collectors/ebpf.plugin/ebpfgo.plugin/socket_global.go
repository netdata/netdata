package main

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

const (
	socketGlobalGroup  = "ip"
	socketGlobalFamily = "kernel"
	socketGlobalModule = "socket"
	socketGlobalPlugin = "ebpf-go.plugin"
)

// socketGlobalPublish holds the per-cycle values ready for chart emission.
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

	tcpCloseCalls   uint64 // delta of CallsTCPClose
	tcpRetransmit   uint64 // delta of TCPRetransmit
	tcpV4Conn       uint64 // delta of CallsTCPConnectIPv4
	tcpV6Conn       uint64 // delta of CallsTCPConnectIPv6
	udpRecvCalls    uint64 // delta of CallsUDPRecvmsg
	udpSendCalls    uint64 // delta of CallsUDPSendmsg
	udpRecvKbits    int64  // +bytesToKbits(BytesUDPRecvmsg)
	udpSendKbits    int64  // -bytesToKbits(BytesUDPSendmsg)     [negative = sent]
	udpRecvErr      uint64 // delta of ErrorUDPRecvmsg
	udpSendErr      uint64 // delta of ErrorUDPSendmsg
	inboundTCP      uint64 // InboundConnTCP (raw cumulative; INCREMENTAL chart)
	inboundUDP      uint64 // InboundConnUDP (raw cumulative; INCREMENTAL chart)
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
	p := socketGlobalPublish{
		inboundTCP: snap.InboundConnTCP,
		inboundUDP: snap.InboundConnUDP,
	}

	if !s.initialized {
		s.prev = snap
		s.initialized = true
		return p, true
	}

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

	s.prev = snap
	return p, true
}

// ---- chart creation --------------------------------------------------------

var socketGlobalChartsOnce sync.Once
var socketStdoutMutex sync.Mutex

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

func createSocketGlobalCharts(api *netdataapi.API, updateEvery int) {
	socketGlobalChartsOnce.Do(func() {
		socketStdoutMutex.Lock()
		defer socketStdoutMutex.Unlock()
		emitSocketGlobalCharts(api, updateEvery)
	})
}

func emitSocketGlobalCharts(api *netdataapi.API, updateEvery int) {
	if api == nil {
		return
	}

	order := 21070 // NETDATA_SOCKET_CHART_ORDER_BASE

	// inbound_conn
	api.CHART(netdataapi.ChartOpts{
		TypeID: socketGlobalGroup, ID: "inbound_conn",
		Title: "Inbound connections.", Units: "connections/s",
		Family: socketGlobalFamily, Context: "ip.inbound_conn",
		ChartType: "line", Priority: order, UpdateEvery: updateEvery,
		Plugin: socketGlobalPlugin, Module: socketGlobalModule,
	})
	api.DIMENSION(netdataapi.DimensionOpts{ID: "inet_csk_accept_tcp", Name: "connected_tcp", Algorithm: "incremental", Multiplier: 1, Divisor: 1})
	api.DIMENSION(netdataapi.DimensionOpts{ID: "inet_csk_accept_udp", Name: "connected_udp", Algorithm: "incremental", Multiplier: 1, Divisor: 1})
	order++

	// tcp_outbound_conn
	api.CHART(netdataapi.ChartOpts{
		TypeID: socketGlobalGroup, ID: "tcp_outbound_conn",
		Title: "TCP outbound connections.", Units: "connections/s",
		Family: socketGlobalFamily, Context: "ip.tcp_outbound_conn",
		ChartType: "line", Priority: order, UpdateEvery: updateEvery,
		Plugin: socketGlobalPlugin, Module: socketGlobalModule,
	})
	api.DIMENSION(netdataapi.DimensionOpts{ID: "tcp_connect_v4", Name: "connected_V4", Algorithm: "absolute", Multiplier: 1, Divisor: 1})
	api.DIMENSION(netdataapi.DimensionOpts{ID: "tcp_connect_v6", Name: "connected_V6", Algorithm: "absolute", Multiplier: 1, Divisor: 1})
	order++

	// tcp_functions
	api.CHART(netdataapi.ChartOpts{
		TypeID: socketGlobalGroup, ID: "tcp_functions",
		Title: "Calls to internal functions", Units: "calls/s",
		Family: socketGlobalFamily, Context: "ip.tcp_functions",
		ChartType: "line", Priority: order, UpdateEvery: updateEvery,
		Plugin: socketGlobalPlugin, Module: socketGlobalModule,
	})
	api.DIMENSION(netdataapi.DimensionOpts{ID: "tcp_cleanup_rbuf", Name: "received", Algorithm: "absolute", Multiplier: 1, Divisor: 1})
	api.DIMENSION(netdataapi.DimensionOpts{ID: "tcp_sendmsg", Name: "sent", Algorithm: "absolute", Multiplier: 1, Divisor: 1})
	api.DIMENSION(netdataapi.DimensionOpts{ID: "tcp_close", Name: "close", Algorithm: "absolute", Multiplier: 1, Divisor: 1})
	order++

	// total_tcp_bandwidth
	api.CHART(netdataapi.ChartOpts{
		TypeID: socketGlobalGroup, ID: "total_tcp_bandwidth",
		Title: "TCP bandwidth", Units: "kilobits/s",
		Family: socketGlobalFamily, Context: "ip.total_tcp_bandwidth",
		ChartType: "line", Priority: order, UpdateEvery: updateEvery,
		Plugin: socketGlobalPlugin, Module: socketGlobalModule,
	})
	api.DIMENSION(netdataapi.DimensionOpts{ID: "tcp_cleanup_rbuf", Name: "received", Algorithm: "absolute", Multiplier: 1, Divisor: 1})
	api.DIMENSION(netdataapi.DimensionOpts{ID: "tcp_sendmsg", Name: "sent", Algorithm: "absolute", Multiplier: 1, Divisor: 1})
	order++

	// tcp_error
	api.CHART(netdataapi.ChartOpts{
		TypeID: socketGlobalGroup, ID: "tcp_error",
		Title: "TCP errors", Units: "calls/s",
		Family: socketGlobalFamily, Context: "ip.tcp_error",
		ChartType: "line", Priority: order, UpdateEvery: updateEvery,
		Plugin: socketGlobalPlugin, Module: socketGlobalModule,
	})
	api.DIMENSION(netdataapi.DimensionOpts{ID: "tcp_cleanup_rbuf", Name: "received", Algorithm: "absolute", Multiplier: 1, Divisor: 1})
	api.DIMENSION(netdataapi.DimensionOpts{ID: "tcp_sendmsg", Name: "sent", Algorithm: "absolute", Multiplier: 1, Divisor: 1})
	order++

	// tcp_retransmit
	api.CHART(netdataapi.ChartOpts{
		TypeID: socketGlobalGroup, ID: "tcp_retransmit",
		Title: "Packages retransmitted", Units: "calls/s",
		Family: socketGlobalFamily, Context: "ip.tcp_retransmit",
		ChartType: "line", Priority: order, UpdateEvery: updateEvery,
		Plugin: socketGlobalPlugin, Module: socketGlobalModule,
	})
	api.DIMENSION(netdataapi.DimensionOpts{ID: "tcp_retransmit_skb", Name: "retransmitted", Algorithm: "absolute", Multiplier: 1, Divisor: 1})
	order++

	// udp_functions
	api.CHART(netdataapi.ChartOpts{
		TypeID: socketGlobalGroup, ID: "udp_functions",
		Title: "UDP calls", Units: "calls/s",
		Family: socketGlobalFamily, Context: "ip.udp_functions",
		ChartType: "line", Priority: order, UpdateEvery: updateEvery,
		Plugin: socketGlobalPlugin, Module: socketGlobalModule,
	})
	api.DIMENSION(netdataapi.DimensionOpts{ID: "udp_recvmsg", Name: "received", Algorithm: "absolute", Multiplier: 1, Divisor: 1})
	api.DIMENSION(netdataapi.DimensionOpts{ID: "udp_sendmsg", Name: "sent", Algorithm: "absolute", Multiplier: 1, Divisor: 1})
	order++

	// total_udp_bandwidth
	api.CHART(netdataapi.ChartOpts{
		TypeID: socketGlobalGroup, ID: "total_udp_bandwidth",
		Title: "UDP bandwidth", Units: "kilobits/s",
		Family: socketGlobalFamily, Context: "ip.total_udp_bandwidth",
		ChartType: "line", Priority: order, UpdateEvery: updateEvery,
		Plugin: socketGlobalPlugin, Module: socketGlobalModule,
	})
	api.DIMENSION(netdataapi.DimensionOpts{ID: "udp_recvmsg", Name: "received", Algorithm: "absolute", Multiplier: 1, Divisor: 1})
	api.DIMENSION(netdataapi.DimensionOpts{ID: "udp_sendmsg", Name: "sent", Algorithm: "absolute", Multiplier: 1, Divisor: 1})
	order++

	// udp_error
	api.CHART(netdataapi.ChartOpts{
		TypeID: socketGlobalGroup, ID: "udp_error",
		Title: "UDP errors", Units: "calls/s",
		Family: socketGlobalFamily, Context: "ip.udp_error",
		ChartType: "line", Priority: order, UpdateEvery: updateEvery,
		Plugin: socketGlobalPlugin, Module: socketGlobalModule,
	})
	api.DIMENSION(netdataapi.DimensionOpts{ID: "udp_recvmsg", Name: "received", Algorithm: "absolute", Multiplier: 1, Divisor: 1})
	api.DIMENSION(netdataapi.DimensionOpts{ID: "udp_sendmsg", Name: "sent", Algorithm: "absolute", Multiplier: 1, Divisor: 1})
}

// ---- chart emission --------------------------------------------------------

func (p socketGlobalPublish) write(api *netdataapi.API, usecSince int) {
	if api == nil {
		return
	}

	socketStdoutMutex.Lock()
	defer socketStdoutMutex.Unlock()

	api.BEGIN(socketGlobalGroup, "inbound_conn", usecSince)
	api.SET("inet_csk_accept_tcp", int64(p.inboundTCP))
	api.SET("inet_csk_accept_udp", int64(p.inboundUDP))
	api.END()

	api.BEGIN(socketGlobalGroup, "tcp_outbound_conn", usecSince)
	api.SET("tcp_connect_v4", int64(p.tcpV4Conn))
	api.SET("tcp_connect_v6", int64(p.tcpV6Conn))
	api.END()

	api.BEGIN(socketGlobalGroup, "tcp_functions", usecSince)
	api.SET("tcp_cleanup_rbuf", int64(p.tcpDimCleanupCalls))
	api.SET("tcp_sendmsg", int64(p.tcpDimSendmsgCalls))
	api.SET("tcp_close", int64(p.tcpCloseCalls))
	api.END()

	api.BEGIN(socketGlobalGroup, "total_tcp_bandwidth", usecSince)
	api.SET("tcp_cleanup_rbuf", p.tcpDimCleanupKbits)
	api.SET("tcp_sendmsg", p.tcpDimSendmsgKbits)
	api.END()

	api.BEGIN(socketGlobalGroup, "tcp_error", usecSince)
	api.SET("tcp_cleanup_rbuf", int64(p.tcpDimCleanupErr))
	api.SET("tcp_sendmsg", int64(p.tcpDimSendmsgErr))
	api.END()

	api.BEGIN(socketGlobalGroup, "tcp_retransmit", usecSince)
	api.SET("tcp_retransmit_skb", int64(p.tcpRetransmit))
	api.END()

	api.BEGIN(socketGlobalGroup, "udp_functions", usecSince)
	api.SET("udp_recvmsg", int64(p.udpRecvCalls))
	api.SET("udp_sendmsg", int64(p.udpSendCalls))
	api.END()

	api.BEGIN(socketGlobalGroup, "total_udp_bandwidth", usecSince)
	api.SET("udp_recvmsg", p.udpRecvKbits)
	api.SET("udp_sendmsg", p.udpSendKbits)
	api.END()

	api.BEGIN(socketGlobalGroup, "udp_error", usecSince)
	api.SET("udp_recvmsg", int64(p.udpRecvErr))
	api.SET("udp_sendmsg", int64(p.udpSendErr))
	api.END()
}

// ---- collection loop -------------------------------------------------------

// runSocketGlobalCollector is the socket plugin collection loop.
// It runs in its own goroutine alongside cachestat.
// store may be nil when there are no apps/cgroups consumers for socket data.
func runSocketGlobalCollector(api *netdataapi.API, handle *SocketLegacyHandle, stop <-chan struct{}, updateEvery int, store *cachestatSharedMemoryStore) {
	if handle == nil || handle.Runtime == nil {
		return
	}

	if updateEvery <= 0 {
		updateEvery = socketDefaultUpdateEvery
	}

	createSocketGlobalCharts(api, updateEvery)

	state := &socketGlobalState{}
	lastCollection := time.Now()

	collectAndPublish := func(usecSince int) {
		snap, err := handle.Runtime.Snapshot(handle.MapsPerCore)
		if err != nil {
			socketRateLimitedStderr("socket.snapshot",
				fmt.Sprintf("ebpf-go.plugin: socket snapshot failed: %v\n", err))
			return
		}
		if publish, ok := state.Update(snap); ok {
			publish.write(api, usecSince)
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

	collectAndPublish(0)
	lastCollection = time.Now()

	ticker := time.NewTicker(time.Duration(updateEvery) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
		}

		now := time.Now()
		usecSince := int(now.Sub(lastCollection).Microseconds())
		if usecSince < 0 {
			usecSince = 0
		}
		lastCollection = now
		collectAndPublish(usecSince)
	}
}
