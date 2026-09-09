// SPDX-License-Identifier: GPL-3.0-or-later

package projector

func bridgePortObservationKey(port bridgePortRef) string {
	return bridgePortCanonicalIdentity(port)
}

func bridgePortObservationVLANKey(port bridgePortRef) string {
	base := bridgePortObservationKey(port)
	if base == "" {
		return ""
	}
	return base + keySep + "scope:" + bridgePortForwardingDomain(port)
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
