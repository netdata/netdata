// SPDX-License-Identifier: GPL-3.0-or-later

package gcp

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
	configSchema       string
	reGCPSafeProjectID = regexp.MustCompile(`^[a-zA-Z0-9._:-]+$`)
	reGCPSafeName      = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
)

type Config struct {
	Mode                   string                        `json:"mode" yaml:"mode"`
	ModeServiceAccountFile *ModeServiceAccountFileConfig `json:"mode_service_account_file,omitempty" yaml:"mode_service_account_file,omitempty"`
}

type ModeServiceAccountFileConfig struct {
	Path string `json:"path" yaml:"path"`
}

type provider struct {
	apiClient      *http.Client
	metadataClient *http.Client
	secretEndpoint string
	now            func() time.Time
}

type store struct {
	Config    `yaml:",inline" json:""`
	provider  *provider
	published *publishedStore
}

type publishedStore struct {
	provider               *provider
	mode                   string
	serviceAccountFilePath string
}

func New() secretstore.Creator {
	p := &provider{
		apiClient:      httpx.APIClient(10 * time.Second),
		metadataClient: httpx.NoProxyClient(2 * time.Second),
		now:            time.Now,
	}

	return secretstore.Creator{
		Kind:        secretstore.KindGCPSM,
		DisplayName: "Google Secret Manager",
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
