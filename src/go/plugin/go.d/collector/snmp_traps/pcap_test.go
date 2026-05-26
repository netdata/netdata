// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"net"
	"os"
	"strings"
	"testing"
)

type pcapGolden struct {
	OID       string `json:"oid"`
	SourceIP  string `json:"source_ip"`
	PeerIP    string `json:"peer_ip"`
	Community string `json:"community"`
	Version   string `json:"version"`
	PduType   string `json:"pdu_type"`
	Varbinds  int    `json:"varbinds"`
}

func TestDecodeTrapFromPcapCorpus(t *testing.T) {
	goldens := readPcapGoldens(t)
	tests := map[string]struct {
		fixture string
	}{
		"arista PEN trap": {
			fixture: "testdata/arista_pen_30065.pcap.hex",
		},
		"aruba PEN trap": {
			fixture: "testdata/aruba_pen_14823.pcap.hex",
		},
		"cisco PEN trap": {
			fixture: "testdata/cisco_pen_9.pcap.hex",
		},
		"hp PEN trap": {
			fixture: "testdata/hp_pen_11.pcap.hex",
		},
		"juniper PEN trap": {
			fixture: "testdata/juniper_pen_2636.pcap.hex",
		},
		"v2c coldStart": {
			fixture: "testdata/v2c_coldstart.pcap.hex",
		},
		"v1 enterpriseSpecific": {
			fixture: "testdata/v1_enterprise_specific.pcap.hex",
		},
		"inform request": {
			fixture: "testdata/inform_request.pcap.hex",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			packets := readPcapUDPPackets(t, tc.fixture)
			if len(packets) != 1 {
				t.Fatalf("expected one UDP packet in %s, got %d", tc.fixture, len(packets))
			}
			packet := packets[0]
			pdu, err := DecodeTrap(packet.payload, packet.peer)
			if err != nil {
				t.Fatalf("DecodeTrap failed: %v", err)
			}
			golden, ok := goldens[tc.fixture]
			if !ok {
				t.Fatalf("missing golden for %s", tc.fixture)
			}
			if pdu.OID != golden.OID {
				t.Errorf("OID mismatch: got %q want %q", pdu.OID, golden.OID)
			}
			if pdu.SourceIP != golden.SourceIP {
				t.Errorf("SourceIP mismatch: got %q want %q", pdu.SourceIP, golden.SourceIP)
			}
			if pdu.PeerIP != golden.PeerIP {
				t.Errorf("PeerIP mismatch: got %q want %q", pdu.PeerIP, golden.PeerIP)
			}
			if pdu.Community != golden.Community {
				t.Errorf("Community mismatch: got %q want %q", pdu.Community, golden.Community)
			}
			if string(pdu.Version) != golden.Version {
				t.Errorf("Version mismatch: got %q want %q", pdu.Version, golden.Version)
			}
			if string(pdu.PduType) != golden.PduType {
				t.Errorf("PduType mismatch: got %q want %q", pdu.PduType, golden.PduType)
			}
			if len(pdu.Varbinds) != golden.Varbinds {
				t.Errorf("Varbinds mismatch: got %d want %d", len(pdu.Varbinds), golden.Varbinds)
			}
		})
	}
}

func readPcapGoldens(t *testing.T) map[string]pcapGolden {
	t.Helper()

	data, err := os.ReadFile("testdata/golden.json")
	if err != nil {
		t.Fatalf("failed to read pcap golden file: %v", err)
	}
	var goldens map[string]pcapGolden
	if err := json.Unmarshal(data, &goldens); err != nil {
		t.Fatalf("failed to decode pcap golden file: %v", err)
	}
	return goldens
}

type pcapUDPPacket struct {
	peer    net.IP
	payload []byte
}

func readPcapUDPPackets(t *testing.T, filename string) []pcapUDPPacket {
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

	var packets []pcapUDPPacket
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

func udpPacketFromEthernetIPv4(frame []byte) (pcapUDPPacket, bool) {
	const (
		ethernetHeaderLen = 14
		ipv4HeaderMinLen  = 20
		udpHeaderLen      = 8
	)
	if len(frame) < ethernetHeaderLen+ipv4HeaderMinLen+udpHeaderLen {
		return pcapUDPPacket{}, false
	}
	if binary.BigEndian.Uint16(frame[12:14]) != 0x0800 {
		return pcapUDPPacket{}, false
	}
	ip := frame[ethernetHeaderLen:]
	if ip[0]>>4 != 4 || ip[9] != 17 {
		return pcapUDPPacket{}, false
	}
	ihl := int(ip[0]&0x0f) * 4
	if ihl < ipv4HeaderMinLen || len(ip) < ihl+udpHeaderLen {
		return pcapUDPPacket{}, false
	}
	totalLen := int(binary.BigEndian.Uint16(ip[2:4]))
	if totalLen < ihl+udpHeaderLen || totalLen > len(ip) {
		return pcapUDPPacket{}, false
	}
	udp := ip[ihl:totalLen]
	udpLen := int(binary.BigEndian.Uint16(udp[4:6]))
	if udpLen < udpHeaderLen || udpLen > len(udp) {
		return pcapUDPPacket{}, false
	}
	return pcapUDPPacket{
		peer:    append(net.IP(nil), ip[12:16]...),
		payload: append([]byte(nil), udp[udpHeaderLen:udpLen]...),
	}, true
}
