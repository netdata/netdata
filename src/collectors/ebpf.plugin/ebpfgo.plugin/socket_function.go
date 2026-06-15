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
	socketFunctionName        = "network-protocols"
	socketFunctionHelp        = "Linux eBPF TCP and UDP statistics (IPv4 and IPv6 combined)"
	socketFunctionTimeout     = 10
	socketFunctionTags        = "top"
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
// It exits when stdin is closed or "QUIT" is received.
// closeStop is called on QUIT so that all collector goroutines also shut down.
func runStdinDispatcher(api *netdataapi.API, fnStore *socketFunctionStore, closeStop func()) {
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "QUIT":
			closeStop()
			return
		case strings.HasPrefix(line, "FUNCTION "):
			uid, name := parseMinimalFunctionLine(line)
			if uid == "" {
				continue
			}
			if name == socketFunctionName {
				handleNetworkProtocols(api, fnStore, uid)
			} else {
				sendFunctionError(api, uid, 404, "unknown function: "+name)
			}
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
	if err != nil || len(parts) < 4 {
		return "", ""
	}
	uid = parts[1]
	nameAndArgs := strings.SplitN(parts[3], " ", 2)
	return uid, nameAndArgs[0]
}

// handleNetworkProtocols builds and writes the network-protocols function response.
func handleNetworkProtocols(api *netdataapi.API, fnStore *socketFunctionStore, uid string) {
	p, _ := fnStore.snapshot()

	now := time.Now().Unix()
	expires := now + int64(fnStore.updateEvery)

	payload, err := buildNetworkProtocolsJSON(p, fnStore.updateEvery, expires)
	if err != nil {
		sendFunctionError(api, uid, 500, fmt.Sprintf("json marshal error: %v", err))
		return
	}

	api.FUNCRESULT(netdataapi.FunctionResult{
		UID:             uid,
		Code:            "200",
		ContentType:     "application/json",
		ExpireTimestamp: strconv.FormatInt(expires, 10),
		Payload:         payload,
	})
}

func sendFunctionError(api *netdataapi.API, uid string, code int, msg string) {
	payload := fmt.Sprintf(`{"status":%d,"message":"%s"}`, code, msg)
	api.FUNCRESULT(netdataapi.FunctionResult{
		UID:             uid,
		Code:            strconv.Itoa(code),
		ContentType:     "application/json",
		ExpireTimestamp: strconv.FormatInt(time.Now().Unix(), 10),
		Payload:         payload,
	})
}

