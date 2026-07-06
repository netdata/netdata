// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/awsauth"
)

// cwAccount is one resolved AWS account the collector monitors: the auth identity
// used to build its clients, and its account id (resolved via sts:GetCallerIdentity).
type cwAccount struct {
	identity  awsauth.Identity
	accountID string
}

// ensureAccounts resolves the AWS account id for every configured identity via
// sts:GetCallerIdentity. account_id is part of every series' identity, so at least
// one account MUST resolve; if none do, Check fails. Resolution is fail-soft AND
// retried: an identity whose config-build or STS/AssumeRole call fails stays pending
// and is retried on the next cycle (rate-limited warning), so a transient failure
// does not silently drop an account for the lifetime of the job. Two identities that
// resolve to the same account id are de-duplicated (first kept) — with a shared
// region list they would otherwise collide on the {account_id, region, dimensions}
// series identity.
func (c *Collector) ensureAccounts(ctx context.Context) error {
	identities := c.Auth.Identities()
	if c.resolvedRefs == nil {
		c.resolvedRefs = make(map[string]struct{}, len(identities))
		c.seenAccountID = make(map[string]string, len(identities))
	}

	// Short-circuit once every configured identity has resolved (to a kept account
	// or an intentionally-skipped duplicate). Until then, retry the pending ones.
	allResolved := true
	for _, id := range identities {
		if _, ok := c.resolvedRefs[id.Ref]; !ok {
			allResolved = false
			break
		}
	}
	if allResolved {
		if len(c.accounts) == 0 {
			return errors.New("no AWS accounts resolved")
		}
		return nil
	}

	region := c.regions()[0] // validated non-empty in Init
	var lastErr error

	for _, id := range identities {
		if _, ok := c.resolvedRefs[id.Ref]; ok {
			continue // already resolved (kept or deduped)
		}
		acctID, err := c.resolveAccountID(ctx, id, region)
		if err != nil {
			lastErr = err
			// Keep the identity pending and retry next cycle; throttle the warning so a
			// persistently unreachable role does not warn every cycle.
			c.Limit(logKeyAccountResolveFailed+":"+id.Ref, 1, recurringLogEvery).
				Warningf("CloudWatch: could not resolve account for identity %q (will retry): %v", id.Ref, err)
			continue
		}
		c.resolvedRefs[id.Ref] = struct{}{}
		if firstRef, dup := c.seenAccountID[acctID]; dup {
			c.Warningf("CloudWatch: identity %q resolves to account %s already monitored via %q; skipping the duplicate", id.Ref, acctID, firstRef)
			continue
		}
		c.seenAccountID[acctID] = id.Ref
		c.accounts = append(c.accounts, cwAccount{identity: id, accountID: acctID})
		c.Infof("CloudWatch: monitoring %d AWS account(s): %s", len(c.accounts), strings.Join(c.accountIDs(), ", "))
	}

	if len(c.accounts) == 0 {
		if lastErr != nil {
			return fmt.Errorf("no AWS account could be resolved (the 'sts:GetCallerIdentity' permission is required): %w", lastErr)
		}
		return errors.New("no AWS accounts resolved")
	}
	return nil
}

// resolveAccountID builds a config for one identity and resolves its account id.
func (c *Collector) resolveAccountID(ctx context.Context, id awsauth.Identity, region string) (string, error) {
	cctx, cancel := withTimeout(ctx, c.Timeout.Duration())
	defer cancel()

	cfg, err := c.newAWSConfig(cctx, id, region)
	if err != nil {
		return "", fmt.Errorf("building AWS config: %w", err)
	}

	out, err := c.newSTSClient(cfg).GetCallerIdentity(cctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", fmt.Errorf("sts:GetCallerIdentity: %w", err)
	}
	if out.Account == nil || *out.Account == "" {
		return "", errors.New("sts:GetCallerIdentity returned no account id")
	}
	return *out.Account, nil
}

// identityForAccount returns the auth identity for a resolved account id.
func (c *Collector) identityForAccount(account string) (awsauth.Identity, bool) {
	for _, a := range c.accounts {
		if a.accountID == account {
			return a.identity, true
		}
	}
	return awsauth.Identity{}, false
}

func (c *Collector) accountIDs() []string {
	ids := make([]string, 0, len(c.accounts))
	for _, a := range c.accounts {
		ids = append(ids, a.accountID)
	}
	return ids
}
