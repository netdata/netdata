// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
)

// A cached interval should check only cadence/result availability, independent of
// the number of discovered target groups, and allocate no per-group scratch maps.
func BenchmarkRefreshAttributesCached(b *testing.B) {
	for _, groups := range []int{1, 64} {
		b.Run(fmt.Sprintf("groups=%d", groups), func(b *testing.B) {
			names := make([]string, groups)
			for i := range names {
				names[i] = fmt.Sprintf("target-%d", i)
			}
			c := New()
			c.Config = rdsProfileConfig(names...)
			c.newCatalog = cwprofiles.DefaultCatalog
			c.now = func() time.Time { return time.Unix(1_000_000_000, 0) }
			useFakeClient(
				c,
				&fullCloudWatch{
					list:     map[string][]cwtypes.Metric{"AWS/RDS": rdsMetrics("db-a")},
					gmdValue: 1,
				},
			)
			c.newSTSClient = func(aws.Config) stsClient {
				return &fakeSTS{
					account: "000000000000",
				}
			}
			c.newRDSClient = func(aws.Config) rdsClient {
				return &fakeRDS{
					instances: []rdstypes.DBInstance{rdsInstance("db-a", "mysql", "8", "db.t3.micro")},
				}
			}
			if err := c.Init(context.Background()); err != nil {
				b.Fatal(err)
			}
			if err := c.Check(context.Background()); err != nil {
				b.Fatal(err)
			}
			// Also builds on the synchronous implementation for before/after comparison.
			if runner, ok := any(c).(collectorapi.CollectorV2Runner); ok {
				ctx, cancel := context.WithCancel(context.Background())
				ready, done := make(chan struct{}), make(chan error, 1)
				go func() { done <- runner.Run(ctx, func() { close(ready) }) }()
				<-ready
				b.Cleanup(func() {
					cancel()
					if err := <-done; err != nil {
						b.Error(err)
					}
				})
			}
			deadline := time.Now().Add(5 * time.Second)
			for {
				if _, err := collecttest.CollectScalarSeries(c); err != nil {
					b.Fatal(err)
				}
				if len(c.attributes) == groups {
					break
				}
				if time.Now().After(deadline) {
					b.Fatal("metadata not ready")
				}
				time.Sleep(time.Millisecond)
			}
			ctx := context.Background()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				c.refreshAttributes(ctx)
			}
		})
	}
}
