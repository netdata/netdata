// SPDX-License-Identifier: GPL-3.0-or-later

package smseagle

import (
	"net/http"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/secret"

	configfield "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config/field"
)

type Config struct {
	Secrets      secret.InputMode     `yaml:"-"`
	APIURL       string               `yaml:"api_url,omitempty"`
	AccessToken  string               `yaml:"access_token,omitempty"`
	Recipients   []string             `yaml:"recipients,omitempty"`
	MessageType  string               `yaml:"message_type,omitempty"`
	CallDuration *configfield.Integer `yaml:"call_duration,omitempty"`
	VoiceID      *configfield.Integer `yaml:"voice_id,omitempty"`
}

type Sender struct {
	cfg    Config
	client *http.Client
}

func New(cfg Config, client *http.Client) (*Sender, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	cfg.Recipients = append([]string(nil), cfg.Recipients...)
	if cfg.CallDuration != nil {
		v := *cfg.CallDuration
		cfg.CallDuration = &v
	}
	if cfg.VoiceID != nil {
		v := *cfg.VoiceID
		cfg.VoiceID = &v
	}
	return &Sender{cfg: cfg, client: client}, nil
}
