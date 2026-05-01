// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

func decodeLLDPCapabilities(value string) []string {
	bs, err := decodeHexString(value)
	if err != nil {
		return nil
	}

	names := []string{
		"other",
		"repeater",
		"bridge",
		"wlanAccessPoint",
		"router",
		"telephone",
		"docsisCableDevice",
		"stationOnly",
		"cVlanComponent",
		"sVlanComponent",
		"twoPortMacRelay",
	}

	caps := make([]string, 0, len(names))
	for bit, name := range names {
		if bitSet(bs, bit) {
			caps = append(caps, name)
		}
	}
	return caps
}

func inferCategoryFromCapabilities(caps []string) string {
	has := make(map[string]bool, len(caps))
	for _, c := range caps {
		has[c] = true
	}
	switch {
	case has["router"]:
		return "router"
	case has["wlanAccessPoint"]:
		return "access point"
	case has["telephone"]:
		return "voip"
	case has["bridge"]:
		return "switch"
	case has["repeater"]:
		return "switch"
	case has["docsisCableDevice"]:
		return "network device"
	default:
		return ""
	}
}

func bitSet(bs []byte, bit int) bool {
	idx := bit / 8
	if idx < 0 || idx >= len(bs) {
		return false
	}
	mask := byte(1 << uint(7-(bit%8)))
	return bs[idx]&mask != 0
}
