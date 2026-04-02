// SPDX-License-Identifier: GPL-3.0-or-later

package vault

import (
	"context"
	_ "embed"
	"net/http"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore/internal/httpx"
)

//go:embed config_schema.json
var configSchema string

var defaultTimeout = confopt.Duration(3 * time.Second)

type Config struct {
	Mode          string               `json:"mode" yaml:"mode"`
	ModeToken     *ModeTokenConfig     `json:"mode_token,omitempty" yaml:"mode_token,omitempty"`
	ModeTokenFile *ModeTokenFileConfig `json:"mode_token_file,omitempty" yaml:"mode_token_file,omitempty"`
	Addr          string               `json:"addr" yaml:"addr"`
	Namespace     string               `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	TLSSkipVerify bool                 `json:"tls_skip_verify,omitempty" yaml:"tls_skip_verify,omitempty"`
	Timeout       confopt.Duration     `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

type ModeTokenConfig struct {
	Token string `json:"token" yaml:"token"`
}

type ModeTokenFileConfig struct {
	Path string `json:"path" yaml:"path"`
}

type provider struct {
	httpClient         *http.Client
	httpClientInsecure *http.Client
}

type store struct {
	Config    `yaml:",inline" json:""`
	provider  *provider
	published *publishedStore
}

type publishedStore struct {
	provider       *provider
	mode           string
	tokenValue     string
	tokenFilePath  string
	addr           string
	namespaceValue string
	tlsSkipVerify  bool
}

func New() secretstore.Creator {
	p := &provider{
		httpClient:         httpx.VaultClient(defaultTimeout.Duration()),
		httpClientInsecure: httpx.VaultInsecureClient(defaultTimeout.Duration()),
	}

	return secretstore.Creator{
		Kind:        secretstore.KindVault,
		DisplayName: "Vault",
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
			httpClient:         httpx.CloneClientWithTimeout(p.httpClient, defaultTimeout.Duration()),
			httpClientInsecure: httpx.CloneClientWithTimeout(p.httpClientInsecure, defaultTimeout.Duration()),
		},
	}
}

func (s *store) Configuration() any { return &s.Config }

func (s *store) Init(ctx context.Context) error { return s.init(ctx) }

func (s *store) Publish() secretstore.PublishedStore { return s.published }
