// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/awsauth"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seqSTS returns account ids in call order, so identity[i] (resolved in order by
// ensureAccounts) maps to accounts[i]. failAt[i] makes the i-th call error.
type seqSTS struct {
	accounts []string
	failAt   map[int]bool
	calls    int
}

func (f *seqSTS) GetCallerIdentity(context.Context, *sts.GetCallerIdentityInput, ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	i := f.calls
	f.calls++
	if f.failAt[i] {
		return nil, errors.New("sts denied")
	}
	acct := ""
	if i < len(f.accounts) {
		acct = f.accounts[i]
	}
	return &sts.GetCallerIdentityOutput{Account: aws.String(acct)}, nil
}

func assumeRoleCollector(t *testing.T, roles []awsauth.AWSAssumeRole, includeBase bool, sts stsClient) *Collector {
	t.Helper()
	c := New()
	c.Config = Config{
		Regions: []string{"us-east-1"},
		Auth: awsauth.AWSAuthConfig{
			Mode:           awsauth.AWSAuthModeAssumeRole,
			ModeAssumeRole: &awsauth.AWSModeAssumeRoleConfig{Roles: roles, IncludeBaseAccount: includeBase},
		},
	}
	c.newAWSConfig = func(context.Context, awsauth.Identity, string) (aws.Config, error) { return aws.Config{}, nil }
	c.newSTSClient = func(aws.Config) stsClient { return sts }
	require.NoError(t, c.Init(context.Background()))
	return c
}

func twoRoles() []awsauth.AWSAssumeRole {
	return []awsauth.AWSAssumeRole{
		{RoleARN: "arn:aws:iam::111111111111:role/netdata"},
		{RoleARN: "arn:aws:iam::222222222222:role/netdata"},
	}
}

func TestEnsureAccounts_MultiRole(t *testing.T) {
	c := assumeRoleCollector(t, twoRoles(), false, &seqSTS{accounts: []string{"111111111111", "222222222222"}})

	require.NoError(t, c.ensureAccounts(context.Background()))
	assert.Equal(t, []string{"111111111111", "222222222222"}, c.accountIDs(), "one distinct account per role")
}

func TestEnsureAccounts_DeduplicatesSameAccount(t *testing.T) {
	// Two roles that resolve to the same account id must collapse to one account —
	// they would otherwise collide on the {account_id, region, dimensions} identity.
	c := assumeRoleCollector(t, twoRoles(), false, &seqSTS{accounts: []string{"111111111111", "111111111111"}})

	require.NoError(t, c.ensureAccounts(context.Background()))
	assert.Equal(t, []string{"111111111111"}, c.accountIDs(), "duplicate account id is skipped")
}

func TestEnsureAccounts_FailureIsolation(t *testing.T) {
	// One role failing STS must not sink the others (fail-soft).
	c := assumeRoleCollector(t, twoRoles(), false, &seqSTS{
		accounts: []string{"", "222222222222"},
		failAt:   map[int]bool{0: true},
	})

	require.NoError(t, c.ensureAccounts(context.Background()))
	assert.Equal(t, []string{"222222222222"}, c.accountIDs(), "the healthy account is still monitored")
}

func TestEnsureAccounts_AllFailIsFatal(t *testing.T) {
	c := assumeRoleCollector(t, twoRoles(), false, &seqSTS{failAt: map[int]bool{0: true, 1: true}})

	err := c.ensureAccounts(context.Background())
	assert.Error(t, err, "no account resolving must fail Check")
	assert.Empty(t, c.accounts)
}

func TestEnsureAccounts_IncludeBaseAccount(t *testing.T) {
	// assume_role identities are [role, ..., base]; with include_base_account the base
	// identity resolves to its own account too.
	c := assumeRoleCollector(t, twoRoles(), true, &seqSTS{accounts: []string{"111111111111", "222222222222", "999999999999"}})

	require.NoError(t, c.ensureAccounts(context.Background()))
	assert.Equal(t, []string{"111111111111", "222222222222", "999999999999"}, c.accountIDs(),
		"the base identity's account is monitored alongside the roles")
}

// TestBuildQueryPlan_MultiAccount is the INV.2 analog for the account dimension:
// adding accounts only ADDS series (each stamped with its account_id); it never drops
// an instance. Each account's discovery is queried under its own account_id.
func TestBuildQueryPlan_MultiAccount(t *testing.T) {
	c := New()
	c.Config.Regions = []string{"us-east-1"}
	c.applyDefaults()
	c.accounts = []cwAccount{{accountID: "111111111111"}, {accountID: "222222222222"}}
	c.profiles = []cwprofiles.ResolvedProfile{{Name: "ec2", Config: ec2QueryProfile()}}
	c.discovery = discoverySnapshot{Instances: map[discoveryKey][]discoveredInstance{
		{Account: "111111111111", Profile: "ec2", Region: "us-east-1"}: {{DimensionValues: []string{"i-1"}}},
		{Account: "222222222222", Profile: "ec2", Region: "us-east-1"}: {{DimensionValues: []string{"i-2"}}},
	}}

	plan := c.buildQueryPlan()
	require.Len(t, plan, 6, "2 accounts x 1 instance x (1 + 2 statistics)")

	instByAccount := map[string]map[string]bool{}
	for _, pq := range plan {
		acct := labelValue(pq.labels, "account_id")
		assert.Equal(t, pq.account, acct, "the account_id label matches the query's account")
		if instByAccount[acct] == nil {
			instByAccount[acct] = map[string]bool{}
		}
		instByAccount[acct][labelValue(pq.labels, "instance_id")] = true
	}

	assert.Equal(t, map[string]map[string]bool{
		"111111111111": {"i-1": true},
		"222222222222": {"i-2": true},
	}, instByAccount, "each account collects its own instances, labeled with its own account_id")
}
