// SPDX-License-Identifier: GPL-3.0-or-later

package netflow

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/netsampler/goflow2/v2/decoders/netflow"
	"github.com/netsampler/goflow2/v2/decoders/sflow"
	"github.com/netsampler/goflow2/v2/decoders/utils"
	"github.com/stretchr/testify/require"
)

func TestDecodeV5(t *testing.T) {
	payload := buildNetFlowV5Packet()
	decoder := newFlowDecoder(flowDecoderConfig{enableV5: true})

	records, err := decoder.Decode(payload, net.ParseIP("192.0.2.1"))
	require.NoError(t, err)
	require.Len(t, records, 1)

	rec := records[0]
	require.Equal(t, uint64(1000), rec.Bytes)
	require.Equal(t, uint64(100), rec.Packets)
	require.Equal(t, 12345, rec.Key.SrcPort)
	require.Equal(t, 80, rec.Key.DstPort)
	require.Equal(t, 6, rec.Key.Protocol)
	require.Equal(t, 65001, rec.Key.SrcAS)
	require.Equal(t, 65002, rec.Key.DstAS)
	require.Equal(t, 100, rec.SamplingRate)
}

func TestDecodeDataRecordNFv9(t *testing.T) {
	boot := time.Date(2026, 2, 4, 0, 0, 0, 0, time.UTC)
	base := boot.Add(10 * time.Second)

	fields := []netflow.DataField{
		{Type: netflow.NFV9_FIELD_IN_BYTES, Value: []byte{0x00, 0x00, 0x03, 0xE8}},
		{Type: netflow.NFV9_FIELD_IN_PKTS, Value: []byte{0x00, 0x00, 0x00, 0x0A}},
		{Type: netflow.NFV9_FIELD_PROTOCOL, Value: []byte{0x06}},
		{Type: netflow.NFV9_FIELD_L4_SRC_PORT, Value: []byte{0x30, 0x39}},
		{Type: netflow.NFV9_FIELD_L4_DST_PORT, Value: []byte{0x00, 0x50}},
		{Type: netflow.NFV9_FIELD_IPV4_SRC_ADDR, Value: []byte{0x0A, 0x00, 0x00, 0x01}},
		{Type: netflow.NFV9_FIELD_IPV4_DST_ADDR, Value: []byte{0x0A, 0x00, 0x00, 0x02}},
		{Type: netflow.NFV9_FIELD_SRC_MASK, Value: []byte{0x18}},
		{Type: netflow.NFV9_FIELD_DST_MASK, Value: []byte{0x18}},
		{Type: netflow.NFV9_FIELD_FIRST_SWITCHED, Value: []byte{0x00, 0x00, 0x03, 0xE8}},
		{Type: netflow.NFV9_FIELD_LAST_SWITCHED, Value: []byte{0x00, 0x00, 0x07, 0xD0}},
		{Type: netflow.NFV9_FIELD_SAMPLING_INTERVAL, Value: []byte{0x00, 0x64}},
	}

	rec, ok := decodeDataRecord(9, fields, "203.0.113.10", base, boot)
	require.True(t, ok)
	require.Equal(t, uint64(1000), rec.Bytes)
	require.Equal(t, uint64(10), rec.Packets)
	require.Equal(t, 6, rec.Key.Protocol)
	require.Equal(t, 12345, rec.Key.SrcPort)
	require.Equal(t, 80, rec.Key.DstPort)
	require.Equal(t, "10.0.0.1/24", rec.Key.SrcPrefix)
	require.Equal(t, "10.0.0.2/24", rec.Key.DstPrefix)
	require.Equal(t, 100, rec.SamplingRate)
}

func TestDecodeDataRecordIPFIX(t *testing.T) {
	base := time.Date(2026, 2, 4, 0, 0, 0, 0, time.UTC)
	fields := []netflow.DataField{
		{Type: netflow.IPFIX_FIELD_octetDeltaCount, Value: []byte{0x00, 0x00, 0x08, 0x00}},
		{Type: netflow.IPFIX_FIELD_packetDeltaCount, Value: []byte{0x00, 0x00, 0x00, 0x20}},
		{Type: netflow.IPFIX_FIELD_protocolIdentifier, Value: []byte{0x11}},
		{Type: netflow.IPFIX_FIELD_sourceTransportPort, Value: []byte{0x1F, 0x90}},
		{Type: netflow.IPFIX_FIELD_destinationTransportPort, Value: []byte{0x00, 0x35}},
		{Type: netflow.IPFIX_FIELD_sourceIPv4Address, Value: []byte{0xC0, 0x00, 0x02, 0x01}},
		{Type: netflow.IPFIX_FIELD_destinationIPv4Address, Value: []byte{0xC0, 0x00, 0x02, 0x02}},
		{Type: netflow.IPFIX_FIELD_sourceIPv4PrefixLength, Value: []byte{0x18}},
		{Type: netflow.IPFIX_FIELD_destinationIPv4PrefixLength, Value: []byte{0x18}},
		{Type: netflow.IPFIX_FIELD_ingressInterface, Value: []byte{0x00, 0x00, 0x00, 0x05}},
		{Type: netflow.IPFIX_FIELD_egressInterface, Value: []byte{0x00, 0x00, 0x00, 0x07}},
		{Type: netflow.IPFIX_FIELD_bgpSourceAsNumber, Value: []byte{0x00, 0x00, 0xFD, 0xE8}},
		{Type: netflow.IPFIX_FIELD_bgpDestinationAsNumber, Value: []byte{0x00, 0x00, 0xFD, 0xE9}},
		{Type: netflow.IPFIX_FIELD_samplingInterval, Value: []byte{0x00, 0x64}},
		{Type: netflow.IPFIX_FIELD_flowStartSeconds, Value: []byte{0x65, 0x8F, 0x0C, 0x00}},
		{Type: netflow.IPFIX_FIELD_flowEndSeconds, Value: []byte{0x65, 0x8F, 0x0C, 0x0A}},
		{Type: netflow.IPFIX_FIELD_flowDirection, Value: []byte{0x00}},
	}

	rec, ok := decodeDataRecord(10, fields, "203.0.113.10", base, base)
	require.True(t, ok)
	require.Equal(t, uint64(2048), rec.Bytes)
	require.Equal(t, uint64(32), rec.Packets)
	require.Equal(t, 17, rec.Key.Protocol)
	require.Equal(t, 8080, rec.Key.SrcPort)
	require.Equal(t, 53, rec.Key.DstPort)
	require.Equal(t, "192.0.2.1/24", rec.Key.SrcPrefix)
	require.Equal(t, "192.0.2.2/24", rec.Key.DstPrefix)
	require.Equal(t, 5, rec.Key.InIf)
	require.Equal(t, 7, rec.Key.OutIf)
	require.Equal(t, 65000, rec.Key.SrcAS)
	require.Equal(t, 65001, rec.Key.DstAS)
	require.Equal(t, 100, rec.SamplingRate)
	require.Equal(t, "ingress", rec.Direction)
	require.False(t, rec.Timestamp.IsZero())
}

