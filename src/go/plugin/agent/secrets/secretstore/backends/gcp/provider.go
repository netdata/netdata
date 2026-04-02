// SPDX-License-Identifier: GPL-3.0-or-later

package gcp

import (
	"context"
	_ "embed"
	"net/http"
	"regexp"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
)

var (
	//go:embed config_schema.json
	configSchema       string
	reGCPSafeProjectID = regexp.MustCompile(`^[a-zA-Z0-9._:-]+$`)
	reGCPSafeName      = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
)

var defaultTimeout = confopt.Duration(3 * time.Second)

type Config struct {
	Mode                   string                        `json:"mode" yaml:"mode"`
	ModeServiceAccountFile *ModeServiceAccountFileConfig `json:"mode_service_account_file,omitempty" yaml:"mode_service_account_file,omitempty"`
	Timeout                confopt.Duration              `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

type ModeServiceAccountFileConfig struct {
	Path string `json:"path" yaml:"path"`
}

type runtime struct {
	apiClient      *http.Client
	metadataClient *http.Client
}

type store struct {
	Config    `yaml:",inline" json:""`
	runtime   *runtime
	published *publishedStore
}

type publishedStore struct {
	runtime                *runtime
	mode                   string
	serviceAccountFilePath string
}

func New() secretstore.Creator {
	return secretstore.Creator{
		Kind:        secretstore.KindGCPSM,
		DisplayName: "Google Secret Manager",
		Schema:      configSchema,
		Create: func() secretstore.Store {
			return &store{
				Config: Config{
					Timeout: defaultTimeout,
				},
			}
		},
	}
}

func (s *store) Configuration() any { return &s.Config }

func (s *store) Init(ctx context.Context) error { return s.init(ctx) }

func (s *store) Publish() secretstore.PublishedStore { return s.published }
