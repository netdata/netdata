// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// ensureAccountIdentity resolves the AWS account id once via sts:GetCallerIdentity.
// account_id is part of every series' instance identity, so this is a hard
// requirement; a denied call fails Check with a clear, actionable message.
func (c *Collector) ensureAccountIdentity(ctx context.Context) error {
	if c.accountID != "" {
		return nil
	}

	region := c.regions()[0] // validated non-empty in Init

	cctx, cancel := withTimeout(ctx, c.Timeout.Duration())
	defer cancel()

	cfg, err := c.newAWSConfig(cctx, c.Auth, region)
	if err != nil {
		return fmt.Errorf("building AWS config for region %q: %w", region, err)
	}

	out, err := c.newSTSClient(cfg).GetCallerIdentity(cctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return fmt.Errorf("sts:GetCallerIdentity failed (the 'sts:GetCallerIdentity' permission is required): %w", err)
	}
	if out.Account == nil || *out.Account == "" {
		return errors.New("sts:GetCallerIdentity returned no account id")
	}

	c.accountID = *out.Account
	c.Infof("CloudWatch: using AWS account %s (resolved via sts:GetCallerIdentity)", c.accountID)

	return nil
}
