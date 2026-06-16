// SPDX-License-Identifier: GPL-3.0-or-later

package secretresolver

import "strings"

// Output modifiers post-process a resolved secret value. A reference may carry
// one modifier appended to its scheme token with '+', for example
// "${store+urienc:vault:prod:secret#key}" or "${env+urienc:DB_PASS}". '+' cannot
// appear in a scheme name, so the modifier never collides with operand content.
const (
	// modifierURIEncode percent-encodes the resolved value so it is safe to embed
	// in any URI component (a DSN, a connection string). It is opt-in; without a
	// modifier the raw value is returned unchanged.
	modifierURIEncode = "urienc"
)

// SplitSchemeModifier splits a secret-reference scheme token into its base scheme
// and an optional output modifier joined by '+'. A token without '+' yields an
// empty modifier. Example: "store+urienc" -> ("store", "urienc"), "env" -> ("env", "").
func SplitSchemeModifier(token string) (scheme, modifier string) {
	base, mod, ok := strings.Cut(token, "+")
	if !ok {
		return token, ""
	}
	return base, mod
}

func isKnownModifier(modifier string) bool {
	switch modifier {
	case "", modifierURIEncode:
		return true
	default:
		return false
	}
}

// applyModifier post-processes a resolved value according to modifier. Callers
// MUST validate the modifier with isKnownModifier first; an unrecognized modifier
// returns the value unchanged.
func applyModifier(modifier, value string) string {
	if modifier == modifierURIEncode {
		return percentEncodeURIUnreserved(value)
	}
	return value
}

// percentEncodeURIUnreserved percent-encodes every byte that is not an RFC 3986
// unreserved character (ALPHA / DIGIT / "-" / "." / "_" / "~"). Encoding everything
// outside the unreserved set keeps the result valid in any URI component (userinfo,
// path, query), which is what a secret embedded in a DSN needs. Go's url.QueryEscape
// (space -> '+') and url.PathEscape (leaves '@', ':' and sub-delims) are both unsafe
// for a password in URI userinfo, so the safe set is applied directly here.
func percentEncodeURIUnreserved(value string) string {
	const upperhex = "0123456789ABCDEF"

	var b strings.Builder
	b.Grow(len(value))
	for i := 0; i < len(value); i++ {
		c := value[i]
		if isURIUnreserved(c) {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(upperhex[c>>4])
		b.WriteByte(upperhex[c&0x0F])
	}
	return b.String()
}

func isURIUnreserved(c byte) bool {
	return (c >= 'A' && c <= 'Z') ||
		(c >= 'a' && c <= 'z') ||
		(c >= '0' && c <= '9') ||
		c == '-' || c == '.' || c == '_' || c == '~'
}
