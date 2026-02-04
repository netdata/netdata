// SPDX-License-Identifier: GPL-3.0-or-later

package netflow

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/netsampler/goflow2/v2/decoders/netflow"
	"github.com/netsampler/goflow2/v2/decoders/netflowlegacy"
	"github.com/netsampler/goflow2/v2/decoders/sflow"
)

type flowDecoderConfig struct {
	enableV5    bool
	enableV9    bool
	enableIPFIX bool
	enableSFlow bool
}

type flowDecoder struct {
	cfg       flowDecoderConfig
	templates map[string]netflow.NetFlowTemplateSystem
}

func newFlowDecoder(cfg flowDecoderConfig) *flowDecoder {
	return &flowDecoder{
		cfg:       cfg,
		templates: make(map[string]netflow.NetFlowTemplateSystem),
	}
}

func (d *flowDecoder) Decode(payload []byte, exporterIP net.IP) ([]flowRecord, error) {
	if len(payload) < 2 {
		return nil, errors.New("payload too short")
	}
	if isSFlowPayload(payload) {
		if !d.cfg.enableSFlow {
			return nil, nil
		}
		return d.decodeSFlow(payload, exporterIP)
	}
	version := binary.BigEndian.Uint16(payload[:2])
	exporter := ""
	if exporterIP != nil {
		exporter = exporterIP.String()
	}

	switch version {
	case 5:
		if !d.cfg.enableV5 {
			return nil, nil
		}
		return d.decodeV5(payload, exporter)
	case 9:
		if !d.cfg.enableV9 {
			return nil, nil
		}
		return d.decodeV9(payload, exporter)
	case 10:
		if !d.cfg.enableIPFIX {
			return nil, nil
		}
		return d.decodeIPFIX(payload, exporter)
	default:
		return nil, fmt.Errorf("unsupported netflow version %d", version)
	}
}

func isSFlowPayload(payload []byte) bool {
	if len(payload) < 4 {
		return false
	}
	version := binary.BigEndian.Uint32(payload[:4])
	return version == 5
}

func (d *flowDecoder) decodeV5(payload []byte, exporter string) ([]flowRecord, error) {
	buf := bytes.NewBuffer(payload)
	var packet netflowlegacy.PacketNetFlowV5
	if err := netflowlegacy.DecodeMessageVersion(buf, &packet); err != nil {
		return nil, err
	}

	bootTime := time.Unix(int64(packet.UnixSecs), int64(packet.UnixNSecs)).Add(-time.Duration(packet.SysUptime) * time.Millisecond)
	samplingRate := decodeSamplingInterval(packet.SamplingInterval)

	records := make([]flowRecord, 0, len(packet.Records))
	for _, rec := range packet.Records {
		srcIP := ipv4FromLegacy(rec.SrcAddr)
		dstIP := ipv4FromLegacy(rec.DstAddr)
		start := bootTime.Add(time.Duration(rec.First) * time.Millisecond)
		end := bootTime.Add(time.Duration(rec.Last) * time.Millisecond)

		record := flowRecord{
			Timestamp:   end,
			DurationSec: durationSeconds(start, end),
			Key: flowKey{
				SrcPrefix: ipWithPrefix(srcIP, int(rec.SrcMask)),
				DstPrefix: ipWithPrefix(dstIP, int(rec.DstMask)),
				SrcPort:   int(rec.SrcPort),
				DstPort:   int(rec.DstPort),
				Protocol:  int(rec.Proto),
				SrcAS:     int(rec.SrcAS),
				DstAS:     int(rec.DstAS),
				InIf:      int(rec.Input),
				OutIf:     int(rec.Output),
			},
			Bytes:        uint64(rec.DOctets),
			Packets:      uint64(rec.DPkts),
			Flows:        1,
			SamplingRate: samplingRate,
			ExporterIP:   exporter,
			FlowVersion:  "v5",
		}
		records = append(records, record)
	}

	return records, nil
}

func (d *flowDecoder) decodeV9(payload []byte, exporter string) ([]flowRecord, error) {
	buf := bytes.NewBuffer(payload)
	var packet netflow.NFv9Packet
	if err := netflow.DecodeMessageVersion(buf, d.templateSystem(exporter), &packet, nil); err != nil {
		return nil, err
	}
	baseTime := time.Unix(int64(packet.UnixSeconds), 0)
	bootTime := baseTime.Add(-time.Duration(packet.SystemUptime) * time.Millisecond)

	return d.decodeFlowSets(9, packet.FlowSets, exporter, baseTime, bootTime), nil
}

