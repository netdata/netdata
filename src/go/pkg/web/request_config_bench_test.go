// SPDX-License-Identifier: GPL-3.0-or-later

package web

import (
	"context"
	"testing"
)

// The default path constructs only an HTTP request; it creates no credential
// reader or helper process. Work is O(request configuration bytes).
func BenchmarkNewHTTPRequestNoCredentials(b *testing.B) {
	cfg := RequestConfig{URL: "http://localhost/metrics"}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := NewHTTPRequest(ctx, cfg); err != nil {
			b.Fatal(err)
		}
	}
}
