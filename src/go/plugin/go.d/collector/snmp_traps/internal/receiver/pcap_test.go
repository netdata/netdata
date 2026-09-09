// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/traptest"
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
			fixture: "../../testdata/arista_pen_30065.pcap.hex",
		},
		"aruba PEN trap": {
			fixture: "../../testdata/aruba_pen_14823.pcap.hex",
		},
		"cisco PEN trap": {
			fixture: "../../testdata/cisco_pen_9.pcap.hex",
		},
		"hp PEN trap": {
			fixture: "../../testdata/hp_pen_11.pcap.hex",
		},
		"juniper PEN trap": {
			fixture: "../../testdata/juniper_pen_2636.pcap.hex",
		},
		"v2c coldStart": {
			fixture: "../../testdata/v2c_coldstart.pcap.hex",
		},
		"v1 enterpriseSpecific": {
			fixture: "../../testdata/v1_enterprise_specific.pcap.hex",
		},
		"inform request": {
			fixture: "../../testdata/inform_request.pcap.hex",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			packets := traptest.ReadPcapUDPPackets(t, tc.fixture)
			if len(packets) != 1 {
				t.Fatalf("expected one UDP packet in %s, got %d", tc.fixture, len(packets))
			}
			packet := packets[0]
			ctx, err := decodeTrap(packet.Payload, packet.Peer, nil)
			if err != nil {
				t.Fatalf("decodeTrap failed: %v", err)
			}
			pdu := ctx.PDU
			golden, ok := goldens[strings.TrimPrefix(tc.fixture, "../../")]
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

	data, err := os.ReadFile("../../testdata/golden.json")
	if err != nil {
		t.Fatalf("failed to read pcap golden file: %v", err)
	}
	var goldens map[string]pcapGolden
	if err := json.Unmarshal(data, &goldens); err != nil {
		t.Fatalf("failed to decode pcap golden file: %v", err)
	}
	return goldens
}
