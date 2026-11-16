// SPDX-License-Identifier: GPL-3.0-or-later

package ddprofiledefinition

import "strings"

// CommonOIDPrefix returns the complete OID components shared by every input.
// Leading and trailing dots are ignored. The input slice is not modified.
func CommonOIDPrefix(oids []string) string {
	if len(oids) == 0 {
		return ""
	}
	prefix := strings.Trim(oids[0], ".")
	for _, oid := range oids[1:] {
		oid = strings.Trim(oid, ".")
		for prefix != "" && oid != prefix && !strings.HasPrefix(oid, prefix+".") {
			index := strings.LastIndexByte(prefix, '.')
			if index < 0 {
				return ""
			}
			prefix = prefix[:index]
		}
	}
	return prefix
}
