// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"crypto/sha1"
	"encoding/hex"
	"strings"
)

func stringsLowerTrim(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}

func stringsTrim(v string) string {
	return strings.TrimSpace(v)
}

func hashShort(v string) string {
	sum := sha1.Sum([]byte(strings.ToLower(strings.TrimSpace(v))))
	return hex.EncodeToString(sum[:6])
}
