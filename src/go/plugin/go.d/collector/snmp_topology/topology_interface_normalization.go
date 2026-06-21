// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
	"strconv"
	"strings"
)

func normalizeInterfaceType(value string) string {
	value = topologyutil.CanonicalSNMPEnumValue(value)
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if name, ok := ianaIfTypeByNumber[value]; ok {
		return name
	}
	if _, err := strconv.Atoi(value); err == nil {
		return "type-" + value
	}
	return strings.ToLower(strings.NewReplacer("_", "", "-", "", " ", "").Replace(value))
}
