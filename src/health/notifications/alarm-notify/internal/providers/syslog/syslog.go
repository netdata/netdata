// SPDX-License-Identifier: GPL-3.0-or-later

package syslog

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/commandexec"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config/field"
	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	notifymsg "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/message"
)

type Config struct {
	Executable string            `yaml:"executable,omitempty"`
	Args       []string          `yaml:"args,omitempty"`
	Env        map[string]string `yaml:"env,omitempty"`
	Facility   string            `yaml:"facility,omitempty"`
	Level      string            `yaml:"level,omitempty"`
	Prefix     string            `yaml:"prefix,omitempty"`
	Host       string            `yaml:"host,omitempty"`
	Port       *field.Integer    `yaml:"port,omitempty"`
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
	if config.Port != nil {
		config.Port = new(*config.Port)
	}
	return &Sender{config: config, runner: runner}, nil
}

func (dst Config) validate() error {
	if err := commandexec.ValidateOptions(dst.Executable, dst.Args, dst.Env); err != nil {
		return fmt.Errorf("syslog: %w", err)
	}
	switch dst.Facility {
	case "", "auth", "authpriv", "cron", "daemon", "ftp", "kern", "lpr", "mail", "news", "security", "syslog", "user", "uucp",
		"local0", "local1", "local2", "local3", "local4", "local5", "local6", "local7":
	default:
		return errors.New("syslog facility must be a standard lowercase syslog facility name")
	}
	switch dst.Level {
	case "", "emerg", "alert", "crit", "err", "warning", "notice", "info", "debug", "panic", "error", "warn":
	default:
		return errors.New("syslog level must be a standard lowercase syslog severity name")
	}
	if strings.ContainsRune(dst.Prefix, 0) {
		return errors.New("syslog prefix must not contain NUL")
	}
	if dst.Host != "" && !commandexec.ValidHost(dst.Host) {
		return errors.New("syslog host must be a hostname or unbracketed IP address without whitespace or options")
	}
	if dst.Port != nil && (dst.Host == "" || *dst.Port < 1 || *dst.Port > 65535) {
		return errors.New("syslog port requires host and must be an integer from 1 to 65535")
	}
	return nil
}

func renderSyslog(dst Config, event notifyevent.Event) ([]string, error) {
	facility, level, prefix := dst.Facility, dst.Level, dst.Prefix
	if facility == "" {
		facility = "local6"
	}
	if level == "" {
		level = "info"
		switch event.Status {
		case "CRITICAL":
			level = "crit"
		case "WARNING":
			level = "warning"
		}
	}
	if prefix == "" {
		prefix = "netdata"
	}
	text := prefix + " " + event.Status + " on " + event.Node + " at " + event.Timestamp.Format(time.RFC3339) + ":"
	if event.Chart != "" {
		text += " " + event.Chart
	}
	if event.Value != nil {
		text += " " + strconv.FormatFloat(*event.Value, 'g', -1, 64)
		if event.Units != "" {
			text += " " + event.Units
		}
	}
	if strings.ContainsRune(text, 0) {
		return nil, errors.New("syslog message must not contain NUL")
	}
	args := []string{"-p", facility + "." + level}
	if dst.Host != "" {
		args = append(args, "-n", dst.Host)
		if dst.Port != nil {
			args = append(args, "-P", strconv.FormatInt(int64(*dst.Port), 10))
		}
	}
	args = append(args, dst.Args...)
	return append(args, "--", notifymsg.EscapeControls(text)), nil
}

func (s *Sender) Send(ctx context.Context, event notifyevent.Event) error {
	dst, processes := s.config, s.runner
	args, err := renderSyslog(dst, event)
	if err != nil {
		return err
	}
	env, err := commandexec.Environment(ctx, dst.Env)
	if err != nil {
		return err
	}
	return processes.Run(ctx, dst.Executable, args, env, nil)
}
