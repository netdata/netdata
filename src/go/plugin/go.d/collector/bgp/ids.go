// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"hash/fnv"
	"strconv"
	"strings"
)

func makeFamilyID(vrf, afi, safi string) string {
	return joinID(idPart(vrf), idPart(afi), idPart(safi))
}

func makePeerID(familyID, address string) string {
	return makePeerIDWithScope(familyID, address, "")
}

func makePeerIDWithScope(familyID, address, scope string) string {
	if strings.TrimSpace(scope) == "" {
		return joinID(familyID, idPart(address))
	}
	return joinID(familyID, idPart(address), idPart(scope))
}

func makeNeighborID(vrf, address string) string {
	return makeNeighborIDWithScope(vrf, address, "")
}

func makeNeighborIDWithScope(vrf, address, scope string) string {
	if strings.TrimSpace(scope) == "" {
		return joinID(idPart(vrf), idPart(address))
	}
	return joinID(idPart(vrf), idPart(address), idPart(scope))
}

func makeVNIID(tenantVRF string, vni int64, kind string) string {
	return joinID(idPart(tenantVRF), idPart(kind), strconv.FormatInt(vni, 10))
}

func joinID(parts ...string) string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(part, "_")
		if part == "" {
			continue
		}
		filtered = append(filtered, part)
	}
	if len(filtered) == 0 {
		return "unknown"
	}

	id := strings.Join(filtered, "_")
	if len(id) <= 180 {
		return id
	}

	h := fnv.New64a()
	_, _ = h.Write([]byte(id))
	checksum := strconv.FormatUint(h.Sum64(), 36)
	return id[:140] + "_" + checksum
}

func idPart(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "unknown"
	}

	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_':
			b.WriteString("__")
		default:
			b.WriteString("_x")
			b.WriteString(strconv.FormatInt(int64(r), 16))
			b.WriteByte('_')
		}
	}

	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "unknown"
	}
	return out
}
