// SPDX-License-Identifier: GPL-3.0-or-later

package azure

import (
	"context"
	_ "embed"
	"net/http"
	"regexp"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore/internal/httpx"
)

var (
	//go:embed config_schema.json
	configSchema    string
	reAzureSafeName = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)
)

type Config struct {
	Mode                string                     `json:"mode" yaml:"mode"`
	ModeClient          *ModeClientConfig          `json:"mode_client,omitempty" yaml:"mode_client,omitempty"`
	ModeManagedIdentity *ModeManagedIdentityConfig `json:"mode_managed_identity,omitempty" yaml:"mode_managed_identity,omitempty"`
}

type ModeClientConfig struct {
	TenantID     string `json:"tenant_id" yaml:"tenant_id"`
	ClientID     string `json:"client_id" yaml:"client_id"`
	ClientSecret string `json:"client_secret" yaml:"client_secret"`
}

type ModeManagedIdentityConfig struct {
	ClientID string `json:"client_id,omitempty" yaml:"client_id,omitempty"`
}

type provider struct {
	apiClient        *http.Client
	imdsClient       *http.Client
	loginEndpointURL string
}

type store struct {
	Config    `yaml:",inline" json:""`
	provider  *provider
	published *publishedStore
}

type publishedStore struct {
	provider                *provider
	mode                    string
	clientTenantID          string
	clientID                string
	clientSecret            string
	managedIdentityClientID string
}

func New() secretstore.Creator {
	p := &provider{
		apiClient:  httpx.APIClient(10 * time.Second),
		imdsClient: httpx.NoProxyClient(2 * time.Second),
	}

	return secretstore.Creator{
		Kind:        secretstore.KindAzureKV,
		DisplayName: "Azure Key Vault",
		Schema:      configSchema,
		Create:      p.create,
	}
}

func (p *provider) create() secretstore.Store {
	return &store{provider: p}
}

func (s *store) Configuration() any { return &s.Config }

func (s *store) Init(ctx context.Context) error { return s.init(ctx) }

func (s *store) Publish() secretstore.PublishedStore { return s.published }
