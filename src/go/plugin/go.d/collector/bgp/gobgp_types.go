// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"time"

	gobgpapi "github.com/osrg/gobgp/v4/api"
)

type gobgpGlobalInfo struct {
	LocalAS  int64
	RouterID string
}

type gobgpPeerInfo struct {
	Peer *gobgpapi.Peer
}

type gobgpRpkiInfo struct {
	Server *gobgpapi.Rpki
}

type gobgpFamilyRef struct {
	ID     string
	VRF    string
	AFI    string
	SAFI   string
	Family *gobgpapi.Family
}

type gobgpValidationSummary struct {
	HasCorrectness bool
	Valid          int64
	Invalid        int64
	NotFound       int64
}

type gobgpValidationCache struct {
	at        time.Time
	summaries map[string]gobgpValidationSummary
}
