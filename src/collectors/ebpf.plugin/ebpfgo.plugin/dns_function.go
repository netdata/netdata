package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

const (
	dnsFunctionName        = "dns-queries"
	dnsFunctionHelp        = "Linux eBPF DNS query and response statistics (UDP/TCP, IPv4/IPv6)"
	dnsFunctionTimeout     = 10
	dnsFunctionTags        = "top"
	// HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE = (1<<0)|(1<<1) = 3
	dnsFunctionAccess      = "0x00000003"
	dnsFunctionPriority    = 100
	dnsFunctionVersion     = 1
	dnsFunctionUpdateEvery = 5
)

// dnsFunctionStore holds the latest per-cycle DNS snapshot for on-demand
// dns-queries function calls.  Updated by the collector goroutine; read by
// the stdin dispatcher goroutine.
type dnsFunctionStore struct {
	mu          sync.RWMutex
	snap        libbpfloader.DNSSnapshot
	hasData     bool
	updateEvery int
}

func newDNSFunctionStore(updateEvery int) *dnsFunctionStore {
	if updateEvery <= 0 {
		updateEvery = dnsFunctionUpdateEvery
	}
	return &dnsFunctionStore{updateEvery: updateEvery}
}

func (s *dnsFunctionStore) update(snap libbpfloader.DNSSnapshot) {
	s.mu.Lock()
	s.snap = snap
	s.hasData = true
	s.mu.Unlock()
}

func (s *dnsFunctionStore) snapshot() (libbpfloader.DNSSnapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snap, s.hasData
}

// handleDNSQueries builds and writes the dns-queries function response.
func handleDNSQueries(api *netdataapi.API, fnStore *dnsFunctionStore, uid string) {
	snap, hasData := fnStore.snapshot()
	if !hasData {
		sendFunctionError(api, uid, 503, "dns-queries: data not yet available")
		return
	}

	now := time.Now().Unix()
	expires := now + int64(fnStore.updateEvery)

	payload, err := buildDNSQueriesJSON(snap, fnStore.updateEvery, expires)
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

// buildDNSQueriesJSON produces the JSON table for the dns-queries network-viewer function.
// Rows: one per (transport × IP version) combination = 4 rows.
func buildDNSQueriesJSON(snap libbpfloader.DNSSnapshot, updateEvery int, expires int64) (string, error) {
	resp := fnTableResponse{
		Status:      200,
		Type:        "table",
		UpdateEvery: updateEvery,
		HasHistory:  false,
		Help:        dnsFunctionHelp,
		Data: [][]interface{}{
			{"UDP", "IPv4", snap.QueriesUDPv4, snap.ResponsesUDPv4},
			{"UDP", "IPv6", snap.QueriesUDPv6, snap.ResponsesUDPv6},
			{"TCP", "IPv4", snap.QueriesTCPv4, snap.ResponsesTCPv4},
			{"TCP", "IPv6", snap.QueriesTCPv6, snap.ResponsesTCPv6},
		},
		Columns: map[string]fnColumnDef{
			"Transport": fnStrCol(0, "Transport Protocol", true, true),
			"IPFamily":  fnStrCol(1, "IP Protocol Family", true, true),
			"Queries":   fnIntCol(2, "DNS Queries", "queries/s"),
			"Responses": fnIntCol(3, "DNS Responses", "responses/s"),
		},
		DefaultSortColumn: "Queries",
		Charts: map[string]fnChartDef{
			"Traffic": {
				Name:    "Traffic",
				Type:    "stacked-bar",
				Columns: []string{"Queries", "Responses"},
			},
		},
		DefaultCharts: [][]string{{"Traffic", "Transport"}},
		GroupBy: map[string]fnGroupByDef{
			"Transport": {
				Name:    "Transport",
				Columns: []string{"Transport"},
			},
			"IPFamily": {
				Name:    "IP Family",
				Columns: []string{"IPFamily"},
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
