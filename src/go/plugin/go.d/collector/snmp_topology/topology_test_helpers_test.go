// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

func containsMgmtAddr(snapshot topologyData, addrs map[string]struct{}) bool {
	for _, actor := range snapshot.Actors {
		for _, ip := range actor.Match.IPAddresses {
			if _, ok := addrs[ip]; ok {
				return true
			}
		}
	}
	for _, link := range snapshot.Links {
		for _, ip := range link.Src.Match.IPAddresses {
			if _, ok := addrs[ip]; ok {
				return true
			}
		}
		for _, ip := range link.Dst.Match.IPAddresses {
			if _, ok := addrs[ip]; ok {
				return true
			}
		}
	}
	return false
}
