// SPDX-License-Identifier: GPL-3.0-or-later

package kafka

import (
	"net/http"
)

type Config struct {
	URL      string `yaml:"url,omitempty"`
	SenderIP string `yaml:"sender_ip,omitempty"`
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
