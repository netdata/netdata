// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/awsauth"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type seqSTS struct {
	mu       sync.Mutex
	accounts []string
	failAt   map[int]bool
	calls    int
}

func (f *seqSTS) GetCallerIdentity(context.Context, *sts.GetCallerIdentityInput, ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := f.calls
	f.calls++
	if f.failAt[i] {
		return nil, errors.New("sts denied")
	}
	account := ""
	if i < len(f.accounts) {
		account = f.accounts[i]
	}
	return &sts.GetCallerIdentityOutput{Account: aws.String(account)}, nil
}

func twoTargetConfig() Config {
	falseValue := false
	cfg := validBaseConfig()
	cfg.Targets = []TargetConfig{
		{Name: "first", Credentials: "sdk_default"},
		{Name: "second", Credentials: "sdk_default"},
	}
	cfg.Rules = []RuleConfig{{
		Name: "both", Targets: []string{"first", "second"}, Regions: []string{"us-east-1"},
		Profiles: &ProfileSelectorConfig{Defaults: &falseValue, Include: []string{"ec2"}},
	}}
	return cfg
}

func multiTargetCollector(t *testing.T, clients map[string]stsClient) *Collector {
	t.Helper()
	c := New()
	c.Config = twoTargetConfig()
	c.newAWSConfig = func(_ context.Context, identity awsauth.Identity, _ string) (aws.Config, error) {
		return aws.Config{Region: identity.Ref}, nil
	}
	c.newSTSClient = func(cfg aws.Config) stsClient { return clients[cfg.Region] }
	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, c.ensurePlan())
	return c
}

func TestEnsureTargets_RetainsSameAccountTargets(t *testing.T) {
	c := multiTargetCollector(t, map[string]stsClient{
		"first":  &seqSTS{accounts: []string{"111111111111"}},
		"second": &seqSTS{accounts: []string{"111111111111"}},
	})
	require.NoError(t, c.ensureTargets(context.Background()))
	require.Len(t, c.resolvedTargets, 2)
	assert.Equal(t, []string{"first", "second"}, c.resolvedTargetRefs())
	assert.Equal(t, "111111111111", c.resolvedTargets[0].accountID)
	assert.Equal(t, "111111111111", c.resolvedTargets[1].accountID)
}

func TestEnsureTargets_FailureIsolationAndRetry(t *testing.T) {
	c := multiTargetCollector(t, map[string]stsClient{
		"first":  &seqSTS{accounts: []string{"111111111111"}},
		"second": &seqSTS{accounts: []string{"", "222222222222"}, failAt: map[int]bool{0: true}},
	})
	require.NoError(t, c.ensureTargets(context.Background()))
	assert.Equal(t, []string{"first"}, c.resolvedTargetRefs())
	require.NoError(t, c.ensureTargets(context.Background()))
	assert.Equal(t, []string{"first", "second"}, c.resolvedTargetRefs())
}

func TestBuildQueryPlan_FirstTargetOwnsSameAccountSeries(t *testing.T) {
	c := multiTargetCollector(t, map[string]stsClient{
		"first":  &seqSTS{accounts: []string{"111111111111"}},
		"second": &seqSTS{accounts: []string{"111111111111"}},
	})
	require.NoError(t, c.ensureTargets(context.Background()))
	c.discovery = discoverySnapshot{Instances: map[discoveryKey][]discoveredInstance{
		{Target: "first", Profile: "ec2", Region: "us-east-1"}:  {{DimensionValues: []string{"i-1"}}},
		{Target: "second", Profile: "ec2", Region: "us-east-1"}: {{DimensionValues: []string{"i-1"}}},
	}}
	plan := c.buildQueryPlan()
	require.NotEmpty(t, plan)
	perInstance := 0
	for _, metric := range c.plan.Scopes[0].Profile.Config.Metrics {
		perInstance += len(metric.Statistics)
	}
	assert.Len(t, plan, perInstance)
	for _, query := range plan {
		assert.Equal(t, "first", query.target)
		assert.Equal(t, "111111111111", query.account)
	}
}

func TestBuildQueryPlan_SameAccountDisjointResourcesSurvive(t *testing.T) {
	c := multiTargetCollector(t, map[string]stsClient{
		"first":  &seqSTS{accounts: []string{"111111111111"}},
		"second": &seqSTS{accounts: []string{"111111111111"}},
	})
	require.NoError(t, c.ensureTargets(context.Background()))
	c.discovery = discoverySnapshot{Instances: map[discoveryKey][]discoveredInstance{
		{Target: "first", Profile: "ec2", Region: "us-east-1"}:  {{DimensionValues: []string{"i-1"}}},
		{Target: "second", Profile: "ec2", Region: "us-east-1"}: {{DimensionValues: []string{"i-2"}}},
	}}
	plan := c.buildQueryPlan()
	perInstance := 0
	for _, metric := range c.plan.Scopes[0].Profile.Config.Metrics {
		perInstance += len(metric.Statistics)
	}
	assert.Len(t, plan, perInstance*2)
}
