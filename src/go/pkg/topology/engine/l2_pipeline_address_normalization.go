// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"encoding/hex"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
)

func canonicalBridgeAddr(value, fallback string) string {
	if mac := normalizeMAC(value); mac != "" && mac != "00:00:00:00:00:00" {
		return mac
	}
	if mac := normalizeMAC(fallback); mac != "" && mac != "00:00:00:00:00:00" {
		return mac
	}
	return ""
}

func primaryL2MACIdentity(chassisID, baseBridgeAddress string) string {
	for _, candidate := range []string{chassisID, baseBridgeAddress} {
		if mac := normalizeMAC(candidate); mac != "" && mac != "00:00:00:00:00:00" {
			return mac
		}
	}
	return ""
}

func canonicalIP(v string) string {
	if ip := parseAddr(v); ip.IsValid() {
		return ip.String()
	}
	if ip := parseAddr(decodeHexIP(v)); ip.IsValid() {
		return ip.String()
	}
	return ""
}

func parseAddr(v string) netip.Addr {
	addr, err := netip.ParseAddr(strings.TrimSpace(v))
	if err != nil {
		return netip.Addr{}
	}
	return addr.Unmap()
}

func decodeHexIP(v string) string {
	bs := decodeHexBytes(v)
	if len(bs) == 4 {
		addr, ok := netip.AddrFromSlice(bs)
		if ok {
			return addr.Unmap().String()
		}
	}
	if len(bs) == 16 {
		addr, ok := netip.AddrFromSlice(bs)
		if ok {
			return addr.String()
		}
	}
	return ""
}

func decodeHexBytes(v string) []byte {
	clean := strings.ToLower(strings.TrimSpace(v))
	clean = strings.TrimPrefix(clean, "0x")
	if clean == "" {
		return nil
	}

	if strings.ContainsAny(clean, ":-. \t") {
		parts := strings.FieldsFunc(clean, func(r rune) bool {
			return r == ':' || r == '-' || r == '.' || r == ' ' || r == '\t'
		})
		if len(parts) == 0 {
			return nil
		}
		if bs := decodeGroupedHexParts(parts); len(bs) != 0 {
			return bs
		}

		out := make([]byte, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if len(part) > 2 {
				return nil
			}
			if len(part) == 1 {
				part = "0" + part
			}
			b, err := hex.DecodeString(part)
			if err != nil || len(b) != 1 {
				return nil
			}
			out = append(out, b[0])
		}
		if len(out) == 0 {
			return nil
		}
		return out
	}

	if len(clean)%2 == 1 {
		clean = "0" + clean
	}
	bs, err := hex.DecodeString(clean)
	if err != nil {
		return nil
	}
	return bs
}

func decodeGroupedHexParts(parts []string) []byte {
	var joined strings.Builder
	anyWidePart := false

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if len(part)%2 != 0 {
			return nil
		}
		if len(part) > 2 {
			anyWidePart = true
		}
		joined.WriteString(part)
	}

	if !anyWidePart || joined.Len() == 0 {
		return nil
	}

	bs, err := hex.DecodeString(joined.String())
	if err != nil || len(bs) == 0 {
		return nil
	}
	return bs
}

func normalizeMAC(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}

	if bs := parseDottedDecimalBytes(v); len(bs) == 6 {
		return formatMAC(bs)
	}
	if bs := decodeHexBytes(v); len(bs) == 6 {
		return formatMAC(bs)
	}
	return ""
}

func parseDottedDecimalBytes(v string) []byte {
	parts := strings.Split(strings.TrimSpace(v), ".")
	if len(parts) != 6 {
		return nil
	}
	out := make([]byte, 0, 6)
	for _, part := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || n < 0 || n > 255 {
			return nil
		}
		out = append(out, byte(n))
	}
	return out
}

func formatMAC(bs []byte) string {
	if len(bs) != 6 {
		return ""
	}
	parts := make([]string, 0, 6)
	for _, b := range bs {
		parts = append(parts, fmt.Sprintf("%02x", b))
	}
	return strings.Join(parts, ":")
}
