// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

func reconstructLldpRemMgmtAddrHex(tags map[string]string) string {
	lengthStr := strings.TrimSpace(tags[tagLldpRemMgmtAddrLen])
	length, err := strconv.Atoi(lengthStr)
	if err != nil || length <= 0 || length > net.IPv6len {
		return ""
	}

	addr := make([]byte, 0, length)
	for i := 1; i <= length; i++ {
		tag := fmt.Sprintf("%s%d", tagLldpRemMgmtAddrOctetPref, i)
		v := strings.TrimSpace(tags[tag])
		if v == "" {
			return ""
		}
		octet, err := strconv.Atoi(v)
		if err != nil || octet < 0 || octet > 255 {
			return ""
		}
		addr = append(addr, byte(octet))
	}

	return hex.EncodeToString(addr)
}

func normalizeManagementAddress(rawAddr, rawType string) (string, string) {
	rawAddr = strings.TrimSpace(rawAddr)
	if rawAddr == "" {
		return "", normalizeAddressType(rawType, "")
	}

	if ip := topologyutil.NormalizeIPAddress(rawAddr); ip != "" {
		return ip, normalizeAddressType(rawType, ip)
	}

	return rawAddr, normalizeAddressType(rawType, rawAddr)
}
