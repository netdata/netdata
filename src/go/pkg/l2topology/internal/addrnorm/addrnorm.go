// SPDX-License-Identifier: GPL-3.0-or-later

package addrnorm

import (
	"encoding/hex"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
)

func DecodeHexBytes(value string) []byte {
	clean := strings.ToLower(strings.TrimSpace(value))
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

func NormalizeMAC(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	if bs := parseDottedDecimalBytes(value); len(bs) == 6 {
		return formatMAC(bs)
	}
	if bs := DecodeHexBytes(value); len(bs) == 6 {
		return formatMAC(bs)
	}
	return ""
}

func ParseAddr(value string) netip.Addr {
	addr, err := netip.ParseAddr(strings.TrimSpace(value))
	if err != nil {
		return netip.Addr{}
	}
	return addr.Unmap()
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

func parseDottedDecimalBytes(value string) []byte {
	parts := strings.Split(strings.TrimSpace(value), ".")
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
