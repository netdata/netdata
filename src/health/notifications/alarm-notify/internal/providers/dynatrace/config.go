// SPDX-License-Identifier: GPL-3.0-or-later

package dynatrace

import (
	"net/http"
)

type Config struct {
	APIURL         string `yaml:"api_url,omitempty"`
	APIToken       string `yaml:"api_token,omitempty"`
	EntitySelector string `yaml:"entity_selector,omitempty"`
	EventType      string `yaml:"event_type,omitempty"`
	Source         string `yaml:"source,omitempty"`
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
