// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

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
	return multiTargetCollectorWithConfig(t, twoTargetConfig(), clients)
}

func multiTargetCollectorWithConfig(t *testing.T, cfg Config, clients map[string]stsClient) *Collector {
	t.Helper()
	c := New()
	c.Config = cfg
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
	require.Len(t, c.resolvedByRef, 2)
	assert.Equal(t, []string{"first", "second"}, resolvedTargetNames(c))
	assert.Equal(t, "111111111111", c.resolvedByRef["first"].accountID)
	assert.Equal(t, "111111111111", c.resolvedByRef["second"].accountID)
}

func TestEnsureTargets_FailureIsolationAndRetry(t *testing.T) {
	c := multiTargetCollector(t, map[string]stsClient{
		"first":  &seqSTS{accounts: []string{"111111111111"}},
		"second": &seqSTS{accounts: []string{"", "222222222222"}, failAt: map[int]bool{0: true}},
	})
	require.NoError(t, c.ensureTargets(context.Background()))
	assert.Equal(t, []string{"first"}, resolvedTargetNames(c))
	require.NoError(t, c.ensureTargets(context.Background()))
	assert.Equal(t, []string{"first", "second"}, resolvedTargetNames(c))
}

func TestBuildQueryPlan_FirstTargetOwnsSameAccountSeries(t *testing.T) {
	c := multiTargetCollector(t, map[string]stsClient{
		"first":  &seqSTS{accounts: []string{"111111111111"}},
		"second": &seqSTS{accounts: []string{"111111111111"}},
	})
	require.NoError(t, c.ensureTargets(context.Background()))
	c.discovery = discoverySnapshot{Instances: map[discoveryKey][]collectionInstance{
		{Target: "first", Profile: "ec2", Region: "us-east-1"}:  {{DimensionValues: []string{"i-1"}}},
		{Target: "second", Profile: "ec2", Region: "us-east-1"}: {{DimensionValues: []string{"i-1"}}},
	}}
	plan := requireBuildQueryPlan(t, c)
	require.NotEmpty(t, plan)
	perInstance := 0
	for _, metric := range c.plan.Scopes[0].Profile.Config.Metrics {
		perInstance += len(metric.Statistics)
	}
	assert.Len(t, plan, perInstance)
	for _, query := range plan {
		assert.Equal(t, "first", query.target)
		assert.Equal(t, "111111111111", labelValue(query.labels, "account_id"))
	}
}

func TestBuildQueryPlan_SameAccountDisjointResourcesSurvive(t *testing.T) {
	c := multiTargetCollector(t, map[string]stsClient{
		"first":  &seqSTS{accounts: []string{"111111111111"}},
		"second": &seqSTS{accounts: []string{"111111111111"}},
	})
	require.NoError(t, c.ensureTargets(context.Background()))
	c.discovery = discoverySnapshot{Instances: map[discoveryKey][]collectionInstance{
		{Target: "first", Profile: "ec2", Region: "us-east-1"}:  {{DimensionValues: []string{"i-1"}}},
		{Target: "second", Profile: "ec2", Region: "us-east-1"}: {{DimensionValues: []string{"i-2"}}},
	}}
	plan := requireBuildQueryPlan(t, c)
	perInstance := 0
	for _, metric := range c.plan.Scopes[0].Profile.Config.Metrics {
		perInstance += len(metric.Statistics)
	}
	assert.Len(t, plan, perInstance*2)
}

func TestCurrentQueryPlan_ClearsRetainedSeriesWhenTargetOwnershipChanges(t *testing.T) {
	c := multiTargetCollector(t, map[string]stsClient{
		"first":  &seqSTS{accounts: []string{"", "111111111111"}, failAt: map[int]bool{0: true}},
		"second": &seqSTS{accounts: []string{"111111111111"}},
	})
	require.NoError(t, c.ensureTargets(context.Background()))
	c.discovery = discoverySnapshot{Instances: map[discoveryKey][]collectionInstance{
		{Target: "first", Profile: "ec2", Region: "us-east-1"}:  {{DimensionValues: []string{"i-1"}}},
		{Target: "second", Profile: "ec2", Region: "us-east-1"}: {{DimensionValues: []string{"i-1"}}},
	}}

	oldPlan := requireCurrentQueryPlan(t, c)
	require.NotEmpty(t, oldPlan)
	oldQuery := oldPlan[0]
	require.Equal(t, "second", oldQuery.target)
	c.observations.queries[oldQuery.key] = queryState{hasObservation: true, observation: 42, observationAt: time.Unix(1, 0), lastCompletedEnd: time.Unix(2, 0)}

	require.NoError(t, c.ensureTargets(context.Background()))
	newPlan := requireCurrentQueryPlan(t, c)
	require.NotEmpty(t, newPlan)
	newQuery := newPlan[0]
	require.Equal(t, "first", newQuery.target)
	assert.NotEqual(t, oldQuery.key, newQuery.key)
	assert.NotContains(t, c.observations.queries, oldQuery.key)
	assert.NotContains(t, c.observations.queries, newQuery.key, "a target ownership change starts with no retained value")
}
