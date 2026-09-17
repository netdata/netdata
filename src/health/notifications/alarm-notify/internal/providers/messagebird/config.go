// SPDX-License-Identifier: GPL-3.0-or-later

package messagebird

import (
	"net/http"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/secret"
)

type Config struct {
	Secrets    secret.InputMode `yaml:"-"`
	AccessKey  string           `yaml:"access_key,omitempty"`
	Originator string           `yaml:"originator,omitempty"`
	Recipient  string           `yaml:"recipient,omitempty"`
	APIURL     string           `yaml:"api_url,omitempty"`
}

type Sender struct {
	cfg    Config
	client *http.Client
}

func New(cfg Config, client *http.Client) (*Sender, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &Sender{cfg: cfg, client: client}, nil
}
