// SPDX-License-Identifier: GPL-3.0-or-later

package legacydelivery

import (
	"errors"
	"net"
	"strings"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/notifier"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/email"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/irc"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/smstools3"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/syslog"
)

func buildEmail(b *builder, recipients []string) ([]notifier.Sender, error) {
	return one(email.New(email.Config{Executable: b.values["sendmail"], From: b.values["EMAIL_SENDER"], Recipients: recipients, PlainTextOnly: new(b.values["EMAIL_PLAINTEXT_ONLY"] == "YES"), Threading: new(b.values["EMAIL_THREADING"] != "NO")}, b.runner))
}
func buildSMS(b *builder, recipients []string) ([]notifier.Sender, error) {
	return each(recipients, func(r string) (notifier.Sender, error) {
		return smstools3.New(smstools3.Config{Executable: b.values["sendsms"], To: r}, b.runner)
	})
}
func buildIRC(b *builder, recipients []string) ([]notifier.Sender, error) {
	port, err := integer(b.values["IRC_PORT"], "IRC_PORT")
	if err != nil {
		return nil, err
	}
	return each(recipients, func(r string) (notifier.Sender, error) {
		return irc.New(irc.Config{Executable: b.values["nc"], Host: b.values["IRC_NETWORK"], Port: port, Nickname: b.values["IRC_NICKNAME"], Realname: b.values["IRC_REALNAME"], Channel: r}, b.runner)
	})
}
func buildSyslog(b *builder, recipients []string) ([]notifier.Sender, error) {
	return each(recipients, func(r string) (notifier.Sender, error) {
		cfg, err := syslogTarget(r)
		if err != nil {
			return nil, err
		}
		cfg.Executable = b.values["logger"]
		cfg.Args = shellFields(b.values["logger_options"])
		if cfg.Facility == "" {
			cfg.Facility = b.values["SYSLOG_FACILITY"]
		}
		return syslog.New(cfg, b.runner)
	})
}

// Parse [[facility.level][@host[:port]]/]prefix without executing shell syntax.
func syslogTarget(target string) (syslog.Config, error) {
	var cfg syslog.Config
	address, prefix, hasAddress := strings.Cut(target, "/")
	if !hasAddress {
		cfg.Prefix = target
		return cfg, nil
	}
	cfg.Prefix = prefix
	if prefix == "" || strings.Contains(prefix, "/") {
		return cfg, errors.New("syslog recipient requires one nonempty prefix")
	}
	priority, remote, hasRemote := strings.Cut(address, "@")
	if priority != "" {
		facility, level, ok := strings.Cut(priority, ".")
		if !ok || facility == "" || level == "" {
			return cfg, errors.New("syslog priority requires facility.level")
		}
		cfg.Facility, cfg.Level = facility, level
	}
	if hasRemote {
		switch {
		case strings.HasPrefix(remote, "[") && strings.HasSuffix(remote, "]"):
			cfg.Host = strings.TrimSuffix(strings.TrimPrefix(remote, "["), "]")
		case strings.Contains(remote, ":"):
			host, port, err := net.SplitHostPort(remote)
			if err != nil || port == "" {
				return cfg, errors.New("syslog remote requires host:port or bracketed IPv6")
			}
			cfg.Host = host
			cfg.Port, err = integer(port, "syslog port")
			if err != nil {
				return cfg, err
			}
		default:
			cfg.Host = remote
		}
		if cfg.Host == "" {
			return cfg, errors.New("syslog remote host must not be empty")
		}
	}
	return cfg, nil
}
