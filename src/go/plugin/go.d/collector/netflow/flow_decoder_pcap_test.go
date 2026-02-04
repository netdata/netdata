// SPDX-License-Identifier: GPL-3.0-or-later

package netflow

import (
	"net"
	"os"
	"strings"
	"testing"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
)

func TestDecodePCAP_NetFlowV5(t *testing.T) {
	decoder := newFlowDecoder(flowDecoderConfig{enableV5: true})
	records := decodePCAPRecords(t, "testdata/flows/nfv5.pcap", decoder, false)
	require.NotEmpty(t, records)
	requireFlowVersion(t, records, "v5")
}

func TestDecodePCAP_NetFlowV9(t *testing.T) {
	decoder := newFlowDecoder(flowDecoderConfig{enableV9: true})
	_ = decodePCAPRecords(t, "testdata/flows/template.pcap", decoder, true)
	records := decodePCAPRecords(t, "testdata/flows/data.pcap", decoder, true)
	require.NotEmpty(t, records)
	requireFlowVersion(t, records, "v9")
}

func TestDecodePCAP_IPFIX(t *testing.T) {
	decoder := newFlowDecoder(flowDecoderConfig{enableIPFIX: true})
	_ = decodePCAPRecords(t, "testdata/flows/ipfixprobe-templates.pcap", decoder, true)
	records := decodePCAPRecords(t, "testdata/flows/ipfixprobe-data.pcap", decoder, true)
	require.NotEmpty(t, records)
	requireFlowVersion(t, records, "ipfix")
}

func TestDecodePCAP_SFlow(t *testing.T) {
	decoder := newFlowDecoder(flowDecoderConfig{enableSFlow: true})
	records := decodePCAPRecords(t, "testdata/flows/data-sflow-ipv4-data.pcap", decoder, false)
	require.NotEmpty(t, records)
	requireFlowVersion(t, records, "sflow")
}

func decodePCAPRecords(t *testing.T, path string, decoder *flowDecoder, allowTemplateErrors bool) []flowRecord {
	t.Helper()

	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()

	reader, err := pcapgo.NewReader(file)
	require.NoError(t, err)

	source := gopacket.NewPacketSource(reader, reader.LinkType())
	var records []flowRecord
	udpPackets := 0

	for packet := range source.Packets() {
		if packet == nil {
			continue
		}
		udpLayer := packet.Layer(layers.LayerTypeUDP)
		if udpLayer == nil {
			continue
		}
		udpPackets++
		udp := udpLayer.(*layers.UDP)
		if len(udp.Payload) == 0 {
			continue
		}

		var srcIP net.IP
		if ip4 := packet.Layer(layers.LayerTypeIPv4); ip4 != nil {
			srcIP = ip4.(*layers.IPv4).SrcIP
		} else if ip6 := packet.Layer(layers.LayerTypeIPv6); ip6 != nil {
			srcIP = ip6.(*layers.IPv6).SrcIP
		}

		recs, err := decoder.Decode(udp.Payload, srcIP)
		if err != nil {
			if allowTemplateErrors && strings.Contains(err.Error(), "template not found") {
				continue
			}
			require.NoError(t, err)
		}
		records = append(records, recs...)
	}

	require.Greater(t, udpPackets, 0)
	return records
}

func requireFlowVersion(t *testing.T, records []flowRecord, version string) {
	t.Helper()
	for _, record := range records {
		if record.FlowVersion == version {
			return
		}
	}
	require.Failf(t, "missing flow version", "expected version %s in decoded records", version)
}