func (d *flowDecoder) decodeIPFIX(payload []byte, exporter string) ([]flowRecord, error) {
	buf := bytes.NewBuffer(payload)
	var packet netflow.IPFIXPacket
	if err := netflow.DecodeMessageVersion(buf, d.templateSystem(exporter), nil, &packet); err != nil {
		return nil, err
	}
	baseTime := time.Unix(int64(packet.ExportTime), 0)
	bootTime := baseTime

	return d.decodeFlowSets(10, packet.FlowSets, exporter, baseTime, bootTime), nil
}

func (d *flowDecoder) decodeSFlow(payload []byte, exporterIP net.IP) ([]flowRecord, error) {
	buf := bytes.NewBuffer(payload)
	var packet sflow.Packet
	if err := sflow.DecodeMessageVersion(buf, &packet); err != nil {
		return nil, err
	}

	exporter := ""
	if exporterIP != nil {
		exporter = exporterIP.String()
	}
	if len(packet.AgentIP) > 0 {
		exporter = net.IP(packet.AgentIP).String()
	}

	return decodeSFlowPacket(&packet, exporter, time.Now()), nil
}

func (d *flowDecoder) decodeFlowSets(version uint16, flowSets []interface{}, exporter string, baseTime, bootTime time.Time) []flowRecord {
	var records []flowRecord
	for _, fs := range flowSets {
		dataFlow, ok := fs.(netflow.DataFlowSet)
		if !ok {
			continue
		}
		for _, record := range dataFlow.Records {
			flowRecord, ok := decodeDataRecord(version, record.Values, exporter, baseTime, bootTime)
			if !ok {
				continue
			}
			records = append(records, flowRecord)
		}
	}
	return records
}

func (d *flowDecoder) templateSystem(exporter string) netflow.NetFlowTemplateSystem {
	system := d.templates[exporter]
	if system != nil {
		return system
	}
	system = netflow.CreateTemplateSystem()
	d.templates[exporter] = system
	return system
}

