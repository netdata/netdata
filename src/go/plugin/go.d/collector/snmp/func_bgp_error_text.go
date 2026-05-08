// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import "fmt"

var bgpErrorCodeText = map[int64]string{
	1: "Message Header Error",
	2: "OPEN Message Error",
	3: "UPDATE Message Error",
	4: "Hold Timer Expired",
	5: "Finite State Machine Error",
	6: "Cease",
	7: "ROUTE-REFRESH Message Error",
	8: "Send Hold Timer Expired",
	9: "Loss of LSDB Synchronization",
}

var bgpErrorSubcodeText = map[int64]map[int64]string{
	1: {
		0: "Unspecific",
		1: "Connection Not Synchronized",
		2: "Bad Message Length",
		3: "Bad Message Type",
	},
	2: {
		0:  "Unspecific",
		1:  "Unsupported Version Number",
		2:  "Bad Peer AS",
		3:  "Bad BGP Identifier",
		4:  "Unsupported Optional Parameter",
		5:  "Deprecated",
		6:  "Unacceptable Hold Time",
		7:  "Unsupported Capability",
		8:  "Deprecated",
		9:  "Deprecated",
		10: "Deprecated",
		11: "Role Mismatch",
	},
	3: {
		0:  "Unspecific",
		1:  "Malformed Attribute List",
		2:  "Unrecognized Well-known Attribute",
		3:  "Missing Well-known Attribute",
		4:  "Attribute Flags Error",
		5:  "Attribute Length Error",
		6:  "Invalid ORIGIN Attribute",
		7:  "Deprecated",
		8:  "Invalid NEXT_HOP Attribute",
		9:  "Optional Attribute Error",
		10: "Invalid Network Field",
		11: "Malformed AS_PATH",
	},
	5: {
		0: "Unspecified Error",
		1: "Receive Unexpected Message in OpenSent State",
		2: "Receive Unexpected Message in OpenConfirm State",
		3: "Receive Unexpected Message in Established State",
	},
	6: {
		1:  "Maximum Number of Prefixes Reached",
		2:  "Administrative Shutdown",
		3:  "Peer De-configured",
		4:  "Administrative Reset",
		5:  "Connection Rejected",
		6:  "Other Configuration Change",
		7:  "Connection Collision Resolution",
		8:  "Out of Resources",
		9:  "Hard Reset",
		10: "BFD Down",
	},
	7: {
		0: "Reserved",
		1: "Invalid Message Length",
	},
}

func bgpLastErrorText(code, subcode *int64) string {
	if code == nil {
		return ""
	}
	if *code == 0 && (subcode == nil || *subcode == 0) {
		return ""
	}

	codeText, ok := bgpErrorCodeText[*code]
	if !ok {
		if subcode == nil {
			return fmt.Sprintf("Unknown error code %d", *code)
		}
		return fmt.Sprintf("Unknown error code %d / subcode %d", *code, *subcode)
	}

	if subcode == nil {
		return codeText
	}

	if subcodes, ok := bgpErrorSubcodeText[*code]; ok {
		if subcodeText, ok := subcodes[*subcode]; ok {
			return fmt.Sprintf("%s - %s", codeText, subcodeText)
		}
	}

	return fmt.Sprintf("%s (subcode %d)", codeText, *subcode)
}
