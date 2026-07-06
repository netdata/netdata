// SPDX-License-Identifier: GPL-3.0-or-later

package awsauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestConfig_ValidateWithPath(t *testing.T) {
	tests := map[string]struct {
		cfg     Config
		wantErr bool
	}{
		"default mode": {
			cfg: Config{Mode: ModeDefault},
		},
		"default mode case-insensitive": {
			cfg: Config{Mode: "DEFAULT"},
		},
		"empty mode": {
			cfg:     Config{},
			wantErr: true,
		},
		"invalid mode": {
			cfg:     Config{Mode: "bogus"},
			wantErr: true,
		},
		"access_key valid": {
			cfg: Config{Mode: ModeAccessKey, ModeAccessKey: &ModeAccessKeyConfig{
				AccessKeyID: "AKIAEXAMPLE", SecretAccessKey: "secret",
			}},
		},
		"access_key with session token": {
			cfg: Config{Mode: ModeAccessKey, ModeAccessKey: &ModeAccessKeyConfig{
				AccessKeyID: "AKIAEXAMPLE", SecretAccessKey: "secret", SessionToken: "token",
			}},
		},
		"access_key missing struct": {
			cfg:     Config{Mode: ModeAccessKey},
			wantErr: true,
		},
		"access_key missing id": {
			cfg:     Config{Mode: ModeAccessKey, ModeAccessKey: &ModeAccessKeyConfig{SecretAccessKey: "secret"}},
			wantErr: true,
		},
		"access_key missing secret": {
			cfg:     Config{Mode: ModeAccessKey, ModeAccessKey: &ModeAccessKeyConfig{AccessKeyID: "AKIAEXAMPLE"}},
			wantErr: true,
		},
		"assume_role valid single role": {
			cfg: Config{Mode: ModeAssumeRole, ModeAssumeRole: &ModeAssumeRoleConfig{
				Roles: []AssumeRole{{RoleARN: "arn:aws:iam::000000000000:role/example"}},
			}},
		},
		"assume_role valid with external id": {
			cfg: Config{Mode: ModeAssumeRole, ModeAssumeRole: &ModeAssumeRoleConfig{
				Roles: []AssumeRole{{RoleARN: "arn:aws:iam::000000000000:role/example", ExternalID: "ext"}},
			}},
		},
		"assume_role valid multiple roles": {
			cfg: Config{Mode: ModeAssumeRole, ModeAssumeRole: &ModeAssumeRoleConfig{
				Roles: []AssumeRole{
					{RoleARN: "arn:aws:iam::111111111111:role/a"},
					{RoleARN: "arn:aws:iam::222222222222:role/b"},
				},
			}},
		},
		"assume_role valid with include_base_account": {
			cfg: Config{Mode: ModeAssumeRole, ModeAssumeRole: &ModeAssumeRoleConfig{
				Roles:              []AssumeRole{{RoleARN: "arn:aws:iam::111111111111:role/a"}},
				IncludeBaseAccount: true,
			}},
		},
		"assume_role nil struct": {
			cfg:     Config{Mode: ModeAssumeRole},
			wantErr: true,
		},
		"assume_role no roles": {
			cfg:     Config{Mode: ModeAssumeRole, ModeAssumeRole: &ModeAssumeRoleConfig{}},
			wantErr: true,
		},
		"assume_role missing role_arn": {
			cfg: Config{Mode: ModeAssumeRole, ModeAssumeRole: &ModeAssumeRoleConfig{
				Roles: []AssumeRole{{ExternalID: "ext"}},
			}},
			wantErr: true,
		},
		"assume_role one of several roles missing role_arn": {
			cfg: Config{Mode: ModeAssumeRole, ModeAssumeRole: &ModeAssumeRoleConfig{
				Roles: []AssumeRole{{RoleARN: "arn:aws:iam::111111111111:role/a"}, {ExternalID: "ext"}},
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

func TestConfig_Identities(t *testing.T) {
	tests := map[string]struct {
		cfg      Config
		wantRefs []string
	}{
		"default is one base identity": {
			cfg:      Config{Mode: ModeDefault},
			wantRefs: []string{"default"},
		},
		"access_key is one base identity": {
			cfg: Config{Mode: ModeAccessKey, ModeAccessKey: &ModeAccessKeyConfig{
				AccessKeyID: "AKIAEXAMPLE", SecretAccessKey: "secret",
			}},
			wantRefs: []string{"access_key"},
		},
		"assume_role single role": {
			cfg: Config{Mode: ModeAssumeRole, ModeAssumeRole: &ModeAssumeRoleConfig{
				Roles: []AssumeRole{{RoleARN: "arn:a"}},
			}},
			wantRefs: []string{"arn:a"},
		},
		"assume_role multiple roles keep config order": {
			cfg: Config{Mode: ModeAssumeRole, ModeAssumeRole: &ModeAssumeRoleConfig{
				Roles: []AssumeRole{{RoleARN: "arn:a"}, {RoleARN: "arn:b"}, {RoleARN: "arn:c"}},
			}},
			wantRefs: []string{"arn:a", "arn:b", "arn:c"},
		},
		"assume_role with include_base_account appends base last": {
			cfg: Config{Mode: ModeAssumeRole, ModeAssumeRole: &ModeAssumeRoleConfig{
				Roles:              []AssumeRole{{RoleARN: "arn:a"}, {RoleARN: "arn:b"}},
				IncludeBaseAccount: true,
			}},
			wantRefs: []string{"arn:a", "arn:b", "base"},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ids := tc.cfg.Identities()
			refs := make([]string, 0, len(ids))
			for _, id := range ids {
				refs = append(refs, id.Ref)
			}
			assert.Equal(t, tc.wantRefs, refs)
		})
	}
}

func TestIdentity_NewConfig(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid config returns error", func(t *testing.T) {
		_, err := Config{}.Identities()[0].NewConfig(ctx, ConfigOptions{Region: "us-east-1"})
		assert.Error(t, err)
	})

	t.Run("access_key sets region and static credentials", func(t *testing.T) {
		cfg := Config{Mode: ModeAccessKey, ModeAccessKey: &ModeAccessKeyConfig{
			AccessKeyID: "AKIAEXAMPLE", SecretAccessKey: "secret",
		}}
		awsCfg, err := cfg.Identities()[0].NewConfig(ctx, ConfigOptions{Region: "eu-west-1"})
		require.NoError(t, err)
		assert.Equal(t, "eu-west-1", awsCfg.Region)

		creds, err := awsCfg.Credentials.Retrieve(ctx)
		require.NoError(t, err)
		assert.Equal(t, "AKIAEXAMPLE", creds.AccessKeyID)
		assert.Equal(t, "secret", creds.SecretAccessKey)
	})

	t.Run("access_key propagates the session token", func(t *testing.T) {
		cfg := Config{Mode: ModeAccessKey, ModeAccessKey: &ModeAccessKeyConfig{
			AccessKeyID: "AKIAEXAMPLE", SecretAccessKey: "secret", SessionToken: "session-token-value",
		}}
		awsCfg, err := cfg.Identities()[0].NewConfig(ctx, ConfigOptions{Region: "eu-west-1"})
		require.NoError(t, err)

		creds, err := awsCfg.Credentials.Retrieve(ctx)
		require.NoError(t, err)
		assert.Equal(t, "session-token-value", creds.SessionToken, "temporary-credential session token must propagate")
	})

	t.Run("default mode builds a config with the region", func(t *testing.T) {
		awsCfg, err := Config{Mode: ModeDefault}.Identities()[0].NewConfig(ctx, ConfigOptions{Region: "us-east-1"})
		require.NoError(t, err)
		assert.Equal(t, "us-east-1", awsCfg.Region)
	})

	t.Run("assume_role builds a config without resolving credentials eagerly", func(t *testing.T) {
		cfg := Config{Mode: ModeAssumeRole, ModeAssumeRole: &ModeAssumeRoleConfig{
			Roles: []AssumeRole{{RoleARN: "arn:aws:iam::000000000000:role/example"}},
		}}
		awsCfg, err := cfg.Identities()[0].NewConfig(ctx, ConfigOptions{Region: "us-east-1"})
		require.NoError(t, err)
		assert.Equal(t, "us-east-1", awsCfg.Region)
		assert.NotNil(t, awsCfg.Credentials)
	})

	t.Run("assume_role multiple roles each build a config", func(t *testing.T) {
		cfg := Config{Mode: ModeAssumeRole, ModeAssumeRole: &ModeAssumeRoleConfig{
			Roles: []AssumeRole{
				{RoleARN: "arn:aws:iam::111111111111:role/a"},
				{RoleARN: "arn:aws:iam::222222222222:role/b"},
			},
		}}
		ids := cfg.Identities()
		require.Len(t, ids, 2)
		for _, id := range ids {
			awsCfg, err := id.NewConfig(ctx, ConfigOptions{Region: "us-east-1"})
			require.NoError(t, err)
			assert.Equal(t, "us-east-1", awsCfg.Region)
			assert.NotNil(t, awsCfg.Credentials)
		}
	})

	t.Run("included base identity builds a base config without assuming a role", func(t *testing.T) {
		cfg := Config{Mode: ModeAssumeRole, ModeAssumeRole: &ModeAssumeRoleConfig{
			Roles:              []AssumeRole{{RoleARN: "arn:aws:iam::111111111111:role/a"}},
			IncludeBaseAccount: true,
		}}
		ids := cfg.Identities()
		require.Len(t, ids, 2)
		base := ids[len(ids)-1]
		require.Equal(t, "base", base.Ref)
		awsCfg, err := base.NewConfig(ctx, ConfigOptions{Region: "us-east-1"})
		require.NoError(t, err)
		assert.Equal(t, "us-east-1", awsCfg.Region)
	})
}

// assumeRoleResponseXML is a canned STS AssumeRole success response with a
// far-future expiration so the credentials cache treats the result as valid.
const assumeRoleResponseXML = `<AssumeRoleResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <AssumeRoleResult>
    <Credentials>
      <AccessKeyId>ASIATEMPKEY</AccessKeyId>
      <SecretAccessKey>temp-secret</SecretAccessKey>
      <SessionToken>temp-session-token</SessionToken>
      <Expiration>2999-12-31T23:59:59Z</Expiration>
    </Credentials>
    <AssumedRoleUser>
      <Arn>arn:aws:sts::000000000000:assumed-role/example/netdata</Arn>
      <AssumedRoleId>AROAEXAMPLE:netdata</AssumedRoleId>
    </AssumedRoleUser>
  </AssumeRoleResult>
  <ResponseMetadata><RequestId>test</RequestId></ResponseMetadata>
</AssumeRoleResponse>`

// TestIdentity_AssumeRoleRequest drives a real AssumeRole call against a fake STS
// endpoint and asserts the provider sends the configured role ARN, external id, and
// stable session name, signs with the configured STS region, and returns the
// temporary credentials from the response.
func TestIdentity_AssumeRoleRequest(t *testing.T) {
	ctx := context.Background()

	var mu sync.Mutex
	var gotAction, gotRoleARN, gotExternalID, gotSessionName, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		mu.Lock()
		gotAction = r.PostForm.Get("Action")
		gotRoleARN = r.PostForm.Get("RoleArn")
		gotExternalID = r.PostForm.Get("ExternalId")
		gotSessionName = r.PostForm.Get("RoleSessionName")
		gotAuth = r.Header.Get("Authorization")
		mu.Unlock()
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(assumeRoleResponseXML))
	}))
	defer srv.Close()

	// A base identity to sign the AssumeRole request, and redirect STS to the fake.
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIABASEIDENTITY")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "base-secret")
	t.Setenv("AWS_ENDPOINT_URL_STS", srv.URL)

	cfg := Config{Mode: ModeAssumeRole, ModeAssumeRole: &ModeAssumeRoleConfig{
		Roles: []AssumeRole{{RoleARN: "arn:aws:iam::000000000000:role/example", ExternalID: "ext-123"}},
	}}
	awsCfg, err := cfg.Identities()[0].NewConfig(ctx, ConfigOptions{Region: "us-west-2"})
	require.NoError(t, err)

	creds, err := awsCfg.Credentials.Retrieve(ctx)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "AssumeRole", gotAction)
	assert.Equal(t, "arn:aws:iam::000000000000:role/example", gotRoleARN, "configured role ARN is sent")
	assert.Equal(t, "ext-123", gotExternalID, "configured external id is sent")
	assert.Equal(t, "netdata", gotSessionName, "stable session name is sent")
	assert.Contains(t, gotAuth, "/us-west-2/sts/aws4_request", "request is signed with the configured STS region")
	assert.Equal(t, "ASIATEMPKEY", creds.AccessKeyID, "temporary credentials from STS flow back to the caller")
	assert.Equal(t, "temp-session-token", creds.SessionToken)
}

