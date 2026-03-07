// SPDX-License-Identifier: GPL-3.0-or-later

package azureauth

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

const (
	ModeServicePrincipal = "service_principal"
	ModeManagedIdentity  = "managed_identity"
	ModeDefault          = "default"
)

type Config struct {
	Enabled                 bool   `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	Mode                    string `yaml:"mode,omitempty" json:"mode,omitempty"`
	TenantID                string `yaml:"tenant_id,omitempty" json:"tenant_id,omitempty"`
	ClientID                string `yaml:"client_id,omitempty" json:"client_id,omitempty"`
	ClientSecret            string `yaml:"client_secret,omitempty" json:"client_secret,omitempty"`
	ManagedIdentityClientID string `yaml:"managed_identity_client_id,omitempty" json:"managed_identity_client_id,omitempty"`
}

func (c Config) normalizedMode() string {
	mode := strings.TrimSpace(c.Mode)
	if mode == "" {
		return ModeDefault
	}
	return strings.ToLower(mode)
}

func (c Config) Validate() error {
	if !c.Enabled {
		return nil
	}

	mode := c.normalizedMode()
	switch mode {
	case ModeServicePrincipal:
		var errs []error
		if strings.TrimSpace(c.TenantID) == "" {
			errs = append(errs, errors.New("azure_ad.tenant_id is required for service_principal mode"))
		}
		if strings.TrimSpace(c.ClientID) == "" {
			errs = append(errs, errors.New("azure_ad.client_id is required for service_principal mode"))
		}
		if strings.TrimSpace(c.ClientSecret) == "" {
			errs = append(errs, errors.New("azure_ad.client_secret is required for service_principal mode"))
		}
		return errors.Join(errs...)
	case ModeManagedIdentity, ModeDefault:
		return nil
	default:
		return fmt.Errorf("azure_ad.mode %q is invalid: expected one of %q, %q, %q",
			c.Mode, ModeServicePrincipal, ModeManagedIdentity, ModeDefault)
	}
}

func (c Config) NewCredential() (azcore.TokenCredential, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if !c.Enabled {
		return nil, errors.New("azure_ad is not enabled")
	}

	switch c.normalizedMode() {
	case ModeServicePrincipal:
		return azidentity.NewClientSecretCredential(
			strings.TrimSpace(c.TenantID),
			strings.TrimSpace(c.ClientID),
			strings.TrimSpace(c.ClientSecret),
			nil,
		)
	case ModeManagedIdentity:
		if strings.TrimSpace(c.ManagedIdentityClientID) != "" {
			opts := &azidentity.ManagedIdentityCredentialOptions{
				ID: azidentity.ClientID(strings.TrimSpace(c.ManagedIdentityClientID)),
			}
			return azidentity.NewManagedIdentityCredential(opts)
		}
		return azidentity.NewManagedIdentityCredential(nil)
	case ModeDefault:
		return azidentity.NewDefaultAzureCredential(nil)
	default:
		return nil, fmt.Errorf("azure_ad.mode %q is invalid", c.Mode)
	}
}
