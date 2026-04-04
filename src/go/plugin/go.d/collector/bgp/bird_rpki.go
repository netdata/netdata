// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import "strings"

func buildBIRDRPKICaches(protocols []birdProtocol) []rpkiCacheStats {
	caches := make([]rpkiCacheStats, 0)

	for _, proto := range protocols {
		if !strings.EqualFold(proto.Proto, "RPKI") {
			continue
		}

		up, stateText := mapBIRDRPKICacheState(proto)
		uptime := int64(0)
		if up {
			uptime = proto.UptimeSecs
		}

		caches = append(caches, rpkiCacheStats{
			ID:         idPart(proto.Name),
			Backend:    backendBIRD,
			Name:       proto.Name,
			Desc:       proto.Description,
			StateText:  stateText,
			Up:         up,
			HasUptime:  true,
			UptimeSecs: uptime,
		})
	}

	return caches
}

func mapBIRDRPKICacheState(proto birdProtocol) (bool, string) {
	stateText := strings.TrimSpace(proto.Info)
	if stateText == "" {
		stateText = strings.TrimSpace(proto.Status)
	}

	normalizedState := strings.ToLower(strings.TrimSpace(stateText))
	normalizedStatus := strings.ToLower(strings.TrimSpace(proto.Status))

	switch {
	case strings.Contains(normalizedState, "established"):
		return true, stateText
	case normalizedStatus == "up":
		if stateText == "" {
			return true, "up"
		}
		return true, stateText
	default:
		if stateText == "" {
			return false, "down"
		}
		return false, stateText
	}
}
