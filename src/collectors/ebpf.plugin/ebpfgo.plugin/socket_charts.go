// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
)

const (
	socketGlobalGroup  = "ip"
	socketGlobalFamily = "ebpf_socket"
	socketGlobalModule = "socket"
	socketGlobalPlugin = "ebpf-go.plugin"

	socketGlobalPrioInboundConn   = 21500
	socketGlobalPrioTCPOutbound   = 21501
	socketGlobalPrioTCPFunctions  = 21502
	socketGlobalPrioTCPBandwidth  = 21503
	socketGlobalPrioTCPErrors     = 21504
	socketGlobalPrioTCPRetransmit = 21505
	socketGlobalPrioUDPFunctions  = 21506
	socketGlobalPrioUDPBandwidth  = 21507
	socketGlobalPrioUDPErrors     = 21508
)

var socketGlobalChartsOnce sync.Once

// createSocketGlobalCharts registers the nine host-level ip.* charts once.
// It must be called before the first writeCharts call.
func createSocketGlobalCharts(api *netdataapi.API, updateEvery int) {
	socketGlobalChartsOnce.Do(func() {
		if api == nil {
			return
		}

		ue := updateEvery
		if ue <= 0 {
			ue = 1
		}

		pluginOutputMu.Lock()
		defer pluginOutputMu.Unlock()

		api.HOST("")

		emitChart := func(id, title, units, context, chartType string, priority int, dims []netdataapi.DimensionOpts) {
			api.CHART(netdataapi.ChartOpts{
				TypeID:      socketGlobalGroup,
				ID:          id,
				Title:       title,
				Units:       units,
				Family:      socketGlobalFamily,
				Context:     context,
				ChartType:   chartType,
				Priority:    priority,
				UpdateEvery: updateEvery,
				Plugin:      socketGlobalPlugin,
				Module:      socketGlobalModule,
			})
			for _, d := range dims {
				api.DIMENSION(d)
			}
		}

		// rate returns a per-second absolute dimension: value is a per-interval delta,
		// so divisor = update_every converts it to a rate.
		rate := func(id string) netdataapi.DimensionOpts {
			return netdataapi.DimensionOpts{ID: id, Name: id, Algorithm: "absolute", Multiplier: 1, Divisor: ue}
		}
		// bw returns a kilobits/s dimension from raw bytes per interval:
		// bytes/interval * 8 / (ue * 1000) = kilobits/s.
		bw := func(id string) netdataapi.DimensionOpts {
			return netdataapi.DimensionOpts{ID: id, Name: id, Algorithm: "absolute", Multiplier: 8, Divisor: ue * 1000}
		}

		emitChart("inbound_conn", "Inbound connections", "connections/s",
			"ip.inbound_conn", "line", socketGlobalPrioInboundConn,
			[]netdataapi.DimensionOpts{rate("connected_tcp"), rate("connected_udp")})

		emitChart("tcp_outbound_conn", "TCP outbound connections", "connections/s",
			"ip.tcp_outbound_conn", "line", socketGlobalPrioTCPOutbound,
			[]netdataapi.DimensionOpts{rate("received")})

		emitChart("tcp_functions", "Calls to internal functions", "calls/s",
			"ip.tcp_functions", "stacked", socketGlobalPrioTCPFunctions,
			[]netdataapi.DimensionOpts{rate("received"), rate("send"), rate("closed")})

		emitChart("total_tcp_bandwidth", "TCP bandwidth", "kilobits/s",
			"ip.total_tcp_bandwidth", "stacked", socketGlobalPrioTCPBandwidth,
			[]netdataapi.DimensionOpts{bw("received"), bw("send")})

		emitChart("tcp_error", "TCP errors", "calls/s",
			"ip.tcp_error", "stacked", socketGlobalPrioTCPErrors,
			[]netdataapi.DimensionOpts{rate("received"), rate("send")})

		emitChart("tcp_retransmit", "Packages retransmitted", "calls/s",
			"ip.tcp_retransmit", "line", socketGlobalPrioTCPRetransmit,
			[]netdataapi.DimensionOpts{rate("retransmitted")})

		emitChart("udp_functions", "UDP calls", "calls/s",
			"ip.udp_functions", "stacked", socketGlobalPrioUDPFunctions,
			[]netdataapi.DimensionOpts{rate("received"), rate("send")})

		emitChart("total_udp_bandwidth", "UDP bandwidth", "kilobits/s",
			"ip.total_udp_bandwidth", "stacked", socketGlobalPrioUDPBandwidth,
			[]netdataapi.DimensionOpts{bw("received"), bw("send")})

		emitChart("udp_error", "UDP errors", "calls/s",
			"ip.udp_error", "stacked", socketGlobalPrioUDPErrors,
			[]netdataapi.DimensionOpts{rate("received"), rate("send")})
	})
}

// writeCharts emits one BEGIN/SET.../END block per ip.* chart.
// Values are raw per-interval deltas; the DIMENSION multiplier/divisor converts them to rates.
func (p socketGlobalPublish) writeCharts(api *netdataapi.API, usecSince int) {
	if api == nil {
		return
	}

	pluginOutputMu.Lock()
	defer pluginOutputMu.Unlock()

	api.BEGIN(socketGlobalGroup, "inbound_conn", usecSince)
	api.SET("connected_tcp", int64(p.inboundTCP))
	api.SET("connected_udp", int64(p.inboundUDP))
	api.END()

	api.BEGIN(socketGlobalGroup, "tcp_outbound_conn", usecSince)
	api.SET("received", int64(p.tcpV4Conn+p.tcpV6Conn))
	api.END()

	api.BEGIN(socketGlobalGroup, "tcp_functions", usecSince)
	api.SET("received", int64(p.tcpDimReceivedCalls))
	api.SET("send", int64(p.tcpDimSentCalls))
	api.SET("closed", int64(p.tcpCloseCalls))
	api.END()

	// Raw bytes; multiplier/divisor in DIMENSION converts to kilobits/s.
	api.BEGIN(socketGlobalGroup, "total_tcp_bandwidth", usecSince)
	api.SET("received", int64(p.tcpBytesReceived))
	api.SET("send", int64(p.tcpBytesSent))
	api.END()

	api.BEGIN(socketGlobalGroup, "tcp_error", usecSince)
	api.SET("received", int64(p.tcpDimReceivedErr))
	api.SET("send", int64(p.tcpDimSentErr))
	api.END()

	api.BEGIN(socketGlobalGroup, "tcp_retransmit", usecSince)
	api.SET("retransmitted", int64(p.tcpRetransmit))
	api.END()

	api.BEGIN(socketGlobalGroup, "udp_functions", usecSince)
	api.SET("received", int64(p.udpRecvCalls))
	api.SET("send", int64(p.udpSendCalls))
	api.END()

	// Raw bytes; multiplier/divisor in DIMENSION converts to kilobits/s.
	api.BEGIN(socketGlobalGroup, "total_udp_bandwidth", usecSince)
	api.SET("received", int64(p.udpBytesReceived))
	api.SET("send", int64(p.udpBytesSent))
	api.END()

	api.BEGIN(socketGlobalGroup, "udp_error", usecSince)
	api.SET("received", int64(p.udpRecvErr))
	api.SET("send", int64(p.udpSendErr))
	api.END()
}
