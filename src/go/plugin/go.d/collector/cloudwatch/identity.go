// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/awsauth"
)

// resolvedTarget binds one configured target identity to its STS-resolved account.
// Target reference remains the execution identity; accountID is output attribution.
type resolvedTarget struct {
	target    *targetRuntime
	accountID string
}

// ensureTargets resolves the AWS account id for every configured target via
// sts:GetCallerIdentity. account_id is part of every series' identity, so at least
// one target MUST resolve; if none do, Check fails. Resolution is fail-soft AND
// retried: a target whose config-build or STS/AssumeRole call fails stays pending
// and is retried on the next cycle (rate-limited warning), so a transient failure
// does not silently drop a target for the lifetime of the job. Targets are never
// deduplicated by account id because credentials can expose different resources.
func (c *Collector) ensureTargets(ctx context.Context) error {
	if c.runtime == nil {
		return errors.New("CloudWatch collection plan is not compiled")
	}
	if c.resolvedRefs == nil {
		c.resolvedRefs = make(map[string]struct{}, len(c.runtime.Targets))
		c.resolvedByRef = make(map[string]resolvedTarget, len(c.runtime.Targets))
	}

	// Short-circuit once every configured target has resolved. Until then, retry
	// the pending ones.
	allResolved := true
	for _, target := range c.runtime.Targets {
		if _, ok := c.resolvedRefs[target.Name]; !ok {
			allResolved = false
			break
		}
	}
	if allResolved {
		if len(c.resolvedTargets) == 0 {
			return errors.New("no AWS targets resolved")
		}
		return nil
	}

	var lastErr error

	for _, target := range c.runtime.Targets {
		if _, ok := c.resolvedRefs[target.Name]; ok {
			continue // already resolved
		}
		region := target.Regions[0]
		acctID, err := c.resolveAccountID(ctx, target.Identity, region)
		if err != nil {
			lastErr = err
			// Keep the identity pending and retry next cycle; throttle the warning so a
			// persistently unreachable role does not warn every cycle.
			c.Limit(logKeyAccountResolveFailed+":"+target.Name, 1, recurringLogEvery).
				Warningf("CloudWatch: %v (will retry next cycle)", err)
			continue
		}
		c.resolvedRefs[target.Name] = struct{}{}
		resolved := resolvedTarget{target: target, accountID: acctID}
		c.resolvedTargets = append(c.resolvedTargets, resolved)
		c.resolvedByRef[target.Name] = resolved
		c.invalidateQueryPlan()
		c.markDiscoveryStale() // discover the newly-resolved account this cycle, not after refresh_every
		c.markTagsStale()      // and fetch its tags this cycle, so its first charts are created tagged
		c.Infof("CloudWatch: target %q resolved to AWS account %s", target.Name, acctID)
	}

	if len(c.resolvedTargets) == 0 {
		if lastErr != nil {
			return fmt.Errorf("no AWS account could be resolved (the 'sts:GetCallerIdentity' permission is required): %w", lastErr)
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
		return "", fmt.Errorf("identity %q (region %q): building AWS config: %w", id.Ref, region, err)
	}

	out, err := c.newSTSClient(cfg).GetCallerIdentity(cctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", fmt.Errorf("identity %q (region %q): sts:GetCallerIdentity: %w", id.Ref, region, err)
	}
	if out.Account == nil || *out.Account == "" {
		return "", fmt.Errorf("identity %q (region %q): sts:GetCallerIdentity returned no account id", id.Ref, region)
	}
	return *out.Account, nil
}

func (c *Collector) resolvedTargetByRef(ref string) (resolvedTarget, bool) {
	target, ok := c.resolvedByRef[ref]
	return target, ok
}

func (c *Collector) resolvedTargetRefs() []string {
	refs := make([]string, 0, len(c.resolvedTargets))
	for _, target := range c.resolvedTargets {
		refs = append(refs, target.target.Name)
	}
	return refs
}
