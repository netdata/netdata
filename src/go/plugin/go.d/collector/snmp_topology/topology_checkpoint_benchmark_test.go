// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
	snmpmock "github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils/snmpmock"
	"go.uber.org/mock/gomock"
)

// The unchanged minute check is O(devices), independent of retained raw evidence.
// ns/op is a workstation trend indicator; allocation scaling is the useful gate.
func BenchmarkTopologyUnchangedRefresh(b *testing.B) {
	for name, count := range map[string]int{"100": 100, "1000": 1000, "10000": 10000} {
		b.Run(name, func(b *testing.B) {
			c, store := newTestSNMPTopologyCollectorWithStore()
			handler := snmpmock.NewMockSession(gomock.NewController(b))
			handler.EXPECT().Connect().Return(nil).AnyTimes()
			handler.EXPECT().Close().Return(nil).AnyTimes()
			c.newSnmpClient = func(gosnmp.Handler) snmputils.Session { return handler }

			c.topologyProfiles = func(ddsnmp.DeviceConnectionInfo) []*ddsnmp.Profile { return nil }
			now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
			c.now = func() time.Time { return now }
			for i := 0; i < count; i++ {
				store.Register(fmt.Sprintf("job-%d", i), ddsnmp.DeviceConnectionInfo{Hostname: fmt.Sprintf("192.0.%d.%d", i/256, i%256)})
			}
			c.refreshTopology(context.Background())
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				c.refreshTopology(context.Background())
			}
		})
	}
}
