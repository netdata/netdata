// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package netflow

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
)

func TestNetFlowIntegrationReplayPCAP(t *testing.T) {
	collector := New()
	collector.Address = "127.0.0.1:0"
	collector.Aggregation.MaxBuckets = 5
	collector.Aggregation.MaxKeys = 20000

	require.NoError(t, collector.Init(context.Background()))
	defer collector.Cleanup(context.Background())

	udpAddr, ok := collector.conn.LocalAddr().(*net.UDPAddr)
	require.True(t, ok)

	conn, err := net.DialUDP("udp", nil, udpAddr)
	require.NoError(t, err)
	defer conn.Close()

	replayPCAPToUDP(t, conn, "testdata/flows/template.pcap")
	replayPCAPToUDP(t, conn, "testdata/flows/data.pcap")
	replayPCAPToUDP(t, conn, "testdata/flows/nfv5.pcap")
	replayPCAPToUDP(t, conn, "testdata/flows/ipfixprobe-templates.pcap")
	replayPCAPToUDP(t, conn, "testdata/flows/ipfixprobe-data.pcap")
	replayPCAPToUDP(t, conn, "testdata/flows/data-sflow-ipv4-data.pcap")

	stressDir := os.Getenv("NETDATA_NETFLOW_STRESS_PCAPS")
	if stressDir != "" {
		small := filepath.Join(stressDir, "smallFlows.pcap")
		big := filepath.Join(stressDir, "bigFlows.pcap")
		if fileExists(small) {
			replayPCAPToUDP(t, conn, small)
		}
		if fileExists(big) {
			replayPCAPToUDP(t, conn, big)
		}
	}

	require.Eventually(t, func() bool {
		snapshot := collector.aggregator.Snapshot("agent")
		return len(snapshot.Buckets) > 0
	}, 3*time.Second, 100*time.Millisecond)
}

func replayPCAPToUDP(t *testing.T, conn *net.UDPConn, path string) {
	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()

	reader, err := pcapgo.NewReader(file)
	require.NoError(t, err)

	source := gopacket.NewPacketSource(reader, reader.LinkType())
	for packet := range source.Packets() {
		if packet == nil {
			continue
		}
		udpLayer := packet.Layer(layers.LayerTypeUDP)
		if udpLayer == nil {
			continue
		}
		udp := udpLayer.(*layers.UDP)
		if len(udp.Payload) == 0 {
			continue
		}
		_, err := conn.Write(udp.Payload)
		require.NoError(t, err)
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
