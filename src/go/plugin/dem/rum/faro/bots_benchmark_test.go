// SPDX-License-Identifier: GPL-3.0-or-later

package faro

import (
	"testing"
)

// The receiver classifies the User-Agent of every beacon. A browser is the
// common and most expensive case because no entry ends the scan early. Each
// call allocates at most the lower-cased copy; timings are local trends, not
// CI gates.
func BenchmarkIsBot(b *testing.B) {
	for name, ua := range map[string]string{
		"browser":   "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
		"short":     "Mozilla/5.0 Chrome/131.0.0.0 Safari/537.36",
		"mobile":    "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
		"crawler":   "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
		"tool":      "curl/8.4.0",
		"non-ascii": "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36 Édition",
	} {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				IsBot(ua)
			}
		})
	}
}
