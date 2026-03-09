// SPDX-License-Identifier: GPL-3.0-or-later

package sqladapter

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/cloudauth"
)

func TestMSSQLDriver(t *testing.T) {
	assert.Equal(t, MSSQLDriverName, MSSQLDriver(cloudauth.Config{}))
	assert.Equal(t, MSSQLAzureDriverName, MSSQLDriver(cloudauth.Config{Provider: cloudauth.ProviderAzureAD}))
}

func TestBuildMSSQLAzureADDSN(t *testing.T) {
	base := "sqlserver://localhost:1433?database=master"

	tests := []struct {
		name     string
		baseDSN  string
		cfg      cloudauth.Config
		wantErr  bool
		validate func(t *testing.T, dsn string, u *url.URL)
		parseDSN bool
	}{
		{
			name:    "provider disabled returns original dsn",
			baseDSN: base,
			cfg:     cloudauth.Config{Provider: cloudauth.ProviderNone},
			validate: func(t *testing.T, dsn string, _ *url.URL) {
				assert.Equal(t, base, dsn)
			},
		},
		{
			name:    "service principal",
			baseDSN: base,
			cfg: cloudauth.Config{
				Provider: cloudauth.ProviderAzureAD,
				AzureAD: &cloudauth.AzureADAuthConfig{
					Mode:         cloudauth.AzureADAuthModeServicePrincipal,
					TenantID:     "tenant",
					ClientID:     "client",
					ClientSecret: "secret",
				},
			},
			parseDSN: true,
			validate: func(t *testing.T, _ string, u *url.URL) {
				assert.Equal(t, "ActiveDirectoryServicePrincipal", u.Query().Get("fedauth"))
				assert.Equal(t, "client@tenant", u.User.Username())
				pass, ok := u.User.Password()
				require.True(t, ok)
				assert.Equal(t, "secret", pass)
			},
		},
		{
			name:    "managed identity with client id",
			baseDSN: base,
			cfg: cloudauth.Config{
				Provider: cloudauth.ProviderAzureAD,
				AzureAD: &cloudauth.AzureADAuthConfig{
					Mode:                    cloudauth.AzureADAuthModeManagedIdentity,
					ManagedIdentityClientID: "mi-client-id",
				},
			},
			parseDSN: true,
			validate: func(t *testing.T, _ string, u *url.URL) {
				assert.Equal(t, "ActiveDirectoryManagedIdentity", u.Query().Get("fedauth"))
				assert.Equal(t, "mi-client-id", u.Query().Get("user id"))
				assert.Nil(t, u.User)
			},
		},
		{
			name:    "default credential",
			baseDSN: base,
			cfg: cloudauth.Config{
				Provider: cloudauth.ProviderAzureAD,
				AzureAD:  &cloudauth.AzureADAuthConfig{Mode: cloudauth.AzureADAuthModeDefault},
			},
			parseDSN: true,
			validate: func(t *testing.T, _ string, u *url.URL) {
				assert.Equal(t, "ActiveDirectoryDefault", u.Query().Get("fedauth"))
				assert.Nil(t, u.User)
			},
		},
		{
			name:    "cleans mixed-case stale params",
			baseDSN: "sqlserver://olduser:oldpass@localhost:1433?database=master&FedAuth=old&User+ID=old&Password=old",
			cfg: cloudauth.Config{
				Provider: cloudauth.ProviderAzureAD,
				AzureAD:  &cloudauth.AzureADAuthConfig{Mode: cloudauth.AzureADAuthModeDefault},
			},
			parseDSN: true,
			validate: func(t *testing.T, _ string, u *url.URL) {
				assert.Empty(t, u.Query().Get("FedAuth"))
				assert.Empty(t, u.Query().Get("User ID"))
				assert.Empty(t, u.Query().Get("Password"))
				assert.Equal(t, "ActiveDirectoryDefault", u.Query().Get("fedauth"))
				assert.Nil(t, u.User)
			},
		},
		{
			name:    "invalid scheme",
			baseDSN: "server=localhost;database=master",
			cfg: cloudauth.Config{
				Provider: cloudauth.ProviderAzureAD,
				AzureAD:  &cloudauth.AzureADAuthConfig{Mode: cloudauth.AzureADAuthModeDefault},
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dsn, err := BuildMSSQLAzureADDSN(tc.baseDSN, tc.cfg)
			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			if tc.validate == nil {
				return
			}

			var u *url.URL
			if tc.parseDSN {
				parsed, parseErr := url.Parse(dsn)
				require.NoError(t, parseErr)
				u = parsed
			}
			tc.validate(t, dsn, u)
		})
	}
}
