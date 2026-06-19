// SPDX-License-Identifier: GPL-3.0-or-later

package naming

import "strings"

var sanitizer = strings.NewReplacer(
	"/", "_",
	"\\", "_",
	" ", "_",
	":", "_",
	"*", "_",
	"?", "_",
	"\"", "_",
	"<", "_",
	">", "_",
	"|", "_",
)

// Sanitize returns a stable identifier-safe representation for names that can
// flow into IDs, paths, and type identifiers.
func Sanitize(name string) string {
	return sanitizer.Replace(name)
}
