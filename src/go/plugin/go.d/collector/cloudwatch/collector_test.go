// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"

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

func setSingleTargetRuntime(c *Collector, account string, regions []string, profiles []cwprofiles.ResolvedProfile) {
	identity := awsauth.NewIdentity("base", awsauth.CredentialConfig{Type: awsauth.CredentialTypeDefault}, nil)
	target := &targetRuntime{Name: "base", Identity: identity, Regions: regions}
	runtime := &collectorRuntime{
		Targets:      []*targetRuntime{target},
		TargetsByRef: map[string]*targetRuntime{"base": target},
		Profiles:     profiles,
	}
	for _, region := range regions {
		for _, profile := range profiles {
			runtime.Scopes = append(runtime.Scopes, collectionScope{RuleName: "test", Target: target, Profile: profile, Region: region})
		}
	}
	resolved := resolvedTarget{target: target, accountID: account}
	c.runtime = runtime
	c.resolvedTargets = []resolvedTarget{resolved}
	c.resolvedByRef = map[string]resolvedTarget{"base": resolved}
	c.resolvedRefs = map[string]struct{}{"base": {}}
	c.invalidateQueryPlan()
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
				cfg.Credentials["sdk_default"] = awsauth.CredentialConfig{Type: "bogus"}
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

func TestCollector_Check(t *testing.T) {
	t.Run("resolves account id via STS and is idempotent", func(t *testing.T) {
		f := &fakeSTS{account: "000000000000"}
		c := newTestCollector(t, validConfig(), f)
		require.NoError(t, c.Init(context.Background()))

		require.NoError(t, c.Check(context.Background()))
		require.Len(t, c.resolvedTargets, 1)
		assert.Equal(t, "000000000000", c.resolvedTargets[0].accountID)
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
}
