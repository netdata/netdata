package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
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

// socketFunctionStore holds the latest computed per-cycle socket metrics so the
// stdin dispatcher can serve on-demand network-protocols function calls.
// Updated by the collector goroutine; read by the stdin dispatcher goroutine.
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

// runStdinDispatcher reads os.Stdin for FUNCTION calls and dispatches them.
// It exits when stdin is closed or "QUIT" is received, and always calls
// closeStop so collector goroutines shut down on both paths.
// fnStore may be nil when the socket module is disabled.
func runStdinDispatcher(api *netdataapi.API, fnStore *socketFunctionStore, closeStop func()) {
	defer closeStop()
	rd := bufio.NewReaderSize(os.Stdin, 1<<20)
	inPayload := false
	var payloadUID, payloadName string
	for {
		line, err := readBoundedLine(rd)
		if err != nil {
			return
		}
		if line == "" {
			continue // empty or oversized line — skip
		}

		if inPayload {
			if line == "FUNCTION_PAYLOAD_END" {
				inPayload = false
				if payloadUID != "" {
					dispatchSocketFunction(api, fnStore, payloadUID, payloadName)
					payloadUID, payloadName = "", ""
				}
			}
			// All payload body lines — including a bare "QUIT" — are data, not commands.
			continue
		}

		switch {
		case line == "QUIT":
			return
		case strings.HasPrefix(line, "FUNCTION_PAYLOAD "):
			// Payload-carrying call: consume the body block before dispatching.
			// Reuse parseMinimalFunctionLine by replacing the keyword prefix so
			// the uid/name positions are identical to a plain FUNCTION line.
			uid, name := parseMinimalFunctionLine("FUNCTION" + line[len("FUNCTION_PAYLOAD"):])
			if uid != "" {
				payloadUID = uid
				payloadName = name
			}
			inPayload = true
		case strings.HasPrefix(line, "FUNCTION "):
			uid, name := parseMinimalFunctionLine(line)
			if uid == "" {
				continue
			}
			dispatchSocketFunction(api, fnStore, uid, name)
		}
	}
}

// dispatchSocketFunction routes a parsed function call to the right handler.
func dispatchSocketFunction(api *netdataapi.API, fnStore *socketFunctionStore, uid, name string) {
	switch name {
	case socketFunctionName:
		if fnStore != nil {
			handleNetworkProtocols(api, fnStore, uid)
		} else {
			sendFunctionError(api, uid, 503, "network-protocols collector not running")
		}
	default:
		sendFunctionError(api, uid, 404, "unknown function: "+name)
	}
}

// readBoundedLine reads one complete line from rd. Lines exceeding 1 MiB are
// discarded — all chunks are consumed but ("", nil) is returned — so the
// dispatcher continues with the next request instead of shutting down.
// ("", err) signals EOF or a real I/O error and the caller should exit.
func readBoundedLine(rd *bufio.Reader) (string, error) {
	const limit = 1 << 20
	var sb strings.Builder
	accumulated := 0
	oversized := false
	for {
		part, isPrefix, err := rd.ReadLine()
		if err != nil {
			return "", err
		}
		accumulated += len(part)
		if accumulated > limit {
			oversized = true
		}
		if !oversized {
			sb.Write(part)
		}
		if !isPrefix {
			if oversized {
				return "", nil
			}
			return sb.String(), nil
		}
	}
}

// parseMinimalFunctionLine extracts uid and function name from a FUNCTION call line.
// Format: FUNCTION uid timeout "name [args]" access source
func parseMinimalFunctionLine(line string) (uid, name string) {
	r := csv.NewReader(strings.NewReader(line))
	r.Comma = ' '
	r.LazyQuotes = true
	parts, err := r.Read()
	if err != nil || len(parts) < 5 || parts[0] != "FUNCTION" {
		return "", ""
	}
	uid = parts[1]
	nameAndArgs := strings.SplitN(parts[3], " ", 2)
	return uid, nameAndArgs[0]
}

// handleNetworkProtocols builds and writes the network-protocols function response.
func handleNetworkProtocols(api *netdataapi.API, fnStore *socketFunctionStore, uid string) {
	p, hasData := fnStore.snapshot()
	if !hasData {
		sendFunctionError(api, uid, 503, "network-protocols: data not yet available")
		return
	}

	now := time.Now().Unix()
	expires := now + int64(fnStore.updateEvery)

	payload, err := buildNetworkProtocolsJSON(p, fnStore.updateEvery, expires)
	if err != nil {
		sendFunctionError(api, uid, 500, fmt.Sprintf("json marshal error: %v", err))
		return
	}

	pluginOutputMu.Lock()
	api.FUNCRESULT(netdataapi.FunctionResult{
		UID:             uid,
		Code:            "200",
		ContentType:     "application/json",
		ExpireTimestamp: strconv.FormatInt(expires, 10),
		Payload:         payload,
	})
	pluginOutputMu.Unlock()
}

func sendFunctionError(api *netdataapi.API, uid string, code int, msg string) {
	b, _ := json.Marshal(struct {
		Status  int    `json:"status"`
		Message string `json:"message"`
	}{Status: code, Message: msg})
	pluginOutputMu.Lock()
	api.FUNCRESULT(netdataapi.FunctionResult{
		UID:             uid,
		Code:            strconv.Itoa(code),
		ContentType:     "application/json",
		ExpireTimestamp: strconv.FormatInt(time.Now().Unix(), 10),
		Payload:         string(b),
	})
	pluginOutputMu.Unlock()
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
