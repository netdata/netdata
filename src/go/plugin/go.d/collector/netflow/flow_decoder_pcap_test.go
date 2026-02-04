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

func TestDecodePCAP_NetFlowFixtures(t *testing.T) {
	type pcapFixture struct {
		name       string
		version    string
		templates  []string
		allowEmpty bool
	}

	fixtures := []pcapFixture{
		{name: "nfv5.pcap", version: "v5"},

		{name: "data.pcap", version: "v9", templates: []string{"template.pcap"}},
		{name: "data+templates.pcap", version: "v9", allowEmpty: true},
		{name: "datalink-data.pcap", version: "v9", templates: []string{"datalink-template.pcap"}, allowEmpty: true},
		{name: "icmp-data.pcap", version: "v9", templates: []string{"icmp-template.pcap"}},
		{name: "juniper-cpid-data.pcap", version: "v9", templates: []string{"juniper-cpid-template.pcap"}, allowEmpty: true},
		{name: "mpls.pcap", version: "v9", allowEmpty: true},
		{name: "nat.pcap", version: "v9", allowEmpty: true},
		{name: "options-data.pcap", version: "v9", templates: []string{"options-template.pcap"}, allowEmpty: true},
		{name: "physicalinterfaces.pcap", version: "v9", allowEmpty: true},
		{name: "samplingrate-data.pcap", version: "v9", templates: []string{"samplingrate-template.pcap"}},
		{name: "multiplesamplingrates-data.pcap", version: "v9", templates: []string{"multiplesamplingrates-template.pcap"}},
		{name: "multiplesamplingrates-options-data.pcap", version: "v9", templates: []string{"multiplesamplingrates-options-template.pcap"}, allowEmpty: true},

		{name: "template.pcap", version: "v9", allowEmpty: true},
		{name: "datalink-template.pcap", version: "v9", allowEmpty: true},
		{name: "icmp-template.pcap", version: "v9", allowEmpty: true},
		{name: "juniper-cpid-template.pcap", version: "v9", allowEmpty: true},
		{name: "options-template.pcap", version: "v9", allowEmpty: true},
		{name: "samplingrate-template.pcap", version: "v9", allowEmpty: true},
		{name: "multiplesamplingrates-template.pcap", version: "v9", allowEmpty: true},
		{name: "multiplesamplingrates-options-template.pcap", version: "v9", allowEmpty: true},

		{name: "ipfixprobe-data.pcap", version: "ipfix", templates: []string{"ipfixprobe-templates.pcap"}},
		{name: "ipfix-srv6-data.pcap", version: "ipfix", templates: []string{"ipfix-srv6-template.pcap"}, allowEmpty: true},
		{name: "ipfixprobe-templates.pcap", version: "ipfix", allowEmpty: true},
		{name: "ipfix-srv6-template.pcap", version: "ipfix", allowEmpty: true},
	}

	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			cfg := flowDecoderConfig{}
			switch fixture.version {
			case "v5":
				cfg.enableV5 = true
			case "v9":
				cfg.enableV9 = true
			case "ipfix":
				cfg.enableIPFIX = true
			}

			decoder := newFlowDecoder(cfg)
			for _, tmpl := range fixture.templates {
				_ = decodePCAPRecords(t, "testdata/flows/"+tmpl, decoder, true)
			}

			records := decodePCAPRecords(t, "testdata/flows/"+fixture.name, decoder, true)
			if fixture.allowEmpty {
				return
			}
			requireDecodedRecords(t, records, fixture.version)
		})
	}
}

func TestDecodePCAP_SFlowFixtures(t *testing.T) {
	fixtures := []string{
		"data-sflow-ipv4-data.pcap",
		"data-sflow-raw-ipv4.pcap",
		"data-sflow-expanded-sample.pcap",
		"data-encap-vxlan.pcap",
		"data-qinq.pcap",
		"data-icmpv4.pcap",
		"data-icmpv6.pcap",
		"data-multiple-interfaces.pcap",
		"data-discard-interface.pcap",
		"data-local-interface.pcap",
		"data-1140.pcap",
	}

	for _, name := range fixtures {
		name := name
		t.Run(name, func(t *testing.T) {
			decoder := newFlowDecoder(flowDecoderConfig{enableSFlow: true})
			records := decodePCAPRecords(t, "testdata/flows/"+name, decoder, false)
			requireDecodedRecords(t, records, "sflow")
		})
	}
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

func requireDecodedRecords(t *testing.T, records []flowRecord, version string) {
	t.Helper()
	require.NotEmpty(t, records)
	requireFlowVersion(t, records, version)

	for _, record := range records {
		if record.Bytes > 0 || record.Packets > 0 || record.Flows > 0 {
			return
		}
		if record.Key.SrcPrefix != "" || record.Key.DstPrefix != "" {
			return
		}
	}
	require.Fail(t, "no decoded record fields", "expected non-empty bytes/packets/flows or IP prefixes")
}
