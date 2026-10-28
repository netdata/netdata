// SPDX-License-Identifier: GPL-3.0-or-later

package netdataapi

import "strings"

// Match the Agent's external-plugin backslash normalization. An unescaped
// trailing backslash makes the plugins.d splitter consume the closing quote.
var wireValueReplacer = strings.NewReplacer(
	"\\", "/",
	"'", "",
	"\n", " ",
	"\r", " ",
	"\x00", "",
)

// SanitizeWireValue returns the published value of a quoted plugin-protocol
// field. Admission keys and emission must share this normalization. It does not
// trim whitespace; callers that require trimmed identifiers do that separately.
// API serialization remains literal: callers opt into this transformation.
func SanitizeWireValue(value string) string {
	return wireValueReplacer.Replace(value)
}
