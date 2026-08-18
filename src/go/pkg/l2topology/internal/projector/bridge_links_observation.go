// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"strings"
)

func bridgePortObservationKey(port bridgePortRef) string {
	return bridgePortCanonicalIdentity(port)
}

func bridgePortObservationVLANKey(port bridgePortRef) string {
	base := bridgePortObservationBaseKey(port)
	if base == "" {
		return ""
	}
	return base + keySep + "scope:" + strings.ToLower(bridgePortForwardingDomain(port))
}

func bridgePortObservationBaseKey(port bridgePortRef) string {
	return bridgePortCanonicalIdentity(port)
}

func addBridgePortObservationKeys(set map[string]struct{}, port bridgePortRef) {
	if set == nil {
		return
	}
	if key := bridgePortObservationKey(port); key != "" {
		set[key] = struct{}{}
	}
	if key := bridgePortObservationVLANKey(port); key != "" {
		set[key] = struct{}{}
	}
}
