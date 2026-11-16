// SPDX-License-Identifier: GPL-3.0-or-later

package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"maps"
	"slices"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/commandexec"
	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
)

type Config struct {
	Executable string            `yaml:"executable,omitempty"`
	Args       []string          `yaml:"args,omitempty"`
	Env        map[string]string `yaml:"env,omitempty"`
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
	config.Args = slices.Clone(config.Args)
	return &Sender{config: config, runner: runner}, nil
}

func (dst Config) validate() error {
	return commandexec.ValidateOptions(dst.Executable, dst.Args, dst.Env)
}

func (s *Sender) Send(ctx context.Context, event notifyevent.Event) error {
	dst := s.config
	env, err := commandexec.Environment(ctx, dst.Env)
	if err != nil {
		return err
	}
	input, err := json.Marshal(event)
	if err != nil {
		return errors.New("could not encode command event")
	}
	return s.runner.Run(ctx, dst.Executable, dst.Args, env, bytes.NewReader(append(input, '\n')))
}