// buildNetworkProtocolsJSON produces the JSON table matching the FreeBSD network-protocols schema.
func buildNetworkProtocolsJSON(p socketGlobalPublish, updateEvery int, expires int64) (string, error) {
	// Cross-map: "tcp_cleanup_rbuf" dim carries sendmsg (sent) data; "tcp_sendmsg" dim carries cleanup_rbuf (received) data.
	tcpReceived := p.tcpDimSendmsgCalls
	tcpSent := p.tcpDimCleanupCalls
	tcpErrors := p.tcpDimCleanupErr + p.tcpDimSendmsgErr
	tcpConnActive := p.tcpV4Conn + p.tcpV6Conn
	tcpConnPassive := p.inboundTCP
	tcpSegsTotal := p.tcpDimSendmsgCalls + p.tcpDimCleanupCalls + p.tcpCloseCalls
	tcpSegsRetrans := p.tcpRetransmit

	udpReceived := p.udpRecvCalls
	udpSent := p.udpSendCalls
	udpErrors := p.udpRecvErr + p.udpSendErr
	udpConnPassive := p.inboundUDP

	type valueOptions struct {
		Units         string  `json:"units,omitempty"`
		Transform     string  `json:"transform"`
		DecimalPoints int     `json:"decimal_points"`
		DefaultValue  *string `json:"default_value"`
	}
	type columnDef struct {
		Index                 int          `json:"index"`
		UniqueKey             bool         `json:"unique_key"`
		Name                  string       `json:"name"`
		Visible               bool         `json:"visible"`
		Type                  string       `json:"type"`
		Units                 string       `json:"units,omitempty"`
		Visualization         string       `json:"visualization"`
		ValueOptions          valueOptions `json:"value_options"`
		Sort                  string       `json:"sort"`
		Sortable              bool         `json:"sortable"`
		Sticky                bool         `json:"sticky"`
		Summary               string       `json:"summary"`
		Filter                string       `json:"filter"`
		FullWidth             bool         `json:"full_width"`
		Wrap                  bool         `json:"wrap"`
		DefaultExpandedFilter bool         `json:"default_expanded_filter"`
	}
	type chartDef struct {
		Name    string   `json:"name"`
		Type    string   `json:"type"`
		Columns []string `json:"columns"`
	}
	type groupByDef struct {
		Name    string   `json:"name"`
		Columns []string `json:"columns"`
	}
	type response struct {
		Status            int                    `json:"status"`
		Type              string                 `json:"type"`
		UpdateEvery       int                    `json:"update_every"`
		HasHistory        bool                   `json:"has_history"`
		Help              string                 `json:"help"`
		Data              [][]interface{}        `json:"data"`
		Columns           map[string]columnDef   `json:"columns"`
		DefaultSortColumn string                 `json:"default_sort_column"`
		Charts            map[string]chartDef    `json:"charts"`
		DefaultCharts     [][]string             `json:"default_charts"`
		GroupBy           map[string]groupByDef  `json:"group_by"`
		Expires           int64                  `json:"expires"`
	}

	strCol := func(idx int, name string, uniqueKey, sticky bool) columnDef {
		return columnDef{
			Index:         idx,
			UniqueKey:     uniqueKey,
			Name:          name,
			Visible:       true,
			Type:          "string",
			Visualization: "value",
			ValueOptions:  valueOptions{Transform: "none", DecimalPoints: 0},
			Sort:          "ascending",
			Sortable:      true,
			Sticky:        sticky,
			Summary:       "count",
			Filter:        "multiselect",
		}
	}
	intCol := func(idx int, name, units string) columnDef {
		return columnDef{
			Index:         idx,
			UniqueKey:     false,
			Name:          name,
			Visible:       true,
			Type:          "integer",
			Units:         units,
			Visualization: "value",
			ValueOptions:  valueOptions{Units: units, Transform: "number", DecimalPoints: 0},
			Sort:          "descending",
			Sortable:      true,
			Sticky:        false,
			Summary:       "sum",
			Filter:        "range",
		}
	}

	resp := response{
		Status:      200,
		Type:        "table",
		UpdateEvery: updateEvery,
		HasHistory:  false,
		Help:        socketFunctionHelp,
		Data: [][]interface{}{
			{"TCP", "IPv4+IPv6",
				tcpReceived, tcpSent, tcpErrors,
				tcpConnActive, uint64(0), tcpConnPassive, uint64(0),
				tcpSegsTotal, tcpSegsRetrans, uint64(0)},
			{"UDP", "IPv4+IPv6",
				udpReceived, udpSent, udpErrors,
				uint64(0), uint64(0), udpConnPassive, uint64(0),
				uint64(0), uint64(0), uint64(0)},
		},
		Columns: map[string]columnDef{
			"Transport":         strCol(0, "Transport Protocol", true, true),
			"Family":            strCol(1, "IP Protocol Family", true, true),
			"Received":          intCol(2, "Received (Segments/Datagrams)", "segments/datagrams/s"),
			"Sent":              intCol(3, "Sent (Segments/Datagrams)", "segments/datagrams/s"),
			"Errors":            intCol(4, "Errors (Failures/Rx Errors)", "errors"),
			"ConnActive":        intCol(5, "Active Connections Opened", "opens"),
			"ConnEstablished":   intCol(6, "Currently Established Connections", "connections"),
			"ConnPassive":       intCol(7, "Passive Connections Opened", "opens"),
			"ConnReset":         intCol(8, "Reset Connections", "resets"),
			"SegsTotal":         intCol(9, "Total Segments", "segments/s"),
			"SegsRetransmitted": intCol(10, "Retransmitted Segments", "segments/s"),
			"DatagramsNoPort":   intCol(11, "Datagrams with No Port", "datagrams/s"),
		},
		DefaultSortColumn: "Received",
		Charts: map[string]chartDef{
			"Traffic": {
				Name:    "Traffic",
				Type:    "stacked-bar",
				Columns: []string{"Received", "Sent"},
			},
		},
		DefaultCharts: [][]string{{"Traffic", "Transport"}},
		GroupBy: map[string]groupByDef{
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
