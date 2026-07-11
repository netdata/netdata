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

func TestCredentialConfig_ValidateWithPath(t *testing.T) {
	tests := map[string]struct {
		cfg     CredentialConfig
		wantErr bool
	}{
		"default": {cfg: CredentialConfig{Type: CredentialTypeDefault}},
		"default rejects static fields": {
			cfg:     CredentialConfig{Type: CredentialTypeDefault, AccessKeyID: "unexpected"},
			wantErr: true,
		},
		"static": {
			cfg: CredentialConfig{Type: CredentialTypeStatic, AccessKeyID: "AKIAEXAMPLE", SecretAccessKey: "secret"},
		},
		"static with session token": {
			cfg: CredentialConfig{Type: CredentialTypeStatic, AccessKeyID: "AKIAEXAMPLE", SecretAccessKey: "secret", SessionToken: "token"},
		},
		"static missing id": {
			cfg:     CredentialConfig{Type: CredentialTypeStatic, SecretAccessKey: "secret"},
			wantErr: true,
		},
		"static missing secret": {
			cfg:     CredentialConfig{Type: CredentialTypeStatic, AccessKeyID: "AKIAEXAMPLE"},
			wantErr: true,
		},
		"missing type": {wantErr: true},
		"invalid type": {cfg: CredentialConfig{Type: "bogus"}, wantErr: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := tc.cfg.ValidateWithPath("credentials.test")
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIdentity_NewConfig(t *testing.T) {
	ctx := context.Background()

	t.Run("default credentials set region", func(t *testing.T) {
		id := NewIdentity("base", CredentialConfig{Type: CredentialTypeDefault}, nil)
		cfg, err := id.NewConfig(ctx, ConfigOptions{Region: "us-east-1"})
		require.NoError(t, err)
		assert.Equal(t, "us-east-1", cfg.Region)
	})

	t.Run("static credentials and session token", func(t *testing.T) {
		id := NewIdentity("base", CredentialConfig{
			Type: CredentialTypeStatic, AccessKeyID: "AKIAEXAMPLE", SecretAccessKey: "secret", SessionToken: "session",
		}, nil)
		cfg, err := id.NewConfig(ctx, ConfigOptions{Region: "eu-west-1"})
		require.NoError(t, err)
		creds, err := cfg.Credentials.Retrieve(ctx)
		require.NoError(t, err)
		assert.Equal(t, "AKIAEXAMPLE", creds.AccessKeyID)
		assert.Equal(t, "secret", creds.SecretAccessKey)
		assert.Equal(t, "session", creds.SessionToken)
	})

	t.Run("invalid identity", func(t *testing.T) {
		_, err := NewIdentity("", CredentialConfig{}, nil).NewConfig(ctx, ConfigOptions{})
		assert.Error(t, err)
	})
}

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

func TestIdentity_StaticCredentialsAssumeRole(t *testing.T) {
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

	t.Setenv("AWS_ENDPOINT_URL_STS", srv.URL)
	id := NewIdentity("production", CredentialConfig{
		Type: CredentialTypeStatic, AccessKeyID: "AKIABASEIDENTITY", SecretAccessKey: "base-secret",
	}, &AssumeRoleConfig{RoleARN: "arn:aws:iam::000000000000:role/example", ExternalID: "ext-123"})
	cfg, err := id.NewConfig(ctx, ConfigOptions{Region: "us-west-2"})
	require.NoError(t, err)
	creds, err := cfg.Credentials.Retrieve(ctx)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "AssumeRole", gotAction)
	assert.Equal(t, "arn:aws:iam::000000000000:role/example", gotRoleARN)
	assert.Equal(t, "ext-123", gotExternalID)
	assert.Equal(t, "netdata", gotSessionName)
	assert.Contains(t, gotAuth, "Credential=AKIABASEIDENTITY/")
	assert.Contains(t, gotAuth, "/us-west-2/sts/aws4_request")
	assert.Equal(t, "ASIATEMPKEY", creds.AccessKeyID)
	assert.Equal(t, "temp-session-token", creds.SessionToken)
}

func TestCredentialConfig_YAMLRoundTrip(t *testing.T) {
	want := CredentialConfig{
		Type: CredentialTypeStatic, AccessKeyID: "AKIAEXAMPLE", SecretAccessKey: "secret", SessionToken: "token",
	}
	data, err := yaml.Marshal(want)
	require.NoError(t, err)
	var got CredentialConfig
	require.NoError(t, yaml.Unmarshal(data, &got))
	assert.Equal(t, want, got)
}
