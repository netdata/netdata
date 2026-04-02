// SPDX-License-Identifier: GPL-3.0-or-later

package azure

import (
	"context"
	_ "embed"
	"net/http"
	"regexp"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore/internal/httpx"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/cloudauth"
)

var (
	//go:embed config_schema.json
	configSchema    string
	reAzureSafeName = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)
)

var defaultTimeout = confopt.Duration(3 * time.Second)

type Config struct {
	cloudauth.AzureADAuthConfig `yaml:",inline" json:",inline"`
	Timeout                     confopt.Duration `yaml:"timeout,omitempty" json:"timeout,omitempty"`
}

type provider struct {
	apiClient  *http.Client
	imdsClient *http.Client
}

type store struct {
	Config    `yaml:",inline" json:""`
	provider  *provider
	published *publishedStore
}

type publishedStore struct {
	provider      *provider
	tokenProvider *cloudauth.TokenProvider
}

func New() secretstore.Creator {
	p := &provider{
		apiClient:  httpx.APIClient(defaultTimeout.Duration()),
		imdsClient: httpx.NoProxyClient(defaultTimeout.Duration()),
	}

	return secretstore.Creator{
		Kind:        secretstore.KindAzureKV,
		DisplayName: "Azure Key Vault",
		Schema:      configSchema,
		Create:      p.create,
	}
}

func (p *provider) create() secretstore.Store {
	return &store{
		Config: Config{
			Timeout: defaultTimeout,
		},
		provider: &provider{
			apiClient:  httpx.CloneClientWithTimeout(p.apiClient, defaultTimeout.Duration()),
			imdsClient: httpx.CloneClientWithTimeout(p.imdsClient, defaultTimeout.Duration()),
		},
	}
}

func (s *store) Configuration() any { return &s.Config }

func (s *store) Init(ctx context.Context) error { return s.init(ctx) }

func (s *store) Publish() secretstore.PublishedStore { return s.published }
