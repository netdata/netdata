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
// sts:GetCallerIdentity, once. account_id is part of every series' identity, so at
// least one account MUST resolve. Individual identities that fail are skipped with a
// warning (fail-soft, mirroring per-region discovery); only when none resolve does
// Check fail. Two identities that resolve to the same account id are de-duplicated
// (first kept) — with a shared region list they would otherwise collide on the
// {account_id, region, dimensions} series identity.
func (c *Collector) ensureAccounts(ctx context.Context) error {
	if len(c.accounts) > 0 {
		return nil
	}

	region := c.regions()[0] // validated non-empty in Init
	identities := c.Auth.Identities()

	seenAccount := make(map[string]string, len(identities)) // account id -> first identity ref
	var accounts []cwAccount
	var lastErr error

	for _, id := range identities {
		acctID, err := c.resolveAccountID(ctx, id, region)
		if err != nil {
			lastErr = err
			// A single-identity failure is reported once via the returned error below;
			// only warn per-identity when there is more than one to disambiguate.
			if len(identities) > 1 {
				c.Warningf("CloudWatch: skipping identity %q: %v", id.Ref, err)
			}
			continue
		}
		if firstRef, dup := seenAccount[acctID]; dup {
			c.Warningf("CloudWatch: identity %q resolves to account %s already monitored via %q; skipping the duplicate", id.Ref, acctID, firstRef)
			continue
		}
		seenAccount[acctID] = id.Ref
		accounts = append(accounts, cwAccount{identity: id, accountID: acctID})
	}

	if len(accounts) == 0 {
		if lastErr != nil {
			return fmt.Errorf("no AWS account could be resolved (the 'sts:GetCallerIdentity' permission is required): %w", lastErr)
		}
		return errors.New("no AWS accounts resolved")
	}

	c.accounts = accounts
	c.Infof("CloudWatch: monitoring %d AWS account(s): %s", len(accounts), strings.Join(c.accountIDs(), ", "))
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
