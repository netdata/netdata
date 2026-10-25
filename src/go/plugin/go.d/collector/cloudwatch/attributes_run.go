// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"fmt"
	"time"

	"github.com/sourcegraph/conc/pool"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/awsauth"
)

// attributeBatch transfers one immutable request and its results between Collect
// and Run. Collect permits only one outstanding batch, so neither channel grows
// with collection ticks while AWS is slow.
type attributeBatch struct {
	fetches []attributeFetch
	results []attributeFetchResult
}

type attributeClients struct {
	rds     *clientCache[rdsClient]
	timeout time.Duration
}

func (c *Collector) resetAttributeWorker() {
	c.attributeRequests = make(chan attributeBatch, 1)
	c.attributeResults = make(chan attributeBatch, 1)
	c.attributePending = false
	c.attributeRefreshAt = time.Time{}
}

// Run acquires optional resource metadata independently of numeric collection.
// Readiness requires no AWS request. The framework joins Run before Cleanup.
func (c *Collector) Run(ctx context.Context, ready func()) error {
	if c.plan == nil {
		return fmt.Errorf("CloudWatch collection plan has not been initialized")
	}
	identities := make(map[string]awsauth.Identity, len(c.plan.Targets))
	for _, target := range c.plan.Targets {
		identities[target.Name] = target.Identity
	}
	newConfig, newRDS := c.newAWSConfig, c.newRDSClient
	clients := &attributeClients{
		timeout: c.Timeout.Duration(),
		rds: newClientCache(func(ctx context.Context, target, region string) (rdsClient, error) {
			identity, ok := identities[target]
			if !ok {
				return nil, fmt.Errorf("unknown CloudWatch target %q", target)
			}
			cfg, err := newConfig(ctx, identity, region)
			if err != nil {
				return nil, err
			}
			return newRDS(cfg), nil
		}),
	}
	requests, results := c.attributeRequests, c.attributeResults
	ready()
	for {
		select {
		case <-ctx.Done():
			return nil
		case batch := <-requests:
			batch.results = clients.fetch(ctx, batch.fetches)
			if ctx.Err() != nil {
				return nil
			}
			select {
			case <-ctx.Done():
				return nil
			case results <- batch:
			}
		}
	}
}

func (c *attributeClients) fetch(ctx context.Context, fetches []attributeFetch) []attributeFetchResult {
	results := make([]attributeFetchResult, len(fetches))
	p := pool.New().WithMaxGoroutines(apiConcurrency)
	for i := range fetches {
		p.Go(func() {
			labels, err := fetches[i].provider.fetch(ctx, c, fetches[i].key.target, fetches[i].key.region)
			results[i] = attributeFetchResult{
				labels: labels,
				err:    err,
			}
		})
	}
	p.Wait()
	return results
}
