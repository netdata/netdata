// SPDX-License-Identifier: GPL-3.0-or-later

package model

import "strings"

const (
	SysUpTimeOID          = "1.3.6.1.2.1.1.3.0"
	SNMPTrapOID           = "1.3.6.1.6.3.1.1.4.1.0"
	SNMPTrapAddressOID    = "1.3.6.1.6.3.18.1.3.0"
	SNMPTrapCommunityOID  = "1.3.6.1.6.3.18.1.4.0"
	SNMPTrapEnterpriseOID = "1.3.6.1.6.3.1.1.4.3.0"
)

func NormalizeOID(oid string) string {
	return strings.TrimPrefix(oid, ".")
}

func AlternateTrapOID(oid string) string {
	if len(oid) == 0 || oid[0] == '.' || oid[len(oid)-1] == '.' {
		return oid
	}

	dots := 0
	prevDot := -1
	lastDot := -1
	segmentStart := 0

	for i := 0; i < len(oid); i++ {
		c := oid[i]
		switch {
		case c == '.':
			if i == segmentStart {
				return oid
			}
			dots++
			prevDot = lastDot
			lastDot = i
			segmentStart = i + 1
		case c < '0' || c > '9':
			return oid
		}
	}

	if dots < 3 || prevDot < 0 || lastDot <= 0 || lastDot >= len(oid)-1 {
		return oid
	}

	if oid[prevDot+1:lastDot] == "0" {
		return oid[:prevDot] + oid[lastDot:]
	}
	return oid[:lastDot] + ".0" + oid[lastDot:]
}

func IsNumericOID(oid string) bool {
	if oid == "" || oid[0] == '.' || oid[len(oid)-1] == '.' {
		return false
	}
	for part := range strings.SplitSeq(oid, ".") {
		if part == "" {
			return false
		}
		for _, ch := range part {
			if ch < '0' || ch > '9' {
				return false
			}
		}
	}
	return true
}

func OIDMatchesColumn(profileOID, observedOID string) bool {
	if profileOID == "" || observedOID == "" || profileOID == observedOID {
		return false
	}
	return strings.HasPrefix(observedOID, profileOID+".")
}
