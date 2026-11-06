// SPDX-License-Identifier: GPL-3.0-or-later

package x509check

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// BenchmarkCollector_Collect measures one full-chain collection cycle, including the metric-store commit.
func BenchmarkCollector_Collect(b *testing.B) {
	c := New()
	c.CheckFullChain = true
	c.provider = &mockProvider{
		certs: []*x509.Certificate{
			{Subject: pkix.Name{
				CommonName: "leaf",
			}, NotAfter: time.Now().Add(time.Hour)},
			{Subject: pkix.Name{
				CommonName: "intermediate",
			}, NotAfter: time.Now().Add(24 * time.Hour)},
			{Subject: pkix.Name{
				CommonName: "root",
			}, NotAfter: time.Now().Add(48 * time.Hour)},
		},
	}

	b.ReportAllocs()
	for b.Loop() {
		require.NoError(b, collectCycle(b, c))
	}
}
