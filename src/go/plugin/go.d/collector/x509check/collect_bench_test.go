// SPDX-License-Identifier: GPL-3.0-or-later

package x509check

import (
	"context"
	"crypto/x509"
	"crypto/x509/pkix"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

func BenchmarkCollectorCollect(b *testing.B) {
	c := New()
	c.CheckFullChain = true
	c.prov = &mockProvider{certs: []*x509.Certificate{
		{Subject: pkix.Name{CommonName: "leaf"}, NotAfter: time.Now().Add(time.Hour)},
		{Subject: pkix.Name{CommonName: "intermediate"}, NotAfter: time.Now().Add(24 * time.Hour)},
		{Subject: pkix.Name{CommonName: "root"}, NotAfter: time.Now().Add(48 * time.Hour)},
	}}
	ctx := context.Background()
	if err := c.Check(ctx); err != nil {
		b.Fatal(err)
	}
	// The optional store keeps this benchmark usable with the V1 source overlay for migration measurements.
	var cc metrix.CycleController
	if v2, ok := any(c).(interface{ MetricStore() metrix.CollectorStore }); ok {
		store, ok := metrix.AsCycleManagedStore(v2.MetricStore())
		if !ok {
			b.Fatal("metric store is not cycle managed")
		}
		cc = store.CycleController()
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if cc != nil {
			cc.BeginCycle()
		}
		c.Collect(ctx)
		if cc != nil {
			if err := cc.CommitCycleSuccess(); err != nil {
				b.Fatal(err)
			}
		}
	}
}
