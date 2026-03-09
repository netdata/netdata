// SPDX-License-Identifier: GPL-3.0-or-later

package cloudauth

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

const (
	AzureADAuthModeServicePrincipal = "service_principal"
	AzureADAuthModeManagedIdentity  = "managed_identity"
	AzureADAuthModeDefault          = "default"
)

type AzureADAuthConfig struct {
	Mode                    string `yaml:"mode,omitempty" json:"mode,omitempty"`
	TenantID                string `yaml:"tenant_id,omitempty" json:"tenant_id,omitempty"`
	ClientID                string `yaml:"client_id,omitempty" json:"client_id,omitempty"`
	ClientSecret            string `yaml:"client_secret,omitempty" json:"client_secret,omitempty"`
	ManagedIdentityClientID string `yaml:"managed_identity_client_id,omitempty" json:"managed_identity_client_id,omitempty"`
}

func (c AzureADAuthConfig) NormalizedMode() string {
	mode := strings.TrimSpace(c.Mode)
	if mode == "" {
		return AzureADAuthModeDefault
	}
	return strings.ToLower(mode)
}

func (c AzureADAuthConfig) Validate() error {
	switch c.NormalizedMode() {
	case AzureADAuthModeServicePrincipal:
		var errs []error
		if strings.TrimSpace(c.TenantID) == "" {
			errs = append(errs, errors.New("cloud_auth.azure_ad.tenant_id is required for service_principal mode"))
		}
		if strings.TrimSpace(c.ClientID) == "" {
			errs = append(errs, errors.New("cloud_auth.azure_ad.client_id is required for service_principal mode"))
		}
		if strings.TrimSpace(c.ClientSecret) == "" {
			errs = append(errs, errors.New("cloud_auth.azure_ad.client_secret is required for service_principal mode"))
		}
		return errors.Join(errs...)
	case AzureADAuthModeManagedIdentity, AzureADAuthModeDefault:
		return nil
	default:
		return fmt.Errorf("cloud_auth.azure_ad.mode %q is invalid: expected one of %q, %q, %q",
			c.Mode, AzureADAuthModeServicePrincipal, AzureADAuthModeManagedIdentity, AzureADAuthModeDefault)
	}
}

func (c AzureADAuthConfig) NewCredential() (azcore.TokenCredential, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}

	switch c.NormalizedMode() {
	case AzureADAuthModeServicePrincipal:
		return azidentity.NewClientSecretCredential(
			strings.TrimSpace(c.TenantID),
			strings.TrimSpace(c.ClientID),
			strings.TrimSpace(c.ClientSecret),
			nil,
		)
	case AzureADAuthModeManagedIdentity:
		if strings.TrimSpace(c.ManagedIdentityClientID) != "" {
			opts := &azidentity.ManagedIdentityCredentialOptions{
				ID: azidentity.ClientID(strings.TrimSpace(c.ManagedIdentityClientID)),
			}
			return azidentity.NewManagedIdentityCredential(opts)
		}
		return azidentity.NewManagedIdentityCredential(nil)
	case AzureADAuthModeDefault:
		return azidentity.NewDefaultAzureCredential(nil)
	default:
		return nil, fmt.Errorf("cloud_auth.azure_ad.mode %q is invalid", c.Mode)
	}
}
