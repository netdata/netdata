// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/awsauth"
)

// cloudwatchClient is the narrow CloudWatch API surface the collector uses.
// It is an interface so tests can substitute a fake (the mock seam).
type cloudwatchClient interface {
	ListMetrics(ctx context.Context, in *cloudwatch.ListMetricsInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.ListMetricsOutput, error)
	GetMetricData(ctx context.Context, in *cloudwatch.GetMetricDataInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error)
}

// stsClient is the narrow STS API surface used for account-identity bootstrap.
type stsClient interface {
	GetCallerIdentity(ctx context.Context, in *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
}

// rgtaClient is the narrow Resource Groups Tagging API surface used for tag
// enrichment (Phase 2b). GetResources is the only call: it lists tagged resources
// (ARN + tags) per region so the collector can join tags onto discovered series.
type rgtaClient interface {
	GetResources(ctx context.Context, in *resourcegroupstaggingapi.GetResourcesInput, optFns ...func(*resourcegroupstaggingapi.Options)) (*resourcegroupstaggingapi.GetResourcesOutput, error)
}

func defaultNewCloudWatchClient(cfg aws.Config) cloudwatchClient {
	return cloudwatch.NewFromConfig(cfg)
}

func defaultNewSTSClient(cfg aws.Config) stsClient {
	return sts.NewFromConfig(cfg)
}

func defaultNewRGTAClient(cfg aws.Config) rgtaClient {
	return resourcegroupstaggingapi.NewFromConfig(cfg)
}

func defaultNewAWSConfig(ctx context.Context, id awsauth.Identity, region string) (aws.Config, error) {
	return id.NewConfig(ctx, awsauth.ConfigOptions{Region: region})
}

// clientKey identifies a client by configured target and region. Account id is not
// a credential identity: two targets can resolve to one account with different
// permissions and must retain separate clients.
type clientKey struct {
	target string
	region string
}

// clientCache builds and caches one client of type T per (target, region). It is
// generic so the CloudWatch and Resource Groups Tagging API clients (identical
// per-(target, region) lifecycle, different API surface) share one implementation.
// forTargetRegion is safe for concurrent use, so callers need not pre-resolve
// clients to avoid racing a shared cache. A client is built at most once per
// (target, region); only successes are cached, so a transient credential error is
// retried next call.
type clientCache[T any] struct {
	mu       sync.Mutex
	clients  map[clientKey]T
	building map[clientKey]*clientBuild[T]
	build    func(ctx context.Context, target, region string) (T, error)
}

type clientBuild[T any] struct {
	ready  chan struct{}
	client T
	err    error
}

func newClientCache[T any](build func(ctx context.Context, target, region string) (T, error)) *clientCache[T] {
	return &clientCache[T]{
		clients:  make(map[clientKey]T),
		building: make(map[clientKey]*clientBuild[T]),
		build:    build,
	}
}

func (cc *clientCache[T]) forTargetRegion(ctx context.Context, target, region string) (T, error) {
	key := clientKey{target: target, region: region}
	cc.mu.Lock()
	if client, ok := cc.clients[key]; ok {
		cc.mu.Unlock()
		return client, nil
	}
	if pending, ok := cc.building[key]; ok {
		cc.mu.Unlock()
		select {
		case <-pending.ready:
			return pending.client, pending.err
		case <-ctx.Done():
			var zero T
			return zero, ctx.Err()
		}
	}
	pending := &clientBuild[T]{ready: make(chan struct{})}
	cc.building[key] = pending
	cc.mu.Unlock()

	pending.client, pending.err = cc.build(ctx, target, region)

	cc.mu.Lock()
	delete(cc.building, key)
	if pending.err == nil {
		cc.clients[key] = pending.client
	}
	close(pending.ready)
	cc.mu.Unlock()
	return pending.client, pending.err
}

func (cc *clientCache[T]) reset() {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.clients = make(map[clientKey]T)
	cc.building = make(map[clientKey]*clientBuild[T])
}

// buildTargetRegionClient constructs a CloudWatch client for one (target, region)
// using that target's compiled auth identity. It is the clientCache's builder; the
// cache memoizes its successful results. It reads c.newAWSConfig/c.newCloudWatchClient
// at call time so test seams set after New() are honored.
func (c *Collector) buildTargetRegionClient(ctx context.Context, target, region string) (cloudwatchClient, error) {
	resolved, ok := c.resolvedTargetByRef(target)
	if !ok {
		return nil, fmt.Errorf("target %q is not resolved", target)
	}
	cfg, err := c.newAWSConfig(ctx, resolved.target.Identity, region)
	if err != nil {
		return nil, err
	}
	return c.newCloudWatchClient(cfg), nil
}

// buildTargetRegionRGTAClient mirrors buildTargetRegionClient for the Resource
// Groups Tagging API.
func (c *Collector) buildTargetRegionRGTAClient(ctx context.Context, target, region string) (rgtaClient, error) {
	resolved, ok := c.resolvedTargetByRef(target)
	if !ok {
		return nil, fmt.Errorf("target %q is not resolved", target)
	}
	cfg, err := c.newAWSConfig(ctx, resolved.target.Identity, region)
	if err != nil {
		return nil, err
	}
	return c.newRGTAClient(cfg), nil
}
