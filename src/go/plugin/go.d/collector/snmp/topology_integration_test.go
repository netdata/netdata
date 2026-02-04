// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package snmp

import (
	"context"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTopologyIntegrationWithSnmpsim(t *testing.T) {
	endpoint := strings.TrimSpace(os.Getenv("NETDATA_SNMPSIM_ENDPOINT"))
	communitiesRaw := strings.TrimSpace(os.Getenv("NETDATA_SNMPSIM_COMMUNITIES"))
	if endpoint == "" || communitiesRaw == "" {
		t.Skip("missing NETDATA_SNMPSIM_ENDPOINT or NETDATA_SNMPSIM_COMMUNITIES")
	}

	host, port := parseSnmpEndpoint(t, endpoint)
	communities := splitCSV(communitiesRaw)

	var totalLinks int
	for _, community := range communities {
		cfg := prepareV2Config()
		cfg.Hostname = host
		cfg.Community = community
		cfg.Options.Port = port
		cfg.Topology.Autoprobe = true
		cfg.Ping.Enabled = false
		cfg.CreateVnode = false

		coll := New()
		coll.Config = cfg
		require.NoError(t, coll.Init(context.Background()))
		require.NoError(t, coll.Check(context.Background()))
		_ = coll.Collect(context.Background())

		coll.topologyCache.mu.RLock()
		snapshot, ok := coll.topologyCache.snapshot()
		coll.topologyCache.mu.RUnlock()
		require.True(t, ok)
		if len(snapshot.Links) > 0 {
			totalLinks += len(snapshot.Links)
		}

		coll.Cleanup(context.Background())
	}

	require.Greater(t, totalLinks, 0)
}

func parseSnmpEndpoint(t *testing.T, endpoint string) (string, int) {
	if host, portStr, err := net.SplitHostPort(endpoint); err == nil {
		port, err := parsePort(portStr)
		require.NoError(t, err)
		return host, port
	}
	return endpoint, 161
}

func parsePort(value string) (int, error) {
	port, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	return port, nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		p := strings.TrimSpace(part)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
