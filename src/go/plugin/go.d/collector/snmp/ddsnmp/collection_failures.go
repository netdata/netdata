// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import "github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"

// FailureCount counts observed failures and keeps only the last safe reason.
// It is a per-attempt summary, not an error or operation history.
type FailureCount struct {
	Count uint64            `json:"count"`
	Last  snmputils.Failure `json:"last"`
}

func (f *FailureCount) Record(failure snmputils.Failure) {
	if failure.Reason == "" {
		return
	}
	if f.Count != ^uint64(0) {
		f.Count++
	}
	f.Last = failure
}

// CollectionFailuresLogicalBytes covers five counted reasons and five processing counters.
const CollectionFailuresLogicalBytes uint64 = 5*(8+snmputils.FailureLogicalBytes) + 5*8

type CollectionFailures struct {
	Profiles   FailureCount            `json:"profiles"`
	GET        FailureCount            `json:"get"`
	WALK       FailureCount            `json:"walk"`
	BGP        FailureCount            `json:"bgp"`
	Licensing  FailureCount            `json:"licensing"`
	Processing ProcessingFailureCounts `json:"processing"`
}

type ProcessingFailureCounts struct {
	Preparation int64 `json:"preparation"`
	Scalar      int64 `json:"scalar"`
	Table       int64 `json:"table"`
	BGP         int64 `json:"bgp"`
	Licensing   int64 `json:"licensing"`
}

func (f CollectionFailures) Valid() bool {
	for _, c := range []FailureCount{f.Profiles, f.GET, f.WALK, f.BGP, f.Licensing} {
		if !c.Last.Valid() || (c.Count == 0) != (c.Last.Reason == "") {
			return false
		}
	}
	return f.Processing.Preparation >= 0 && f.Processing.Scalar >= 0 && f.Processing.Table >= 0 && f.Processing.BGP >= 0 &&
		f.Processing.Licensing >= 0
}
