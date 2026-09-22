// SPDX-License-Identifier: GPL-3.0-or-later

package smstools3

import (
	"bytes"
	"context"
	"maps"
	"strconv"
	"strings"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/commandexec"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config/field"
	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
)

type Config struct {
	Executable string            `yaml:"executable,omitempty"`
	Env        map[string]string `yaml:"env,omitempty"`
	To         string            `yaml:"to,omitempty"`
}

type Sender struct {
	config Config
	runner *commandexec.Runner
}

func New(config Config, runner *commandexec.Runner) (*Sender, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	config.Env = maps.Clone(config.Env)
	return &Sender{config: config, runner: runner}, nil
}

func (dst Config) validate() error {
	if err := commandexec.ValidateOptions(dst.Executable, nil, dst.Env); err != nil {
		return err
	}
	return field.PhoneNumber(dst.To, "smstools3", "to")
}

func (s *Sender) Send(ctx context.Context, event notifyevent.Event) error {
	dst := s.config
	env, err := commandexec.Environment(ctx, dst.Env)
	if err != nil {
		return err
	}
	return s.runner.Run(ctx, dst.Executable, []string{dst.To, renderSMSTools3(event)}, env, bytes.NewReader(nil))
}

func renderSMSTools3(event notifyevent.Event) string {
	status := "needs attention"
	switch event.Status {
	case "CRITICAL":
		status = "is critical"
	case "CLEAR":
		status = "recovered"
	}
	text := event.Node + " " + status + ": "
	if event.Chart != "" {
		text += event.Chart + ", "
	}
	text += strings.ReplaceAll(event.Summary, "_", " ")
	if event.Status != "CLEAR" && event.Value != nil {
		text += " = " + strconv.FormatFloat(*event.Value, 'g', -1, 64)
		if event.Units != "" {
			text += " " + event.Units
		}
	}
	count := 0
	for index := range text {
		if count == 160 {
			return text[:index]
		}
		count++
	}
	return text
}
