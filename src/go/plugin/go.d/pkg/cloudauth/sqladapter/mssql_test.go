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

	t.Run("provider disabled returns original dsn", func(t *testing.T) {
		dsn, err := BuildMSSQLAzureADDSN(base, cloudauth.Config{Provider: cloudauth.ProviderNone})
		require.NoError(t, err)
		assert.Equal(t, base, dsn)
	})

	t.Run("service principal", func(t *testing.T) {
		cfg := cloudauth.Config{
			Provider: cloudauth.ProviderAzureAD,
			AzureAD: cloudauth.AzureADAuthConfig{
				Mode:         cloudauth.AzureADAuthModeServicePrincipal,
				TenantID:     "tenant",
				ClientID:     "client",
				ClientSecret: "secret",
			},
		}

		dsn, err := BuildMSSQLAzureADDSN(base, cfg)
		require.NoError(t, err)

		u, err := url.Parse(dsn)
		require.NoError(t, err)

		assert.Equal(t, "ActiveDirectoryServicePrincipal", u.Query().Get("fedauth"))
		assert.Equal(t, "client@tenant", u.User.Username())
		pass, ok := u.User.Password()
		require.True(t, ok)
		assert.Equal(t, "secret", pass)
	})

	t.Run("managed identity with client id", func(t *testing.T) {
		cfg := cloudauth.Config{
			Provider: cloudauth.ProviderAzureAD,
			AzureAD: cloudauth.AzureADAuthConfig{
				Mode:                    cloudauth.AzureADAuthModeManagedIdentity,
				ManagedIdentityClientID: "mi-client-id",
			},
		}

		dsn, err := BuildMSSQLAzureADDSN(base, cfg)
		require.NoError(t, err)

		u, err := url.Parse(dsn)
		require.NoError(t, err)

		assert.Equal(t, "ActiveDirectoryManagedIdentity", u.Query().Get("fedauth"))
		assert.Equal(t, "mi-client-id", u.Query().Get("user id"))
		assert.Nil(t, u.User)
	})

	t.Run("default credential", func(t *testing.T) {
		cfg := cloudauth.Config{
			Provider: cloudauth.ProviderAzureAD,
			AzureAD:  cloudauth.AzureADAuthConfig{Mode: cloudauth.AzureADAuthModeDefault},
		}

		dsn, err := BuildMSSQLAzureADDSN(base, cfg)
		require.NoError(t, err)

		u, err := url.Parse(dsn)
		require.NoError(t, err)

		assert.Equal(t, "ActiveDirectoryDefault", u.Query().Get("fedauth"))
		assert.Nil(t, u.User)
	})

	t.Run("cleans mixed-case stale params", func(t *testing.T) {
		stale := "sqlserver://olduser:oldpass@localhost:1433?database=master&FedAuth=old&User+ID=old&Password=old"
		cfg := cloudauth.Config{
			Provider: cloudauth.ProviderAzureAD,
			AzureAD:  cloudauth.AzureADAuthConfig{Mode: cloudauth.AzureADAuthModeDefault},
		}

		dsn, err := BuildMSSQLAzureADDSN(stale, cfg)
		require.NoError(t, err)

		u, err := url.Parse(dsn)
		require.NoError(t, err)

		assert.Empty(t, u.Query().Get("FedAuth"))
		assert.Empty(t, u.Query().Get("User ID"))
		assert.Empty(t, u.Query().Get("Password"))
		assert.Equal(t, "ActiveDirectoryDefault", u.Query().Get("fedauth"))
		assert.Nil(t, u.User)
	})

	t.Run("invalid scheme", func(t *testing.T) {
		cfg := cloudauth.Config{
			Provider: cloudauth.ProviderAzureAD,
			AzureAD:  cloudauth.AzureADAuthConfig{Mode: cloudauth.AzureADAuthModeDefault},
		}

		_, err := BuildMSSQLAzureADDSN("server=localhost;database=master", cfg)
		require.Error(t, err)
	})
}