func decodeDataRecord(version uint16, fields []netflow.DataField, exporter string, baseTime, bootTime time.Time) (flowRecord, bool) {
	var (
		bytesVal   uint64
		packetsVal uint64
		flowsVal   uint64
		proto      int
		srcPort    int
		dstPort    int
		srcAS      int
		dstAS      int
		inIf       int
		outIf      int
		srcIP      net.IP
		dstIP      net.IP
		srcMask    int
		dstMask    int
		sampling   int
		direction  string
		startTime  time.Time
		endTime    time.Time
	)

	for _, field := range fields {
		valueBytes, ok := field.Value.([]byte)
		if !ok {
			continue
		}
		switch field.Type {
		case netflow.NFV9_FIELD_IN_BYTES:
			bytesVal = decodeUint(valueBytes)
		case netflow.IPFIX_FIELD_postOctetDeltaCount:
			if bytesVal == 0 {
				bytesVal = decodeUint(valueBytes)
			}
		case netflow.NFV9_FIELD_IN_PKTS:
			packetsVal = decodeUint(valueBytes)
		case netflow.IPFIX_FIELD_postPacketDeltaCount:
			if packetsVal == 0 {
				packetsVal = decodeUint(valueBytes)
			}
		case netflow.NFV9_FIELD_FLOWS:
			flowsVal = decodeUint(valueBytes)
		case netflow.NFV9_FIELD_PROTOCOL:
			proto = int(decodeUint(valueBytes))
		case netflow.NFV9_FIELD_L4_SRC_PORT, netflow.IPFIX_FIELD_udpSourcePort, netflow.IPFIX_FIELD_tcpSourcePort:
			srcPort = int(decodeUint(valueBytes))
		case netflow.NFV9_FIELD_L4_DST_PORT, netflow.IPFIX_FIELD_udpDestinationPort, netflow.IPFIX_FIELD_tcpDestinationPort:
			dstPort = int(decodeUint(valueBytes))
		case netflow.NFV9_FIELD_SRC_AS:
			srcAS = int(decodeUint(valueBytes))
		case netflow.NFV9_FIELD_DST_AS:
			dstAS = int(decodeUint(valueBytes))
		case netflow.NFV9_FIELD_INPUT_SNMP:
			inIf = int(decodeUint(valueBytes))
		case netflow.NFV9_FIELD_OUTPUT_SNMP:
			outIf = int(decodeUint(valueBytes))
		case netflow.NFV9_FIELD_IPV4_SRC_ADDR:
			srcIP = decodeIP(valueBytes)
		case netflow.NFV9_FIELD_IPV4_DST_ADDR:
			dstIP = decodeIP(valueBytes)
		case netflow.NFV9_FIELD_IPV6_SRC_ADDR:
			srcIP = decodeIP(valueBytes)
		case netflow.NFV9_FIELD_IPV6_DST_ADDR:
			dstIP = decodeIP(valueBytes)
		case netflow.NFV9_FIELD_SRC_MASK, netflow.NFV9_FIELD_IPV6_SRC_MASK:
			srcMask = int(decodeUint(valueBytes))
		case netflow.NFV9_FIELD_DST_MASK, netflow.NFV9_FIELD_IPV6_DST_MASK:
			dstMask = int(decodeUint(valueBytes))
		case netflow.NFV9_FIELD_SAMPLING_INTERVAL:
			sampling = int(decodeUint(valueBytes))
		case netflow.NFV9_FIELD_DIRECTION, netflow.IPFIX_FIELD_biflowDirection:
			direction = decodeDirection(decodeUint(valueBytes))
		case netflow.NFV9_FIELD_FIRST_SWITCHED:
			startTime = bootTime.Add(time.Duration(decodeUint(valueBytes)) * time.Millisecond)
		case netflow.NFV9_FIELD_LAST_SWITCHED:
			endTime = bootTime.Add(time.Duration(decodeUint(valueBytes)) * time.Millisecond)
		case netflow.IPFIX_FIELD_flowStartSeconds:
			startTime = time.Unix(int64(decodeUint(valueBytes)), 0)
		case netflow.IPFIX_FIELD_flowEndSeconds:
			endTime = time.Unix(int64(decodeUint(valueBytes)), 0)
		case netflow.IPFIX_FIELD_flowStartMilliseconds:
			startTime = time.Unix(0, int64(decodeUint(valueBytes))*int64(time.Millisecond))
		case netflow.IPFIX_FIELD_flowEndMilliseconds:
			endTime = time.Unix(0, int64(decodeUint(valueBytes))*int64(time.Millisecond))
		}
	}

	if startTime.IsZero() && !baseTime.IsZero() {
		startTime = baseTime
	}
	if endTime.IsZero() {
		endTime = startTime
	}

	record := flowRecord{
		Timestamp:   endTime,
		DurationSec: durationSeconds(startTime, endTime),
		Key: flowKey{
			SrcPrefix: ipWithPrefix(srcIP, srcMask),
			DstPrefix: ipWithPrefix(dstIP, dstMask),
			SrcPort:   srcPort,
			DstPort:   dstPort,
			Protocol:  proto,
			SrcAS:     srcAS,
			DstAS:     dstAS,
			InIf:      inIf,
			OutIf:     outIf,
		},
		Bytes:        bytesVal,
		Packets:      packetsVal,
		Flows:        flowsVal,
		SamplingRate: sampling,
		Direction:    direction,
		ExporterIP:   exporter,
		FlowVersion:  flowVersion(version),
	}

	if record.Bytes == 0 && record.Packets == 0 && record.Flows == 0 {
		return flowRecord{}, false
	}

	return record, true
}

func flowVersion(version uint16) string {
	switch version {
	case 9:
		return "v9"
	case 10:
		return "ipfix"
	default:
		return "unknown"
	}
}

func decodeSamplingInterval(raw uint16) int {
	interval := int(raw & 0x3FFF)
	if interval <= 0 {
		return 1
	}
	return interval
}

