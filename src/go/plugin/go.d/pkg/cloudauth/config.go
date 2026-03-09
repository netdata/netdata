// SPDX-License-Identifier: GPL-3.0-or-later

package cloudauth

import (
	"errors"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
)

type Config struct {
	Provider Provider           `yaml:"provider" json:"provider"`
	AzureAD  *AzureADAuthConfig `yaml:"azure_ad,omitempty" json:"azure_ad,omitempty"`
}

func (c Config) ProviderName() Provider {
	return c.Provider.Normalized()
}

func (c Config) IsProvider(provider Provider) bool {
	return c.ProviderName() == provider.Normalized()
}

func (c Config) IsEnabled() bool {
	return c.ProviderName() != ProviderNone
}

func (c Config) Validate() error {
	switch c.ProviderName() {
	case ProviderNone:
		return nil
	case ProviderAzureAD:
		return c.azureADConfig().Validate()
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
		return c.azureADConfig().NewCredential()
	case ProviderNone:
		return nil, errors.New("cloud_auth is not enabled")
	default:
		return nil, fmt.Errorf("cloud_auth.provider %q is invalid", c.Provider)
	}
}

func (c Config) azureADConfig() AzureADAuthConfig {
	if c.AzureAD == nil {
		return AzureADAuthConfig{}
	}
	return *c.AzureAD
}
