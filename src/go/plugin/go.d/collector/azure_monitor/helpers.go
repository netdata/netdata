// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"strings"
	"time"
)

const profileNamePattern = "^[a-z][a-z0-9_]*$"

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

func withOptionalTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

func isValidProfileName(v string) bool {
	v = stringsTrim(v)
	if len(v) == 0 {
		return false
	}
	if v[0] < 'a' || v[0] > 'z' {
		return false
	}
	for i := 1; i < len(v); i++ {
		c := v[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' {
			continue
		}
		return false
	}
	return true
}
