// SPDX-License-Identifier: GPL-3.0-or-later

package opsgenie

import (
	"net/http"
)

type Config struct {
	APIKey string `yaml:"api_key,omitempty"`
	APIURL string `yaml:"api_url,omitempty"`
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
