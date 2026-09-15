// SPDX-License-Identifier: GPL-3.0-or-later

package web

import (
	"context"
	"net/http"
	"testing"
)

type benchmarkTokenReader struct {
	token []byte
}

func (r benchmarkTokenReader) Read(context.Context, string) ([]byte, error) { return r.token, nil }

// Measures request construction separately from filesystem/IPC cost. Binding
// is once per client; per-request work remains O(config bytes + token bytes).
func BenchmarkHTTPClientNewRequest(b *testing.B) {
	files := benchmarkTokenReader{token: []byte("synthetic-token")}
	client := WrapHTTPClient(&http.Client{}, files)
	cfg := RequestConfig{URL: "http://localhost/metrics", BearerTokenFile: "synthetic-path"}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := client.NewRequest(ctx, cfg); err != nil {
			b.Fatal(err)
		}
	}
}
