// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
)

const (
	endpointKeyHexChars = 32
	resourceKeyHexChars = 32
	digestHexChars      = sha256.Size * 2
)

func stableKey(domain string, value string, hexChars int) string {
	digest := sha256.Sum256([]byte(domain + "\x00" + value))
	encoded := hex.EncodeToString(digest[:])
	if hexChars <= 0 || hexChars >= len(encoded) {
		return encoded
	}
	return encoded[:hexChars]
}

func structuralTuple(parts ...string) string {
	var encoded []byte
	for _, part := range parts {
		encoded = binary.AppendUvarint(encoded, uint64(len(part)))
		encoded = append(encoded, part...)
	}
	return string(encoded)
}

func stableTupleDigest(domain string, parts ...string) string {
	hash := sha256.New()
	writePart := func(part string) {
		var length [binary.MaxVarintLen64]byte
		count := binary.PutUvarint(length[:], uint64(len(part)))
		_, _ = hash.Write(length[:count])
		_, _ = hash.Write([]byte(part))
	}
	writePart(domain)
	for _, part := range parts {
		writePart(part)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func resourceKey(origin, kind, locator string) string {
	return stableKey(
		"netdata:redfish:resource:v1",
		structuralTuple(origin, kind, locator),
		resourceKeyHexChars,
	)
}
