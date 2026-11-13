// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

func chartIDFromName(name string) string {
	if isBGPChartMetricName(name) {
		return "snmp_" + cleanMetricName.Replace(name)
	}
	return "snmp_device_prof_" + cleanMetricName.Replace(name)
}

func metricIDFromName(name string, subkeys ...string) string {
	if isBGPChartMetricName(name) {
		return cleanedMetricID("snmp_", name, subkeys...)
	}
	return rawMetricID("snmp_device_prof_", name, subkeys...)
}

func metricIDFromKey(key string, subkeys ...string) string {
	if len(subkeys) == 0 {
		return key
	}
	// Frame job-scoped sample keys; V1 still emits the chart-scoped dimension Name when set.
	var storage [128]byte
	encoded := storage[:0]
	for _, subkey := range subkeys {
		encoded = appendIdentityPart(encoded, subkey)
	}
	digest := sha256.Sum256(encoded)
	var id strings.Builder
	id.Grow(len(key) + len("_dim_") + hex.EncodedLen(len(digest)))
	id.WriteString(key)
	id.WriteString("_dim_")
	var digestHex [sha256.Size * 2]byte
	hex.Encode(digestHex[:], digest[:])
	id.Write(digestHex[:])
	return id.String()
}

// tableMetricKey owns logical row identity. Display tags do not partition a row;
// every other tag, including an explicitly empty value, does. Length prefixes
// preserve field boundaries, while the digest keeps IDs below protocol limits.
func tableMetricKey(m ddsnmp.Metric) string {
	if m.Name == "" {
		return ""
	}
	keys := make([]string, 0, len(m.Tags))
	for key := range m.Tags {
		if strings.HasPrefix(key, "_") {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	// Most row identities fit on the stack; unusually long custom labels grow
	// normally without retaining a device-sized buffer between collections.
	var storage [512]byte
	encoded := storage[:0]
	encoded = appendIdentityPart(encoded, "snmp-table-v1")
	encoded = appendIdentityPart(encoded, m.Name)
	for _, key := range keys {
		encoded = appendIdentityPart(encoded, key)
		encoded = appendIdentityPart(encoded, m.Tags[key])
	}
	digest := sha256.Sum256(encoded)
	// This prefix is only a reading aid. The full original name is in the digest.
	// The final key is already chart-safe; consumers must not normalize it again.
	const prefixLimit = 64
	var id strings.Builder
	prefix := "snmp_device_prof_"
	if isBGPChartMetricName(m.Name) {
		prefix = "snmp_"
	}
	id.Grow(len(prefix) + min(len(m.Name), prefixLimit) + 1 + hex.EncodedLen(len(digest)))
	id.WriteString(prefix)
	for i := 0; i < len(m.Name) && i < prefixLimit; i++ {
		ch := m.Name[i]
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '_') {
			ch = '_'
		}
		id.WriteByte(ch)
	}
	id.WriteByte('_')
	var digestHex [sha256.Size * 2]byte
	hex.Encode(digestHex[:], digest[:])
	id.Write(digestHex[:])
	return id.String()
}

func appendIdentityPart(dst []byte, value string) []byte {
	dst = binary.AppendUvarint(dst, uint64(len(value)))
	return append(dst, value...)
}

func rawMetricID(prefix, base string, subkeys ...string) string {
	var id strings.Builder
	id.WriteString(prefix + base)
	for _, subkey := range subkeys {
		id.WriteString("_" + subkey)
	}
	return id.String()
}

func cleanedMetricID(prefix, base string, subkeys ...string) string {
	var id strings.Builder
	id.WriteString(prefix + cleanMetricName.Replace(base))
	for _, subkey := range subkeys {
		id.WriteString("_" + cleanMetricName.Replace(subkey))
	}
	return id.String()
}

func chartContextID(name string) string {
	if isBGPChartMetricName(name) {
		return "snmp." + name
	}
	return "snmp.device_prof_" + cleanMetricName.Replace(name)
}

func isBGPChartMetricName(name string) bool {
	return strings.HasPrefix(name, "bgp.")
}
