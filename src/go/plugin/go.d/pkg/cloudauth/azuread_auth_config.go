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

	azureADAuthConfigPath = "cloud_auth.azure_ad"
)

type AzureADModeServicePrincipalConfig struct {
	TenantID     string `yaml:"tenant_id,omitempty" json:"tenant_id,omitempty"`
	ClientID     string `yaml:"client_id,omitempty" json:"client_id,omitempty"`
	ClientSecret string `yaml:"client_secret,omitempty" json:"client_secret,omitempty"`
}

type AzureADModeManagedIdentityConfig struct {
	ClientID string `yaml:"client_id,omitempty" json:"client_id,omitempty"`
}

type AzureADAuthConfig struct {
	Mode                 string                             `yaml:"mode,omitempty" json:"mode,omitempty"`
	ModeServicePrincipal *AzureADModeServicePrincipalConfig `yaml:"mode_service_principal,omitempty" json:"mode_service_principal,omitempty"`
	ModeManagedIdentity  *AzureADModeManagedIdentityConfig  `yaml:"mode_managed_identity,omitempty" json:"mode_managed_identity,omitempty"`
}

type AzureADCredentialOptions struct {
	ClientOptions azcore.ClientOptions
}

func (c AzureADAuthConfig) NormalizedMode() string {
	mode := strings.TrimSpace(c.Mode)
	if mode == "" {
		return AzureADAuthModeDefault
	}
	return strings.ToLower(mode)
}

func (c AzureADAuthConfig) Validate() error {
	return c.ValidateWithPath(azureADAuthConfigPath)
}

func (c AzureADAuthConfig) ValidateWithPath(path string) error {
	modeField := fieldPath(path, "mode")

	switch c.NormalizedMode() {
	case AzureADAuthModeServicePrincipal:
		var errs []error
		if c.ModeServicePrincipal == nil {
			return fmt.Errorf("%s is required when %s is %q", fieldPath(path, "mode_service_principal"), modeField, AzureADAuthModeServicePrincipal)
		}

		if strings.TrimSpace(c.ModeServicePrincipal.TenantID) == "" {
			errs = append(errs, errors.New(fieldPath(path, "mode_service_principal.tenant_id")+" is required"))
		}
		if strings.TrimSpace(c.ModeServicePrincipal.ClientID) == "" {
			errs = append(errs, errors.New(fieldPath(path, "mode_service_principal.client_id")+" is required"))
		}
		if strings.TrimSpace(c.ModeServicePrincipal.ClientSecret) == "" {
			errs = append(errs, errors.New(fieldPath(path, "mode_service_principal.client_secret")+" is required"))
		}
		return errors.Join(errs...)
	case AzureADAuthModeManagedIdentity, AzureADAuthModeDefault:
		return nil
	default:
		return fmt.Errorf("%s %q is invalid: expected one of %q, %q, %q",
			modeField, c.Mode, AzureADAuthModeServicePrincipal, AzureADAuthModeManagedIdentity, AzureADAuthModeDefault)
	}
}

func (c AzureADAuthConfig) NewCredential() (azcore.TokenCredential, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}

	return c.newCredential(nil)
}

func (c AzureADAuthConfig) NewCredentialWithOptions(opts *AzureADCredentialOptions) (azcore.TokenCredential, error) {
	return c.newCredential(opts)
}

func (c AzureADAuthConfig) newCredential(opts *AzureADCredentialOptions) (azcore.TokenCredential, error) {
	switch c.NormalizedMode() {
	case AzureADAuthModeServicePrincipal:
		cfg := c.servicePrincipalConfig()
		credOpts := &azidentity.ClientSecretCredentialOptions{}
		if opts != nil {
			credOpts.ClientOptions = opts.ClientOptions
		}
		return azidentity.NewClientSecretCredential(
			strings.TrimSpace(cfg.TenantID),
			strings.TrimSpace(cfg.ClientID),
			strings.TrimSpace(cfg.ClientSecret),
			credOpts,
		)
	case AzureADAuthModeManagedIdentity:
		cfg := c.managedIdentityConfig()
		credOpts := &azidentity.ManagedIdentityCredentialOptions{}
		if opts != nil {
			credOpts.ClientOptions = opts.ClientOptions
		}
		if strings.TrimSpace(cfg.ClientID) != "" {
			credOpts.ID = azidentity.ClientID(strings.TrimSpace(cfg.ClientID))
		}
		return azidentity.NewManagedIdentityCredential(credOpts)
	case AzureADAuthModeDefault:
		credOpts := &azidentity.DefaultAzureCredentialOptions{}
		if opts != nil {
			credOpts.ClientOptions = opts.ClientOptions
		}
		return azidentity.NewDefaultAzureCredential(credOpts)
	default:
		return nil, fmt.Errorf("%s %q is invalid", fieldPath(azureADAuthConfigPath, "mode"), c.Mode)
	}
}

func (c AzureADAuthConfig) servicePrincipalConfig() AzureADModeServicePrincipalConfig {
	if c.ModeServicePrincipal == nil {
		return AzureADModeServicePrincipalConfig{}
	}
	return *c.ModeServicePrincipal
}

func (c AzureADAuthConfig) managedIdentityConfig() AzureADModeManagedIdentityConfig {
	if c.ModeManagedIdentity == nil {
		return AzureADModeManagedIdentityConfig{}
	}
	return *c.ModeManagedIdentity
}

func fieldPath(path, field string) string {
	if path == "" {
		return field
	}
	return path + "." + field
}
