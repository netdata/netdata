// SPDX-License-Identifier: GPL-3.0-or-later

package ddprofiledefinition

func valueSourceOID(symbolOID, from, legacyOID string) (oid, field string) {
	switch {
	case symbolOID != "":
		return symbolOID, "symbol.OID"
	case from != "":
		return from, "from"
	case legacyOID != "":
		return legacyOID, "OID"
	default:
		return "", ""
	}
}
