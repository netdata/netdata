// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/sourcegraph/conc/pool"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/awsauth"
)

// resolvedTarget binds one configured target identity to its STS-resolved account.
// Target reference remains the execution identity; accountID is output attribution.
type resolvedTarget struct {
	target    *collectionTarget
	accountID string
}

type targetResolution struct {
	target    *collectionTarget
	accountID string
	err       error
}

// ensureTargets resolves the AWS account id for every configured target via
// sts:GetCallerIdentity. account_id is part of every series' identity, so at least
// one target MUST resolve; if none do, Check fails. Resolution is fail-soft AND
// retried: a target whose config-build or STS/AssumeRole call fails stays pending
// and is retried on the next cycle (rate-limited warning), so a transient failure
// does not silently drop a target for the lifetime of the job. Targets are never
// deduplicated by account id because credentials can expose different resources.
func (c *Collector) ensureTargets(ctx context.Context) error {
	if c.plan == nil {
		return errors.New("CloudWatch collection plan is not compiled")
	}
	if c.resolvedByRef == nil {
		c.resolvedByRef = make(map[string]resolvedTarget, len(c.plan.Targets))
	}

	// Short-circuit once every configured target has resolved. Until then, retry
	// the pending ones.
	if len(c.resolvedByRef) == len(c.plan.Targets) {
		if len(c.resolvedByRef) == 0 {
			return errors.New("no AWS targets resolved")
		}
		return nil
	}

	var pending []*collectionTarget
	for _, target := range c.plan.Targets {
		if _, ok := c.resolvedByRef[target.Name]; ok {
			continue // already resolved
		}
		pending = append(pending, target)
	}

	p := pool.NewWithResults[targetResolution]().WithMaxGoroutines(maxTargets)
	for _, target := range pending {
		p.Go(func() targetResolution {
			accountID, err := c.resolveAccountID(ctx, target.Identity, target.Regions[0])
			return targetResolution{target: target, accountID: accountID, err: err}
		})
	}
	results := p.Wait()

	var failures []operationFailure
	for _, result := range results {
		target := result.target
		if result.err != nil {
			failures = append(failures, operationFailure{Target: target.Name, Region: target.Regions[0], Err: result.err})
			continue
		}
		resolved := resolvedTarget{target: target, accountID: result.accountID}
		c.resolvedByRef[target.Name] = resolved
		c.invalidateQueryPlan()
		c.markDiscoveryStale() // discover the newly-resolved account this cycle, not after refresh_every
		c.markTagsStale()      // and fetch its tags this cycle, so its first charts are created tagged
		c.Infof("CloudWatch: target %q resolved successfully", target.Name)
	}
	c.warnOperationFailures(logKeyAccountResolveFailed, "account resolution", " (will retry next cycle)", failures)

	if len(c.resolvedByRef) == 0 {
		if len(failures) > 0 {
			last := failures[len(failures)-1]
			return fmt.Errorf("no AWS target could be resolved (%d failed); last failure for target %q region %q: %w",
				len(failures), last.Target, last.Region, sanitizeAWSError(last.Err))
		}
		return errors.New("no AWS targets resolved")
	}
	return nil
}

// resolveAccountID builds a config for one identity and resolves its account id.
func (c *Collector) resolveAccountID(ctx context.Context, id awsauth.Identity, region string) (string, error) {
	cctx, cancel := withTimeout(ctx, c.Timeout.Duration())
	defer cancel()

	cfg, err := c.newAWSConfig(cctx, id, region)
	if err != nil {
		return "", fmt.Errorf("build AWS configuration: %w", sanitizeAWSError(err))
	}

	out, err := c.newSTSClient(cfg).GetCallerIdentity(cctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", fmt.Errorf("GetCallerIdentity: %w", sanitizeAWSError(err))
	}
	if out.Account == nil || *out.Account == "" {
		return "", errors.New("GetCallerIdentity returned no account ID")
	}
	return *out.Account, nil
}

func (c *Collector) resolvedTargetByRef(ref string) (resolvedTarget, bool) {
	target, ok := c.resolvedByRef[ref]
	return target, ok
}