func ipv4FromLegacy(ip netflowlegacy.IPAddress) net.IP {
	v := uint32(ip)
	return net.IPv4(byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

func decodeUint(value []byte) uint64 {
	var result uint64
	for _, b := range value {
		result = (result << 8) | uint64(b)
	}
	return result
}

func decodeIP(value []byte) net.IP {
	if len(value) == 4 || len(value) == 16 {
		return net.IP(value)
	}
	return nil
}

func decodeDirection(value uint64) string {
	switch value {
	case 0:
		return "ingress"
	case 1:
		return "egress"
	default:
		return ""
	}
}

func ipWithPrefix(ip net.IP, mask int) string {
	if ip == nil || ip.String() == "" {
		return ""
	}
	if mask <= 0 {
		return ip.String()
	}
	return fmt.Sprintf("%s/%d", ip.String(), mask)
}

func decodeSFlowPacket(packet *sflow.Packet, exporter string, now time.Time) []flowRecord {
	if packet == nil {
		return nil
	}
	if exporter == "" && len(packet.AgentIP) > 0 {
		exporter = net.IP(packet.AgentIP).String()
	}

	var records []flowRecord
	for _, sample := range packet.Samples {
		record, ok := decodeSFlowSample(sample, exporter, now)
		if !ok {
			continue
		}
		records = append(records, record)
	}
	return records
}

func decodeSFlowSample(sample interface{}, exporter string, now time.Time) (flowRecord, bool) {
	var (
		samplingRate int
		inIf         int
		outIf        int
		bytesVal     uint64
		packetsVal   uint64
		proto        int
		srcPort      int
		dstPort      int
		srcAS        int
		dstAS        int
		srcMask      int
		dstMask      int
		srcIP        net.IP
		dstIP        net.IP
		records      []sflow.FlowRecord
	)

	switch s := sample.(type) {
	case sflow.FlowSample:
		records = s.Records
		samplingRate = int(s.SamplingRate)
		inIf = int(s.Input)
		outIf = int(s.Output)
	case sflow.ExpandedFlowSample:
		records = s.Records
		samplingRate = int(s.SamplingRate)
		inIf = int(s.InputIfValue)
		outIf = int(s.OutputIfValue)
	default:
		return flowRecord{}, false
	}

	for _, record := range records {
		switch data := record.Data.(type) {
		case sflow.SampledIPv4:
			srcIP = net.IP(data.SrcIP)
			dstIP = net.IP(data.DstIP)
			srcPort = int(data.SrcPort)
			dstPort = int(data.DstPort)
			proto = int(data.Protocol)
			if data.Length > 0 {
				bytesVal = uint64(data.Length)
			}
			if packetsVal == 0 {
				packetsVal = 1
			}
		case sflow.SampledIPv6:
			srcIP = net.IP(data.SrcIP)
			dstIP = net.IP(data.DstIP)
			srcPort = int(data.SrcPort)
			dstPort = int(data.DstPort)
			proto = int(data.Protocol)
			if data.Length > 0 {
				bytesVal = uint64(data.Length)
			}
			if packetsVal == 0 {
				packetsVal = 1
			}
		case sflow.SampledHeader:
			if bytesVal == 0 {
				if data.FrameLength > 0 {
					bytesVal = uint64(data.FrameLength)
				} else if data.OriginalLength > 0 {
					bytesVal = uint64(data.OriginalLength)
				}
			}
			if packetsVal == 0 {
				packetsVal = 1
			}
		case sflow.ExtendedRouter:
			if data.SrcMaskLen > 0 {
				srcMask = int(data.SrcMaskLen)
			}
			if data.DstMaskLen > 0 {
				dstMask = int(data.DstMaskLen)
			}
		case sflow.ExtendedGateway:
			if data.SrcAS > 0 {
				srcAS = int(data.SrcAS)
			} else if data.AS > 0 {
				srcAS = int(data.AS)
			}
			if len(data.ASPath) > 0 {
				dstAS = int(data.ASPath[len(data.ASPath)-1])
			} else if data.AS > 0 {
				dstAS = int(data.AS)
			}
		}
	}

	if bytesVal == 0 && packetsVal == 0 {
		return flowRecord{}, false
	}

	record := flowRecord{
		Timestamp: now,
		Key: flowKey{
			SrcPrefix: ipWithPrefix(srcIP, srcMask),
			DstPrefix: ipWithPrefix(dstIP, dstMask),
			SrcPort:   srcPort,
			DstPort:   dstPort,
			Protocol:  proto,
			SrcAS:     srcAS,
			DstAS:     dstAS,
			InIf:      inIf,
			OutIf:     outIf,
		},
		Bytes:        bytesVal,
		Packets:      packetsVal,
		Flows:        1,
		SamplingRate: samplingRate,
		ExporterIP:   exporter,
		FlowVersion:  "sflow",
	}

	return record, true
}

func durationSeconds(start, end time.Time) int {
	if start.IsZero() || end.IsZero() {
		return 0
	}
	d := end.Sub(start)
	if d < 0 {
		return 0
	}
	return int(d.Seconds())
}
