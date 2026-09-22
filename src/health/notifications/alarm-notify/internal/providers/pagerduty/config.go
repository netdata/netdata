// SPDX-License-Identifier: GPL-3.0-or-later

package pagerduty

import (
	"net/http"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/secret"

	configfield "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config/field"
)

type Config struct {
	Secrets        secret.InputMode     `yaml:"-"`
	IntegrationKey string               `yaml:"integration_key,omitempty"`
	APIURL         string               `yaml:"api_url,omitempty"`
	APIVersion     *configfield.Integer `yaml:"api_version,omitempty"`
}

type Sender struct {
	cfg    Config
	client *http.Client
}

func New(cfg Config, client *http.Client) (*Sender, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if cfg.APIVersion != nil {
		v := *cfg.APIVersion
		cfg.APIVersion = &v
	}
	return &Sender{cfg: cfg, client: client}, nil
}
