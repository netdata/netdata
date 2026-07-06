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

// clientKey identifies a client by the account it authenticates as and the region
// it targets. Multi-account jobs need a distinct client per (account, region): the
// same region is queried under each account's own credentials.
type clientKey struct {
	account string
	region  string
}

// clientCache builds and caches one client of type T per (account, region). It is
// generic so the CloudWatch and Resource Groups Tagging API clients (identical
// per-(account, region) lifecycle, different API surface) share one implementation.
// forAccountRegion is safe for concurrent use, so callers need not pre-resolve
// clients to avoid racing a shared cache. A client is built at most once per
// (account, region); only successes are cached, so a transient credential error is
// retried next call.
type clientCache[T any] struct {
	mu      sync.Mutex
	clients map[clientKey]T
	build   func(ctx context.Context, account, region string) (T, error)
}

func newClientCache[T any](build func(ctx context.Context, account, region string) (T, error)) *clientCache[T] {
	return &clientCache[T]{
		clients: make(map[clientKey]T),
		build:   build,
	}
}

func (cc *clientCache[T]) forAccountRegion(ctx context.Context, account, region string) (T, error) {
	key := clientKey{account: account, region: region}
	cc.mu.Lock()
	defer cc.mu.Unlock()
	if client, ok := cc.clients[key]; ok {
		return client, nil
	}
	client, err := cc.build(ctx, account, region)
	if err != nil {
		var zero T
		return zero, err
	}
	cc.clients[key] = client
	return client, nil
}

func (cc *clientCache[T]) reset() {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.clients = make(map[clientKey]T)
}

// buildAccountRegionClient constructs a CloudWatch client for one (account, region)
// using that account's resolved auth identity. It is the clientCache's builder; the
// cache memoizes its successful results. It reads c.newAWSConfig/c.newCloudWatchClient
// at call time so test seams set after New() are honored.
func (c *Collector) buildAccountRegionClient(ctx context.Context, account, region string) (cloudwatchClient, error) {
	id, ok := c.identityForAccount(account)
	if !ok {
		return nil, fmt.Errorf("no resolved identity for account %q", account)
	}
	cfg, err := c.newAWSConfig(ctx, id, region)
	if err != nil {
		return nil, err
	}
	return c.newCloudWatchClient(cfg), nil
}

// buildAccountRegionRGTAClient constructs a Resource Groups Tagging API client for
// one (account, region), mirroring buildAccountRegionClient. RGTA is regional, so
// the tag lookup for a series uses the client for that series' (account, region).
func (c *Collector) buildAccountRegionRGTAClient(ctx context.Context, account, region string) (rgtaClient, error) {
	id, ok := c.identityForAccount(account)
	if !ok {
		return nil, fmt.Errorf("no resolved identity for account %q", account)
	}
	cfg, err := c.newAWSConfig(ctx, id, region)
	if err != nil {
		return nil, err
	}
	return c.newRGTAClient(cfg), nil
}
