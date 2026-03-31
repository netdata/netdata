// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"encoding/hex"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
)

func normalizeTopologyDevice(dev topologyDevice) topologyDevice {
	if dev.ChartIDPrefix == "" {
		dev.ChartIDPrefix = topologyProfileChartIDPrefix
	}
	if dev.ChartContextPrefix == "" {
		dev.ChartContextPrefix = topologyProfileChartContextPrefix
	}
	if dev.ManagementIP == "" && len(dev.ManagementAddresses) > 0 {
		if ip := pickManagementIP(dev.ManagementAddresses); ip != "" {
			dev.ManagementIP = ip
		}
	}
	if len(dev.Capabilities) == 0 {
		if len(dev.CapabilitiesEnabled) > 0 {
			dev.Capabilities = dev.CapabilitiesEnabled
		} else if len(dev.CapabilitiesSupported) > 0 {
			dev.Capabilities = dev.CapabilitiesSupported
		}
	}
	if dev.Labels == nil {
		dev.Labels = make(map[string]string)
	}
	if strings.TrimSpace(dev.Labels["type"]) == "" && len(dev.Capabilities) > 0 {
		dev.Labels["type"] = inferCategoryFromCapabilities(dev.Capabilities)
	}
	if dev.ChassisID == "" && dev.ManagementIP != "" {
		dev.ChassisID = dev.ManagementIP
		dev.ChassisIDType = "management_ip"
	}
	if dev.ChassisID != "" && dev.ChassisIDType == "" {
		dev.ChassisIDType = "unknown"
	}
	if value := topologyMetadataValue(dev.Labels, topologyMetadataAliasSysDescr); value != "" && dev.SysDescr == "" {
		dev.SysDescr = value
	}
	if value := topologyMetadataValue(dev.Labels, topologyMetadataAliasSysContact); value != "" && dev.SysContact == "" {
		dev.SysContact = value
	}
	if value := topologyMetadataValue(dev.Labels, topologyMetadataAliasSysLocation); value != "" && dev.SysLocation == "" {
		dev.SysLocation = value
	}
	if value := topologyMetadataValue(dev.Labels, topologyMetadataAliasVendor); value != "" && dev.Vendor == "" {
		dev.Vendor = value
	}
	if value := topologyMetadataValue(dev.Labels, topologyMetadataAliasModel); value != "" && dev.Model == "" {
		dev.Model = value
	}
	if dev.SysUptime <= 0 {
		if value := topologyMetadataValue(dev.Labels, topologyMetadataAliasSysUptime); value != "" {
			dev.SysUptime = parsePositiveInt64(value)
		}
	}
	if value := topologyMetadataValue(dev.Labels, topologyMetadataAliasSerial); value != "" && dev.SerialNumber == "" {
		dev.SerialNumber = value
		setTopologyMetadataLabelIfMissing(dev.Labels, "serial_number", value)
	}
	if value := topologyMetadataValue(dev.Labels, topologyMetadataAliasSoftware); value != "" && dev.SoftwareVersion == "" {
		dev.SoftwareVersion = value
		setTopologyMetadataLabelIfMissing(dev.Labels, "software_version", value)
	}
	if value := topologyMetadataValue(dev.Labels, topologyMetadataAliasFirmware); value != "" && dev.FirmwareVersion == "" {
		dev.FirmwareVersion = value
		setTopologyMetadataLabelIfMissing(dev.Labels, "firmware_version", value)
	}
	if value := topologyMetadataValue(dev.Labels, topologyMetadataAliasHardware); value != "" && dev.HardwareVersion == "" {
		dev.HardwareVersion = value
		setTopologyMetadataLabelIfMissing(dev.Labels, "hardware_version", value)
	}
	return dev
}

func topologyDeviceKey(dev topologyDevice) string {
	if dev.ChassisID == "" {
		return ""
	}
	return dev.ChassisIDType + ":" + dev.ChassisID
}

func normalizeLLDPSubtype(value string, mapping map[string]string) string {
	if v, ok := mapping[value]; ok {
		return v
	}
	return value
}

func appendManagementAddress(addrs []topologyManagementAddress, addr topologyManagementAddress) []topologyManagementAddress {
	if addr.Address == "" {
		return addrs
	}
	for _, existing := range addrs {
		if existing.Address == addr.Address && existing.AddressType == addr.AddressType && existing.Source == addr.Source {
			return addrs
		}
	}
	return append(addrs, addr)
}

