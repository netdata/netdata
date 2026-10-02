// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

// The unchanged minute check is O(devices), independent of retained raw evidence.
// ns/op is a workstation trend indicator; allocation scaling is the useful gate.
func BenchmarkTopologyUnchangedRefresh(b *testing.B) {
	for name, count := range map[string]int{"100": 100, "1000": 1000, "10000": 10000} {
		b.Run(name, func(b *testing.B) {
			c, store := newTestSNMPTopologyCollectorWithStore()
			handler := snmpmock.NewMockHandler(gomock.NewController(b))
			handler.EXPECT().SetTarget(gomock.Any()).AnyTimes()
			handler.EXPECT().SetPort(gomock.Any()).AnyTimes()
			handler.EXPECT().SetRetries(gomock.Any()).AnyTimes()
			handler.EXPECT().SetTimeout(gomock.Any()).AnyTimes()
			handler.EXPECT().SetMaxOids(gomock.Any()).AnyTimes()
			handler.EXPECT().SetMaxRepetitions(gomock.Any()).AnyTimes()
			handler.EXPECT().SetCommunity(gomock.Any()).AnyTimes()
			handler.EXPECT().SetVersion(gomock.Any()).AnyTimes()
			handler.EXPECT().Connect().Return(nil).AnyTimes()
			handler.EXPECT().Close().Return(nil).AnyTimes()
			c.newSnmpClient = func() gosnmp.Handler { return handler }

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
