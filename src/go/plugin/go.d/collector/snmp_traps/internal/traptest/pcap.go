// SPDX-License-Identifier: GPL-3.0-or-later

package traptest

import (
	"encoding/binary"
	"encoding/hex"
	"net"
	"os"
	"strings"
	"testing"
)

type UDPPacket struct {
	Peer    net.IP
	Payload []byte
}

func ReadPcapUDPPackets(t testing.TB, filename string) []UDPPacket {
	t.Helper()

	hexData, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("failed to read pcap fixture %s: %v", filename, err)
	}
	raw, err := hex.DecodeString(strings.Join(strings.Fields(string(hexData)), ""))
	if err != nil {
		t.Fatalf("failed to decode pcap fixture %s: %v", filename, err)
	}
	if len(raw) < 24 {
		t.Fatalf("pcap fixture %s is too short", filename)
	}
	if binary.LittleEndian.Uint32(raw[:4]) != 0xa1b2c3d4 {
		t.Fatalf("pcap fixture %s has unsupported magic", filename)
	}
	if network := binary.LittleEndian.Uint32(raw[20:24]); network != 1 {
		t.Fatalf("pcap fixture %s has unsupported link type %d", filename, network)
	}

	var packets []UDPPacket
	for off := 24; off < len(raw); {
		if off+16 > len(raw) {
			t.Fatalf("pcap fixture %s has truncated packet header", filename)
		}
		inclLen := int(binary.LittleEndian.Uint32(raw[off+8 : off+12]))
		off += 16
		if inclLen < 0 || off+inclLen > len(raw) {
			t.Fatalf("pcap fixture %s has invalid packet length %d", filename, inclLen)
		}
		frame := raw[off : off+inclLen]
		off += inclLen
		packet, ok := udpPacketFromEthernetIPv4(frame)
		if ok {
			packets = append(packets, packet)
		}
	}
	return packets
}

func udpPacketFromEthernetIPv4(frame []byte) (UDPPacket, bool) {
	const (
		ethernetHeaderLen = 14
		ipv4HeaderMinLen  = 20
		udpHeaderLen      = 8
	)
	if len(frame) < ethernetHeaderLen+ipv4HeaderMinLen+udpHeaderLen {
		return UDPPacket{}, false
	}
	if binary.BigEndian.Uint16(frame[12:14]) != 0x0800 {
		return UDPPacket{}, false
	}
	ip := frame[ethernetHeaderLen:]
	if ip[0]>>4 != 4 || ip[9] != 17 {
		return UDPPacket{}, false
	}
	ihl := int(ip[0]&0x0f) * 4
	if ihl < ipv4HeaderMinLen || len(ip) < ihl+udpHeaderLen {
		return UDPPacket{}, false
	}
	totalLen := int(binary.BigEndian.Uint16(ip[2:4]))
	if totalLen < ihl+udpHeaderLen || totalLen > len(ip) {
		return UDPPacket{}, false
	}
	udp := ip[ihl:totalLen]
	udpLen := int(binary.BigEndian.Uint16(udp[4:6]))
	if udpLen < udpHeaderLen || udpLen > len(udp) {
		return UDPPacket{}, false
	}
	return UDPPacket{
		Peer:    append(net.IP(nil), ip[12:16]...),
		Payload: append([]byte(nil), udp[udpHeaderLen:udpLen]...),
	}, true
}