func appendCdpManagementAddresses(entry *cdpRemote, current []topologyManagementAddress) []topologyManagementAddress {
	addrs := current
	if entry.primaryMgmtAddr != "" {
		addr, addrType := normalizeManagementAddress(entry.primaryMgmtAddr, entry.primaryMgmtAddrType)
		if addr != "" {
			addrs = appendManagementAddress(addrs, topologyManagementAddress{
				Address:     addr,
				AddressType: addrType,
				Source:      "cdp_primary_mgmt",
			})
		}
	}
	if entry.secondaryMgmtAddr != "" {
		addr, addrType := normalizeManagementAddress(entry.secondaryMgmtAddr, entry.secondaryMgmtAddrType)
		if addr != "" {
			addrs = appendManagementAddress(addrs, topologyManagementAddress{
				Address:     addr,
				AddressType: addrType,
				Source:      "cdp_secondary_mgmt",
			})
		}
	}
	if entry.address != "" {
		addr, addrType := normalizeManagementAddress(entry.address, entry.addressType)
		if addr != "" {
			addrs = appendManagementAddress(addrs, topologyManagementAddress{
				Address:     addr,
				AddressType: addrType,
				Source:      "cdp_cache_address",
			})
		}
	}
	return addrs
}

func pickManagementIP(addrs []topologyManagementAddress) string {
	if len(addrs) == 0 {
		return ""
	}

	ipSet := make(map[string]struct{}, len(addrs))
	ipValues := make([]string, 0, len(addrs))
	rawSet := make(map[string]struct{}, len(addrs))
	rawValues := make([]string, 0, len(addrs))

	for _, addr := range addrs {
		value := strings.TrimSpace(addr.Address)
		if value == "" {
			continue
		}
		if ip := normalizeIPAddress(value); ip != "" {
			if _, exists := ipSet[ip]; exists {
				continue
			}
			ipSet[ip] = struct{}{}
			ipValues = append(ipValues, ip)
			continue
		}
		if _, exists := rawSet[value]; exists {
			continue
		}
		rawSet[value] = struct{}{}
		rawValues = append(rawValues, value)
	}

	if len(ipValues) > 0 {
		sort.Strings(ipValues)
		return ipValues[0]
	}
	if len(rawValues) > 0 {
		sort.Strings(rawValues)
		return rawValues[0]
	}
	return ""
}

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

	if ip := net.ParseIP(rawAddr); ip != nil {
		return ip.String(), normalizeAddressType(rawType, ip.String())
	}

	if bs, err := decodeHexString(rawAddr); err == nil {
		if ip := parseIPFromDecodedBytes(bs); ip != nil {
			return ip.String(), normalizeAddressType(rawType, ip.String())
		}
	}

	return rawAddr, normalizeAddressType(rawType, rawAddr)
}

func normalizeIPAddress(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if ip := net.ParseIP(value); ip != nil {
		return ip.String()
	}

	if bs, err := decodeHexString(value); err == nil {
		if ip := parseIPFromDecodedBytes(bs); ip != nil {
			return ip.String()
		}
	}

	return ""
}

func parseIPFromDecodedBytes(bs []byte) net.IP {
	if len(bs) == net.IPv4len || len(bs) == net.IPv6len {
		ip := net.IP(bs)
		if ip.To16() != nil {
			return ip
		}
	}

	ascii := decodePrintableASCII(bs)
	if ascii == "" {
		return nil
	}

	if ip := net.ParseIP(ascii); ip != nil {
		return ip
	}
	return nil
}

func decodePrintableASCII(bs []byte) string {
	if len(bs) == 0 {
		return ""
	}

	for _, b := range bs {
		if b == 0 {
			continue
		}
		if b < 32 || b > 126 {
			return ""
		}
	}

	s := strings.TrimRight(string(bs), "\x00")
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return s
}

func normalizeMAC(value string) string {
	value = normalizeSNMPHexText(value)
	if value == "" {
		return ""
	}

	if hw, err := net.ParseMAC(value); err == nil {
		return strings.ToLower(hw.String())
	}

	clean := strings.NewReplacer(":", "", "-", "", ".", "", " ", "").Replace(strings.ToLower(value))
	if clean == "" {
		return ""
	}

	bs, err := decodeHexString(clean)
	if err != nil || len(bs) != 6 {
		return ""
	}

	return strings.ToLower(net.HardwareAddr(bs).String())
}

func normalizeHexToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	if mac := normalizeMAC(value); mac != "" {
		return mac
	}
	if ip := normalizeIPAddress(value); ip != "" {
		return ip
	}
	return strings.TrimSpace(value)
}

func normalizeHexIdentifier(value string) string {
	value = normalizeSNMPHexText(value)
	if value == "" {
		return ""
	}

	bs, err := decodeHexString(value)
	if err == nil && len(bs) > 0 {
		return strings.ToLower(hex.EncodeToString(bs))
	}

	clean := strings.NewReplacer(":", "", "-", "", ".", "", " ", "").Replace(strings.ToLower(value))
	if clean == "" {
		return ""
	}
	return clean
}
