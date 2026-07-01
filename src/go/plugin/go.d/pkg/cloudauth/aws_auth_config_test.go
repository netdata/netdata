// SPDX-License-Identifier: GPL-3.0-or-later

package cloudauth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestAWSAuthConfig_ValidateWithPath(t *testing.T) {
	tests := map[string]struct {
		cfg     AWSAuthConfig
		wantErr bool
	}{
		"default mode": {
			cfg: AWSAuthConfig{Mode: AWSAuthModeDefault},
		},
		"default mode case-insensitive": {
			cfg: AWSAuthConfig{Mode: "DEFAULT"},
		},
		"empty mode": {
			cfg:     AWSAuthConfig{},
			wantErr: true,
		},
		"invalid mode": {
			cfg:     AWSAuthConfig{Mode: "bogus"},
			wantErr: true,
		},
		"access_key valid": {
			cfg: AWSAuthConfig{Mode: AWSAuthModeAccessKey, ModeAccessKey: &AWSModeAccessKeyConfig{
				AccessKeyID: "AKIAEXAMPLE", SecretAccessKey: "secret",
			}},
		},
		"access_key with session token": {
			cfg: AWSAuthConfig{Mode: AWSAuthModeAccessKey, ModeAccessKey: &AWSModeAccessKeyConfig{
				AccessKeyID: "AKIAEXAMPLE", SecretAccessKey: "secret", SessionToken: "token",
			}},
		},
		"access_key missing struct": {
			cfg:     AWSAuthConfig{Mode: AWSAuthModeAccessKey},
			wantErr: true,
		},
		"access_key missing id": {
			cfg:     AWSAuthConfig{Mode: AWSAuthModeAccessKey, ModeAccessKey: &AWSModeAccessKeyConfig{SecretAccessKey: "secret"}},
			wantErr: true,
		},
		"access_key missing secret": {
			cfg:     AWSAuthConfig{Mode: AWSAuthModeAccessKey, ModeAccessKey: &AWSModeAccessKeyConfig{AccessKeyID: "AKIAEXAMPLE"}},
			wantErr: true,
		},
		"assume_role valid single role": {
			cfg: AWSAuthConfig{Mode: AWSAuthModeAssumeRole, ModeAssumeRole: &AWSModeAssumeRoleConfig{
				Roles: []AWSAssumeRole{{RoleARN: "arn:aws:iam::000000000000:role/example"}},
			}},
		},
		"assume_role valid with external id": {
			cfg: AWSAuthConfig{Mode: AWSAuthModeAssumeRole, ModeAssumeRole: &AWSModeAssumeRoleConfig{
				Roles: []AWSAssumeRole{{RoleARN: "arn:aws:iam::000000000000:role/example", ExternalID: "ext"}},
			}},
		},
		"assume_role nil struct": {
			cfg:     AWSAuthConfig{Mode: AWSAuthModeAssumeRole},
			wantErr: true,
		},
		"assume_role no roles": {
			cfg:     AWSAuthConfig{Mode: AWSAuthModeAssumeRole, ModeAssumeRole: &AWSModeAssumeRoleConfig{}},
			wantErr: true,
		},
		"assume_role more than one role (MVP)": {
			cfg: AWSAuthConfig{Mode: AWSAuthModeAssumeRole, ModeAssumeRole: &AWSModeAssumeRoleConfig{
				Roles: []AWSAssumeRole{{RoleARN: "a"}, {RoleARN: "b"}},
			}},
			wantErr: true,
		},
		"assume_role missing role_arn": {
			cfg: AWSAuthConfig{Mode: AWSAuthModeAssumeRole, ModeAssumeRole: &AWSModeAssumeRoleConfig{
				Roles: []AWSAssumeRole{{ExternalID: "ext"}},
			}},
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := tc.cfg.ValidateWithPath("auth")
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAWSAuthConfig_NewConfig(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid config returns error", func(t *testing.T) {
		_, err := AWSAuthConfig{}.NewConfig(ctx, AWSConfigOptions{Region: "us-east-1"})
		assert.Error(t, err)
	})

	t.Run("access_key sets region and static credentials", func(t *testing.T) {
		cfg := AWSAuthConfig{Mode: AWSAuthModeAccessKey, ModeAccessKey: &AWSModeAccessKeyConfig{
			AccessKeyID: "AKIAEXAMPLE", SecretAccessKey: "secret",
		}}
		awsCfg, err := cfg.NewConfig(ctx, AWSConfigOptions{Region: "eu-west-1"})
		require.NoError(t, err)
		assert.Equal(t, "eu-west-1", awsCfg.Region)

		creds, err := awsCfg.Credentials.Retrieve(ctx)
		require.NoError(t, err)
		assert.Equal(t, "AKIAEXAMPLE", creds.AccessKeyID)
		assert.Equal(t, "secret", creds.SecretAccessKey)
	})

	t.Run("default mode builds a config with the region", func(t *testing.T) {
		awsCfg, err := AWSAuthConfig{Mode: AWSAuthModeDefault}.NewConfig(ctx, AWSConfigOptions{Region: "us-east-1"})
		require.NoError(t, err)
		assert.Equal(t, "us-east-1", awsCfg.Region)
	})

	t.Run("assume_role builds a config without resolving credentials eagerly", func(t *testing.T) {
		cfg := AWSAuthConfig{Mode: AWSAuthModeAssumeRole, ModeAssumeRole: &AWSModeAssumeRoleConfig{
			Roles: []AWSAssumeRole{{RoleARN: "arn:aws:iam::000000000000:role/example"}},
		}}
		awsCfg, err := cfg.NewConfig(ctx, AWSConfigOptions{Region: "us-east-1"})
		require.NoError(t, err)
		assert.Equal(t, "us-east-1", awsCfg.Region)
		assert.NotNil(t, awsCfg.Credentials)
	})
}

func TestAWSAuthConfig_YAMLRoundTrip(t *testing.T) {
	tests := map[string]AWSAuthConfig{
		"access_key with session token": {
			Mode:          AWSAuthModeAccessKey,
			ModeAccessKey: &AWSModeAccessKeyConfig{AccessKeyID: "AKIAEXAMPLE", SecretAccessKey: "secret", SessionToken: "token"},
		},
		"assume_role with external id": {
			Mode: AWSAuthModeAssumeRole,
			ModeAssumeRole: &AWSModeAssumeRoleConfig{
				Roles: []AWSAssumeRole{{RoleARN: "arn:aws:iam::000000000000:role/example", ExternalID: "ext"}},
			},
		},
	}
	for name, orig := range tests {
		t.Run(name, func(t *testing.T) {
			data, err := yaml.Marshal(orig)
			require.NoError(t, err)

			var got AWSAuthConfig
			require.NoError(t, yaml.Unmarshal(data, &got))
			assert.Equal(t, orig, got)
		})
	}
}
