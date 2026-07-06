// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
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

func defaultNewCloudWatchClient(cfg aws.Config) cloudwatchClient {
	return cloudwatch.NewFromConfig(cfg)
}

func defaultNewSTSClient(cfg aws.Config) stsClient {
	return sts.NewFromConfig(cfg)
}

func defaultNewAWSConfig(ctx context.Context, auth awsauth.AWSAuthConfig, region string) (aws.Config, error) {
	return auth.NewConfig(ctx, awsauth.AWSConfigOptions{Region: region})
}

// clientCache builds and caches one CloudWatch client per region. forRegion is
// safe for concurrent use, so callers need not pre-resolve clients to avoid
// racing a shared cache. A client is built at most once per region; only
// successes are cached, so a transient credential error is retried next call.
type clientCache struct {
	mu      sync.Mutex
	clients map[string]cloudwatchClient
	build   func(ctx context.Context, region string) (cloudwatchClient, error)
}

func newClientCache(build func(ctx context.Context, region string) (cloudwatchClient, error)) *clientCache {
	return &clientCache{
		clients: make(map[string]cloudwatchClient),
		build:   build,
	}
}

func (cc *clientCache) forRegion(ctx context.Context, region string) (cloudwatchClient, error) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	if client, ok := cc.clients[region]; ok {
		return client, nil
	}
	client, err := cc.build(ctx, region)
	if err != nil {
		return nil, err
	}
	cc.clients[region] = client
	return client, nil
}

func (cc *clientCache) reset() {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.clients = make(map[string]cloudwatchClient)
}

// buildRegionClient constructs a CloudWatch client for one region from the
// configured auth. It is the clientCache's builder; the cache memoizes its
// successful results. It reads c.newAWSConfig/c.newCloudWatchClient at call time
// so test seams set after New() are honored.
func (c *Collector) buildRegionClient(ctx context.Context, region string) (cloudwatchClient, error) {
	cfg, err := c.newAWSConfig(ctx, c.Auth, region)
	if err != nil {
		return nil, err
	}
	return c.newCloudWatchClient(cfg), nil
}
