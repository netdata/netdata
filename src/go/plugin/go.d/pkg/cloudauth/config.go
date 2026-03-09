// SPDX-License-Identifier: GPL-3.0-or-later

package cloudauth

import (
	"errors"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/cloudauth/azureadauth"
)

type Config struct {
	Provider string             `yaml:"provider,omitempty" json:"provider,omitempty"`
	AzureAD  azureadauth.Config `yaml:"azure_ad,omitempty" json:"azure_ad,omitempty"`
}

func (c Config) ProviderName() string {
	return normalizeProviderValue(c.Provider)
}

func (c Config) IsProvider(provider string) bool {
	return c.ProviderName() == normalizeProviderValue(provider)
}

func (c Config) IsEnabled() bool {
	return c.ProviderName() != ProviderNone
}

func (c Config) Validate() error {
	switch c.ProviderName() {
	case ProviderNone:
		return nil
	case ProviderAzureAD:
		return c.AzureAD.Validate()
	default:
		return fmt.Errorf("cloud_auth.provider %q is invalid: expected one of %q, %q",
			c.Provider, ProviderNone, ProviderAzureAD)
	}
}

func (c Config) NewCredential() (azcore.TokenCredential, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}

	switch c.ProviderName() {
	case ProviderAzureAD:
		return c.AzureAD.NewCredential()
	case ProviderNone:
		return nil, errors.New("cloud_auth is not enabled")
	default:
		return nil, fmt.Errorf("cloud_auth.provider %q is invalid", c.Provider)
	}
}
