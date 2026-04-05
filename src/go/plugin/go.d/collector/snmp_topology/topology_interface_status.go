// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

func normalizeInterfaceAdminStatus(value string) string {
	value = canonicalSNMPEnumValue(value)
	switch value {
	case "1":
		return "up"
	case "2":
		return "down"
	case "3":
		return "testing"
	case "up", "down", "testing":
		return value
	default:
		return ""
	}
}

func normalizeInterfaceOperStatus(value string) string {
	value = canonicalSNMPEnumValue(value)
	switch value {
	case "1":
		return "up"
	case "2":
		return "down"
	case "3":
		return "testing"
	case "4":
		return "unknown"
	case "5":
		return "dormant"
	case "6":
		return "notPresent"
	case "7":
		return "lowerLayerDown"
	case "up", "down", "testing", "unknown", "dormant":
		return value
	case "notpresent":
		return "notPresent"
	case "not_present":
		return "notPresent"
	case "lowerlayerdown":
		return "lowerLayerDown"
	case "lower_layer_down":
		return "lowerLayerDown"
	default:
		return ""
	}
}

func normalizeInterfaceDuplex(value string) string {
	value = canonicalSNMPEnumValue(value)
	switch value {
	case "1", "unknown":
		return "unknown"
	case "2", "half", "halfduplex", "half_duplex":
		return "half"
	case "3", "full", "fullduplex", "full_duplex":
		return "full"
	default:
		return ""
	}
}
