// SPDX-License-Identifier: GPL-3.0-or-later

package beacon

import (
	"crypto/sha1"
	"encoding/hex"
	"regexp"
	"strings"
)

// fingerprintLen is the number of hex characters kept from the sha1 sum
// ("first 12 hex").
const fingerprintLen = 12

var (
	reQuotedDouble = regexp.MustCompile(`"[^"]*"`)
	reQuotedSingle = regexp.MustCompile(`'[^']*'`)
	reURL          = regexp.MustCompile(`(?i)https?://\S+`)
	reHex          = regexp.MustCompile(`\b[0-9a-fA-F]{6,}\b`)
	reDigits       = regexp.MustCompile(`\b\d+\b`)
	reSpace        = regexp.MustCompile(`\s+`)
)

// NormalizeMessage collapses volatile substrings in an error message so
// otherwise-identical errors fingerprint the same: quoted
// strings and URLs are collapsed first (they may themselves contain
// digits/hex that would otherwise leak through), then remaining hex
// tokens (>=6 chars) and decimal runs become "#", then whitespace is
// collapsed.
func NormalizeMessage(s string) string {
	s = reQuotedDouble.ReplaceAllString(s, `"…"`)
	s = reQuotedSingle.ReplaceAllString(s, `"…"`)
	s = reURL.ReplaceAllString(s, "URL")
	s = reHex.ReplaceAllString(s, "#")
	s = reDigits.ReplaceAllString(s, "#")
	s = reSpace.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// Fingerprint identifies an error group: first 12 hex of
// sha1(type + "\n" + normalized message + "\n" + firstFrame). firstFrame
// is the caller's already-line/column-stripped first stack frame (empty
// when the beacon carried no structured frames).
func Fingerprint(typ, message, firstFrame string) string {
	h := sha1.New()
	h.Write([]byte(typ))
	h.Write([]byte("\n"))
	h.Write([]byte(NormalizeMessage(message)))
	h.Write([]byte("\n"))
	h.Write([]byte(firstFrame))
	sum := hex.EncodeToString(h.Sum(nil))
	return sum[:fingerprintLen]
}
