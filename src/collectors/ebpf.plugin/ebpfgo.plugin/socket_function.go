package main

import (
	"encoding/json"
	"sync"
)

const (
	socketFunctionName    = "network-protocols"
	socketFunctionHelp    = "Linux eBPF TCP and UDP statistics (IPv4 and IPv6 combined)"
	socketFunctionTimeout = 10
	socketFunctionTags    = "top"
	// HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE = (1<<0)|(1<<1) = 3
	socketFunctionAccess      = "0x00000003"
	socketFunctionPriority    = 100
	socketFunctionVersion     = 1
	socketFunctionUpdateEvery = 5
)

// socketFunctionStore holds the latest computed per-cycle socket metrics
// for on-demand network-protocols function calls via the agent framework.
// Updated by the collector goroutine; read by function handler goroutine.
type socketFunctionStore struct {
	mu          sync.RWMutex
	publish     socketGlobalPublish
	hasData     bool
	updateEvery int
}

func newSocketFunctionStore(updateEvery int) *socketFunctionStore {
	if updateEvery <= 0 {
		updateEvery = socketFunctionUpdateEvery
	}
	return &socketFunctionStore{updateEvery: updateEvery}
}

func (s *socketFunctionStore) update(p socketGlobalPublish) {
	s.mu.Lock()
	s.publish = p
	s.hasData = true
	s.mu.Unlock()
}

func (s *socketFunctionStore) snapshot() (socketGlobalPublish, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.publish, s.hasData
}

// buildNetworkProtocolsJSON produces the JSON table matching the FreeBSD network-protocols schema.
//
// Field names are semantic: Received = cleanup_rbuf data, Sent = sendmsg data.
// The BPF counter -> dimension mapping is pinned by
// TestBuildNetworkProtocolsJSON_BPFMapping so a kernel ABI rename silently
// breaks the test instead of silently flipping Sent/Received.
func buildNetworkProtocolsJSON(p socketGlobalPublish, updateEvery int, expires int64) (string, error) {
	// Convert per-interval deltas to per-second rates to match the declared column units.
	// Round instead of truncate: on low-traffic hosts a small delta (e.g. 5 calls
	// in 10 s) would floor to 0, making the entire table look empty.
	inv := uint64(updateEvery)
	if inv == 0 {
		inv = 1
	}
	// ratePS rounds a per-interval counter to a per-second rate.
	ratePS := func(v uint64) uint64 { return (v + inv/2) / inv }

	tcpReceived := ratePS(p.tcpDimReceivedCalls)
	tcpSent := ratePS(p.tcpDimSentCalls)
	tcpErrors := ratePS(p.tcpDimReceivedErr + p.tcpDimSentErr)
	tcpConnActive := ratePS(p.tcpV4Conn + p.tcpV6Conn)
	tcpConnPassive := ratePS(p.inboundTCP)
	tcpSegsTotal := ratePS(p.tcpDimReceivedCalls + p.tcpDimSentCalls + p.tcpCloseCalls)
	tcpSegsRetrans := ratePS(p.tcpRetransmit)

	udpReceived := ratePS(p.udpRecvCalls)
	udpSent := ratePS(p.udpSendCalls)
	udpErrors := ratePS(p.udpRecvErr + p.udpSendErr)
	udpConnPassive := ratePS(p.inboundUDP)

	resp := fnTableResponse{
		Status:      200,
		Type:        "table",
		UpdateEvery: updateEvery,
		HasHistory:  false,
		Help:        socketFunctionHelp,
		Data: [][]any{
			{"TCP", "IPv4+IPv6",
				tcpReceived, tcpSent, tcpErrors,
				tcpConnActive, uint64(0), tcpConnPassive, uint64(0),
				tcpSegsTotal, tcpSegsRetrans, uint64(0)},
			{"UDP", "IPv4+IPv6",
				udpReceived, udpSent, udpErrors,
				uint64(0), uint64(0), udpConnPassive, uint64(0),
				uint64(0), uint64(0), uint64(0)},
		},
		Columns: map[string]fnColumnDef{
			"Transport":         fnStrCol(0, "Transport Protocol", true, true),
			"Family":            fnStrCol(1, "IP Protocol Family", true, true),
			"Received":          fnIntCol(2, "Received (Segments/Datagrams)", "segments/datagrams/s"),
			"Sent":              fnIntCol(3, "Sent (Segments/Datagrams)", "segments/datagrams/s"),
			"Errors":            fnIntCol(4, "Errors (Failures/Rx Errors)", "errors/s"),
			"ConnActive":        fnIntCol(5, "Active Connections Opened", "opens/s"),
			"ConnEstablished":   fnIntCol(6, "Currently Established Connections", "connections"),
			"ConnPassive":       fnIntCol(7, "Passive Connections Opened", "opens/s"),
			"ConnReset":         fnIntCol(8, "Reset Connections", "resets"),
			"SegsTotal":         fnIntCol(9, "Total Segments", "segments/s"),
			"SegsRetransmitted": fnIntCol(10, "Retransmitted Segments", "segments/s"),
			"DatagramsNoPort":   fnIntCol(11, "Datagrams with No Port", "datagrams/s"),
		},
		DefaultSortColumn: "Received",
		Charts: map[string]fnChartDef{
			"Traffic": {
				Name:    "Traffic",
				Type:    "stacked-bar",
				Columns: []string{"Received", "Sent"},
			},
		},
		DefaultCharts: [][]string{{"Traffic", "Transport"}},
		GroupBy: map[string]fnGroupByDef{
			"Transport": {
				Name:    "Transport",
				Columns: []string{"Transport"},
			},
		},
		Expires: expires,
	}

	b, err := json.Marshal(resp)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
