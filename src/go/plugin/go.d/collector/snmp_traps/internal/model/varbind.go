// SPDX-License-Identifier: GPL-3.0-or-later

package model

import (
	"fmt"
	"strings"
)

const RedactedVarbindValue = "<redacted>"

func FindVarbindForProfileOID(varbinds []VarbindValue, profileOID string) (VarbindValue, bool) {
	if profileOID == "" {
		return VarbindValue{}, false
	}
	for _, value := range varbinds {
		if value.OID == profileOID {
			return value, true
		}
	}
	for _, value := range varbinds {
		if OIDMatchesColumn(profileOID, value.OID) {
			return value, true
		}
	}
	return VarbindValue{}, false
}

func FindVarbindByName(varbinds []VarbindValue, name string) (VarbindValue, bool) {
	if name == "" {
		return VarbindValue{}, false
	}
	for _, value := range varbinds {
		if value.Name == name {
			return value, true
		}
	}
	return VarbindValue{}, false
}

func IsSensitiveVarbind(value VarbindValue) bool {
	oid := NormalizeOID(value.OID)
	if oid == SNMPTrapCommunityOID || oid == strings.TrimSuffix(SNMPTrapCommunityOID, ".0") {
		return true
	}
	return strings.TrimSuffix(value.Name, ".0") == "snmpTrapCommunity"
}

func VarbindRawValue(value VarbindValue) string {
	switch v := value.Value.(type) {
	case nil:
		return ""
	case string:
		return v
	case []byte:
		return string(v)
	case int64:
		return fmt.Sprintf("%d", v)
	case uint64:
		return fmt.Sprintf("%d", v)
	case float64:
		return fmt.Sprintf("%v", v)
	case bool:
		if v {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", v)
	}
}
