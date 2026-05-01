// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"bytes"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

type summaryRequest struct {
	AFI      string
	SAFI     string
	Required bool
}

func summaryRequests() []summaryRequest {
	return []summaryRequest{
		{AFI: "ipv4", SAFI: "unicast", Required: true},
		{AFI: "ipv6", SAFI: "unicast", Required: true},
		{AFI: "l2vpn", SAFI: "evpn", Required: false},
	}
}

func unmarshalFRRSummary(data []byte, requestedAFI, requestedSAFI string) (map[string]map[string]frrSummaryFamily, error) {
	if requestedAFI == "l2vpn" && requestedSAFI == "evpn" {
		var raw map[string]frrSummaryFamily
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, err
		}
		return wrapFRREVPNSummaries(raw), nil
	}

	var raw map[string]map[string]frrSummaryFamily
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func wrapFRREVPNSummaries(raw map[string]frrSummaryFamily) map[string]map[string]frrSummaryFamily {
	wrapped := make(map[string]map[string]frrSummaryFamily, len(raw))
	for vrfName, summary := range raw {
		wrapped[vrfName] = map[string]frrSummaryFamily{
			"l2VpnEvpn": summary,
		}
	}
	return wrapped
}

func parseFRREVPNVNIs(data []byte, backend string) ([]vniStats, error) {
	if len(bytes.TrimSpace(data)) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("{}")) {
		return nil, nil
	}

	var raw map[string]frrEVPNVNI
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	vnis := make([]vniStats, 0, len(raw))
	for key, entry := range raw {
		vni := entry.VNI
		if vni == 0 {
			parsed, err := strconv.ParseInt(strings.TrimSpace(key), 10, 64)
			if err == nil {
				vni = parsed
			}
		}

		kind := normalizeVNIType(entry.Type)
		tenantVRF := strings.TrimSpace(entry.TenantVRF)
		stat := vniStats{
			ID:          makeVNIID(tenantVRF, vni, kind),
			Backend:     backend,
			TenantVRF:   tenantVRF,
			Type:        kind,
			VXLANIf:     strings.TrimSpace(entry.VXLANIf),
			VNI:         vni,
			MACs:        entry.NumMACs,
			ARPND:       entry.NumARPND,
			RemoteVTEPs: parseFRRRemoteVTEPs(entry.NumRemoteVTEPs),
		}
		vnis = append(vnis, stat)
	}

	sort.Slice(vnis, func(i, j int) bool {
		if vnis[i].TenantVRF != vnis[j].TenantVRF {
			return vnis[i].TenantVRF < vnis[j].TenantVRF
		}
		if vnis[i].VNI != vnis[j].VNI {
			return vnis[i].VNI < vnis[j].VNI
		}
		return vnis[i].Type < vnis[j].Type
	})

	return vnis, nil
}

func normalizeVNIType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "unknown"
	}
	return value
}

func parseFRRRemoteVTEPs(raw json.RawMessage) int64 {
	if len(bytes.TrimSpace(raw)) == 0 {
		return 0
	}

	var count int64
	if err := json.Unmarshal(raw, &count); err == nil {
		return count
	}

	var floatCount float64
	if err := json.Unmarshal(raw, &floatCount); err == nil {
		return int64(floatCount)
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		text = strings.TrimSpace(strings.ToLower(text))
		if text == "" || text == "n/a" {
			return -1
		}
		if parsed, err := strconv.ParseInt(text, 10, 64); err == nil {
			return parsed
		}
	}

	return -1
}
