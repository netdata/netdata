// SPDX-License-Identifier: GPL-3.0-or-later

package aws

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
	AuthMode string           `json:"auth_mode" yaml:"auth_mode"`
	Region   string           `json:"region" yaml:"region"`
	Timeout  confopt.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

type credentials struct {
	accessKeyID     string
	secretAccessKey string
	sessionToken    string
}

type provider struct {
	apiClient  *http.Client
	imdsClient *http.Client
	endpoint   string
	now        func() time.Time
}

type store struct {
	Config    `yaml:",inline" json:""`
	provider  *provider
	published *publishedStore
}

type publishedStore struct {
	provider    *provider
	mode        string
	regionValue string
}

func New() secretstore.Creator {
	p := &provider{
		apiClient:  httpx.APIClient(defaultTimeout.Duration()),
		imdsClient: httpx.NoProxyClient(defaultTimeout.Duration()),
		now:        time.Now,
	}

	return secretstore.Creator{
		Kind:        secretstore.KindAWSSM,
		DisplayName: "AWS Secrets Manager",
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
			endpoint:   p.endpoint,
			now:        p.now,
		},
	}
}

func (s *store) Configuration() any { return &s.Config }

func (s *store) Init(ctx context.Context) error { return s.init(ctx) }

func (s *store) Publish() secretstore.PublishedStore { return s.published }
