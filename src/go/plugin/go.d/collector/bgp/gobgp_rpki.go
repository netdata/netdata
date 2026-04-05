// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"net"
	"strconv"
	"strings"
	"time"

	gobgpapi "github.com/osrg/gobgp/v4/api"
)

func buildGoBGPRPKICaches(now time.Time, servers []*gobgpRpkiInfo) []rpkiCacheStats {
	caches := make([]rpkiCacheStats, 0, len(servers))

	for _, server := range servers {
		if server == nil || server.Server == nil {
			continue
		}

		name := gobgpRPKICacheName(server.Server.GetConf().GetAddress(), server.Server.GetConf().GetRemotePort())
		up := server.Server.GetState().GetUp()
		stateText := "down"
		if up {
			stateText = "up"
		}

		caches = append(caches, rpkiCacheStats{
			ID:          idPart(name),
			Backend:     backendGoBGP,
			Name:        name,
			StateText:   stateText,
			Up:          up,
			HasUptime:   true,
			UptimeSecs:  gobgpRPKICacheUptime(now, server.Server),
			HasRecords:  true,
			RecordIPv4:  int64(server.Server.GetState().GetRecordIpv4()),
			RecordIPv6:  int64(server.Server.GetState().GetRecordIpv6()),
			HasPrefixes: true,
			PrefixIPv4:  int64(server.Server.GetState().GetPrefixIpv4()),
			PrefixIPv6:  int64(server.Server.GetState().GetPrefixIpv6()),
		})
	}

	return caches
}

func gobgpRPKICacheName(address string, port uint32) string {
	address = strings.TrimSpace(address)
	switch {
	case address == "" && port == 0:
		return "unknown"
	case port == 0:
		return address
	case address == "":
		return strconv.FormatUint(uint64(port), 10)
	default:
		return net.JoinHostPort(address, strconv.FormatUint(uint64(port), 10))
	}
}

func gobgpRPKICacheUptime(now time.Time, server *gobgpapi.Rpki) int64 {
	if server == nil || server.GetState() == nil || !server.GetState().GetUp() {
		return 0
	}

	started := server.GetState().GetUptime().AsTime()
	if started.IsZero() || now.Before(started) {
		return 0
	}

	return int64(now.Sub(started).Seconds())
}
