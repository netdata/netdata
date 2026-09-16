// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode"
)

func (dst Destination) validateSyslog() error {
	allowed := Destination{Type: dst.Type, Executable: dst.Executable, Args: dst.Args, Env: dst.Env,
		Facility: dst.Facility, Level: dst.Level, Prefix: dst.Prefix, Host: dst.Host, Port: dst.Port}
	if !reflect.DeepEqual(dst, allowed) {
		return errors.New("syslog destination contains fields for another provider")
	}
	command := Destination{Type: "command", Executable: dst.Executable, Args: dst.Args, Env: dst.Env}
	if err := command.validateCommand(); err != nil {
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
	if dst.Host != "" && !validSyslogHost(dst.Host) {
		return errors.New("syslog host must be a hostname or unbracketed IP address without whitespace or options")
	}
	if dst.Port != nil && (dst.Host == "" || *dst.Port < 1 || *dst.Port > 65535) {
		return errors.New("syslog port requires host and must be an integer from 1 to 65535")
	}
	return nil
}

func validSyslogHost(host string) bool {
	if strings.HasPrefix(host, "-") || strings.IndexFunc(host, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) != -1 {
		return false
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return true
	}
	return !strings.ContainsAny(host, ":/\\[]@?#%${}")
}

func renderSyslog(dst Destination, event Event) ([]string, error) {
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
	return append(args, "--", escapeSyslogControls(text)), nil
}

// Keep message controls out of newline-framed logs and terminal displays.
func escapeSyslogControls(text string) string {
	var escaped strings.Builder
	escaped.Grow(len(text))
	for _, r := range text {
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' {
			quoted := strconv.QuoteRune(r)
			escaped.WriteString(quoted[1 : len(quoted)-1])
		} else {
			escaped.WriteRune(r)
		}
	}
	return escaped.String()
}

func sendSyslog(ctx context.Context, processes *commandProcesses, dst Destination, event Event) error {
	args, err := renderSyslog(dst, event)
	if err != nil {
		return err
	}
	env, err := commandEnvironment(ctx, dst.Env)
	if err != nil {
		return err
	}
	return processes.run(ctx, dst.Executable, args, env, nil)
}
