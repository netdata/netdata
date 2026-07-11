// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"bytes"
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/awsauth"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,
	} {
		require.NotNil(t, data, name)
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	collecttest.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

type fakeSTS struct {
	account string
	err     error
	calls   int
}

func (f *fakeSTS) GetCallerIdentity(context.Context, *sts.GetCallerIdentityInput, ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return &sts.GetCallerIdentityOutput{Account: aws.String(f.account)}, nil
}

func validConfig() Config {
	return validBaseConfig()
}

func configureExactRule(c *Collector, regions, profiles []string) {
	defaults := false
	c.Config = validConfig()
	c.Config.Rules[0].Regions = regions
	c.Config.Rules[0].Profiles = &ProfileSelectorConfig{
		Defaults: &defaults,
		Include:  profiles,
	}
	c.applyDefaults()
}

func setSingleTargetPlan(c *Collector, account string, regions []string, profiles []cwprofiles.ResolvedProfile) {
	identity := awsauth.NewIdentity("base", awsauth.CredentialConfig{Type: awsauth.CredentialTypeDefault}, nil)
	target := &collectionTarget{Name: "base", Identity: identity, Regions: regions}
	plan := &collectionPlan{
		Targets:  []*collectionTarget{target},
		Profiles: profiles,
	}
	for _, region := range regions {
		for _, profile := range profiles {
			plan.Scopes = append(plan.Scopes, collectionScope{Target: target, Profile: profile, Region: region})
		}
	}
	resolved := resolvedTarget{target: target, accountID: account}
	c.plan = plan
	c.resolvedByRef = map[string]resolvedTarget{"base": resolved}
	c.invalidateQueryPlan()
}

func resolvedTargetNames(c *Collector) []string {
	var names []string
	for _, target := range c.plan.Targets {
		if _, ok := c.resolvedByRef[target.Name]; ok {
			names = append(names, target.Name)
		}
	}
	return names
}

func newTestCollector(t *testing.T, cfg Config, f *fakeSTS) *Collector {
	t.Helper()
	c := New()
	c.Config = cfg
	c.newAWSConfig = func(context.Context, awsauth.Identity, string) (aws.Config, error) {
		return aws.Config{}, nil
	}
	c.newSTSClient = func(aws.Config) stsClient { return f }
	return c
}

// useFakeClient wires the AWS-config and CloudWatch-client seams so every region
// resolves to the given fake — the common per-test CloudWatch client setup.
func useFakeClient(c *Collector, fake cloudwatchClient) {
	c.newAWSConfig = func(_ context.Context, _ awsauth.Identity, region string) (aws.Config, error) {
		return aws.Config{Region: region}, nil
	}
	c.newCloudWatchClient = func(aws.Config) cloudwatchClient { return fake }
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		cfg     Config
		wantErr bool
	}{
		"valid default credentials": {
			cfg: validConfig(),
		},
		"missing rules": {
			cfg:     func() Config { cfg := validConfig(); cfg.Rules = nil; return cfg }(),
			wantErr: true,
		},
		"invalid credential type": {
			cfg: func() Config {
				cfg := validConfig()
				cfg.Credentials[0].Type = "bogus"
				return cfg
			}(),
			wantErr: true,
		},
		"unknown rule target": {
			cfg: func() Config {
				cfg := validConfig()
				cfg.Rules[0].Targets = []string{"missing"}
				return cfg
			}(),
			wantErr: true,
		},
		"duplicate rule target": {
			cfg: func() Config {
				cfg := validConfig()
				cfg.Rules[0].Targets = []string{"base", "base"}
				return cfg
			}(),
			wantErr: true,
		},
		"empty rule target": {
			cfg: func() Config {
				cfg := validConfig()
				cfg.Rules[0].Targets = []string{""}
				return cfg
			}(),
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := New()
			c.Config = tc.cfg
			err := c.Init(context.Background())
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCollector_Init_appliesDefaults(t *testing.T) {
	c := New()
	c.Config = validConfig()
	require.NoError(t, c.Init(context.Background()))

	assert.Equal(t, defaultUpdateEvery, c.UpdateEvery)
	assert.Equal(t, defaultDiscoveryRefresh, c.Discovery.RefreshEvery)
	assert.Equal(t, defaultQueryOffset, c.QueryOffset)
	assert.True(t, c.recentlyActiveOnly())
}

func TestCollector_InitRejectsTargetCapBeforeDetailedTraversal(t *testing.T) {
	cfg := validConfig()
	for i := 1; i <= maxTargets; i++ {
		cfg.Targets = append(cfg.Targets, TargetConfig{Name: "INVALID NAME", Credentials: "sdk_default"})
	}
	c := New()
	c.Config = cfg
	err := c.Init(context.Background())
	require.Error(t, err)
	assert.ErrorContains(t, err, "maximum is 64")
	assert.NotContains(t, err.Error(), "must match")
}

func TestCollector_Check(t *testing.T) {
	t.Run("resolves account id via STS and is idempotent", func(t *testing.T) {
		f := &fakeSTS{account: "000000000000"}
		c := newTestCollector(t, validConfig(), f)
		require.NoError(t, c.Init(context.Background()))

		require.NoError(t, c.Check(context.Background()))
		require.Len(t, c.resolvedByRef, 1)
		assert.Equal(t, "000000000000", c.resolvedByRef["base"].accountID)
		assert.Equal(t, 1, f.calls)

		require.NoError(t, c.Check(context.Background()))
		assert.Equal(t, 1, f.calls, "account identity must be resolved only once")
	})

	t.Run("STS failure fails Check", func(t *testing.T) {
		f := &fakeSTS{err: errors.New("access denied")}
		c := newTestCollector(t, validConfig(), f)
		require.NoError(t, c.Init(context.Background()))

		assert.Error(t, c.Check(context.Background()))
	})

	t.Run("empty account fails Check", func(t *testing.T) {
		f := &fakeSTS{account: ""}
		c := newTestCollector(t, validConfig(), f)
		require.NoError(t, c.Init(context.Background()))

		assert.Error(t, c.Check(context.Background()))
	})

	t.Run("sanitizes provider errors in return and logs", func(t *testing.T) {
		const sensitive = "SENSITIVE_PROVIDER_DETAIL"
		var buf bytes.Buffer
		f := &fakeSTS{err: errors.New(sensitive)}
		c := newTestCollector(t, validConfig(), f)
		c.Logger = logger.NewWithWriter(&buf)
		require.NoError(t, c.Init(context.Background()))

		err := c.Check(context.Background())
		require.Error(t, err)
		assert.NotContains(t, err.Error(), sensitive)
		assert.NotContains(t, buf.String(), sensitive)
	})
}

type concurrentSTS struct {
	started chan struct{}
	release chan struct{}
	current atomic.Int32
	maximum atomic.Int32
}

func (f *concurrentSTS) GetCallerIdentity(context.Context, *sts.GetCallerIdentityInput, ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	current := f.current.Add(1)
	for {
		maximum := f.maximum.Load()
		if current <= maximum || f.maximum.CompareAndSwap(maximum, current) {
			break
		}
	}
	f.started <- struct{}{}
	<-f.release
	f.current.Add(-1)
	return &sts.GetCallerIdentityOutput{Account: aws.String("000000000000")}, nil
}

func TestCollector_CheckResolvesAllTargetsConcurrently(t *testing.T) {
	defaults := false
	cfg := validConfig()
	cfg.Targets = []TargetConfig{
		{Name: "target-a", Credentials: "sdk_default"},
		{Name: "target-b", Credentials: "sdk_default"},
		{Name: "target-c", Credentials: "sdk_default"},
	}
	cfg.Rules[0].Targets = []string{"target-a", "target-b", "target-c"}
	cfg.Rules[0].Profiles = &ProfileSelectorConfig{Defaults: &defaults, Include: []string{"ec2"}}

	fake := &concurrentSTS{started: make(chan struct{}, len(cfg.Targets)), release: make(chan struct{})}
	c := New()
	c.Config = cfg
	c.newAWSConfig = func(context.Context, awsauth.Identity, string) (aws.Config, error) { return aws.Config{}, nil }
	c.newSTSClient = func(aws.Config) stsClient { return fake }

	errCh := make(chan error, 1)
	go func() { errCh <- c.Check(context.Background()) }()

	allStarted := true
	for range cfg.Targets {
		select {
		case <-fake.started:
		case <-time.After(250 * time.Millisecond):
			allStarted = false
		}
		if !allStarted {
			break
		}
	}
	close(fake.release)
	require.NoError(t, <-errCh)
	assert.True(t, allStarted, "every target must be attempted without waiting for another target's timeout")
	assert.Equal(t, int32(len(cfg.Targets)), fake.maximum.Load())
}