func TestDecodeSFlowSample(t *testing.T) {
	now := time.Date(2026, 2, 4, 0, 0, 0, 0, time.UTC)
	packet := &sflow.Packet{
		Version: 5,
		AgentIP: utils.IPAddress(net.ParseIP("198.51.100.10").To4()),
		Samples: []interface{}{
			sflow.FlowSample{
				SamplingRate: 100,
				Input:        10,
				Output:       20,
				Records: []sflow.FlowRecord{
					{Data: sflow.SampledIPv4{
						SampledIPBase: sflow.SampledIPBase{
							Length:   128,
							Protocol: 6,
							SrcIP:    utils.IPAddress(net.ParseIP("10.0.0.1").To4()),
							DstIP:    utils.IPAddress(net.ParseIP("10.0.0.2").To4()),
							SrcPort:  1234,
							DstPort:  80,
						},
					}},
					{Data: sflow.ExtendedRouter{SrcMaskLen: 24, DstMaskLen: 24}},
					{Data: sflow.ExtendedGateway{SrcAS: 64512, ASPath: []uint32{64512, 64513}}},
				},
			},
		},
	}

	records := decodeSFlowPacket(packet, "", now)
	require.Len(t, records, 1)

	rec := records[0]
	require.Equal(t, "198.51.100.10", rec.ExporterIP)
	require.Equal(t, "10.0.0.1/24", rec.Key.SrcPrefix)
	require.Equal(t, "10.0.0.2/24", rec.Key.DstPrefix)
	require.Equal(t, 1234, rec.Key.SrcPort)
	require.Equal(t, 80, rec.Key.DstPort)
	require.Equal(t, 6, rec.Key.Protocol)
	require.Equal(t, 64512, rec.Key.SrcAS)
	require.Equal(t, 64513, rec.Key.DstAS)
	require.Equal(t, 10, rec.Key.InIf)
	require.Equal(t, 20, rec.Key.OutIf)
	require.Equal(t, uint64(128), rec.Bytes)
	require.Equal(t, uint64(1), rec.Packets)
	require.Equal(t, 100, rec.SamplingRate)
	require.Equal(t, "sflow", rec.FlowVersion)
	require.True(t, rec.Timestamp.Equal(now))
}

func buildNetFlowV5Packet() []byte {
	buf := &bytes.Buffer{}
	binary.Write(buf, binary.BigEndian, uint16(5))
	binary.Write(buf, binary.BigEndian, uint16(1))
	binary.Write(buf, binary.BigEndian, uint32(100000))
	binary.Write(buf, binary.BigEndian, uint32(1700000000))
	binary.Write(buf, binary.BigEndian, uint32(0))
	binary.Write(buf, binary.BigEndian, uint32(1))
	buf.WriteByte(0)
	buf.WriteByte(0)
	binary.Write(buf, binary.BigEndian, uint16(100))

	binary.Write(buf, binary.BigEndian, uint32(0x0A000001))
	binary.Write(buf, binary.BigEndian, uint32(0x0A000002))
	binary.Write(buf, binary.BigEndian, uint32(0))
	binary.Write(buf, binary.BigEndian, uint16(1))
	binary.Write(buf, binary.BigEndian, uint16(2))
	binary.Write(buf, binary.BigEndian, uint32(100))
	binary.Write(buf, binary.BigEndian, uint32(1000))
	binary.Write(buf, binary.BigEndian, uint32(90000))
	binary.Write(buf, binary.BigEndian, uint32(95000))
	binary.Write(buf, binary.BigEndian, uint16(12345))
	binary.Write(buf, binary.BigEndian, uint16(80))
	buf.WriteByte(0)
	buf.WriteByte(0)
	buf.WriteByte(6)
	buf.WriteByte(0)
	binary.Write(buf, binary.BigEndian, uint16(65001))
	binary.Write(buf, binary.BigEndian, uint16(65002))
	buf.WriteByte(24)
	buf.WriteByte(24)
	binary.Write(buf, binary.BigEndian, uint16(0))

	return buf.Bytes()
}