func TestConfig_YAMLRoundTrip(t *testing.T) {
	tests := map[string]Config{
		"access_key with session token": {
			Mode:          ModeAccessKey,
			ModeAccessKey: &ModeAccessKeyConfig{AccessKeyID: "AKIAEXAMPLE", SecretAccessKey: "secret", SessionToken: "token"},
		},
		"assume_role with external id": {
			Mode: ModeAssumeRole,
			ModeAssumeRole: &ModeAssumeRoleConfig{
				Roles: []AssumeRole{{RoleARN: "arn:aws:iam::000000000000:role/example", ExternalID: "ext"}},
			},
		},
		"assume_role multiple roles with include_base_account": {
			Mode: ModeAssumeRole,
			ModeAssumeRole: &ModeAssumeRoleConfig{
				Roles: []AssumeRole{
					{RoleARN: "arn:aws:iam::111111111111:role/a"},
					{RoleARN: "arn:aws:iam::222222222222:role/b", ExternalID: "ext"},
				},
				IncludeBaseAccount: true,
			},
		},
	}
	for name, orig := range tests {
		t.Run(name, func(t *testing.T) {
			data, err := yaml.Marshal(orig)
			require.NoError(t, err)

			var got Config
			require.NoError(t, yaml.Unmarshal(data, &got))
			assert.Equal(t, orig, got)
		})
	}
}
