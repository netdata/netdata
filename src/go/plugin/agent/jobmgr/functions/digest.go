// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import "encoding/binary"

type digestWriter interface {
	Write([]byte) (int, error)
}

func writeDigestString(writer digestWriter, value string) {
	writeDigestUint64(writer, uint64(len(value)))
	_, _ = writer.Write([]byte(value))
}

func writeDigestUint64(writer digestWriter, value uint64) {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], value)
	_, _ = writer.Write(encoded[:])
}

func writeDigestBool(writer digestWriter, value bool) {
	var encoded [1]byte
	if value {
		encoded[0] = 1
	}
	_, _ = writer.Write(encoded[:])
}
