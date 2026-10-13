// SPDX-License-Identifier: GPL-3.0-or-later

package commandexec

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/secret"
)

const DefaultPath = "/usr/local/bin:/usr/bin:/bin"

func ValidateOptions(executable string, args []string, env map[string]string) error {
	if !filepath.IsAbs(executable) || strings.ContainsAny(executable, "\x00\r\n") || strings.Contains(executable, "${") {
		return errors.New("executable must be a literal absolute path without NUL or line breaks")
	}
	for _, arg := range args {
		if strings.ContainsRune(arg, 0) {
			return errors.New("command args must not contain NUL")
		}
	}
	for name, value := range env {
		if !ValidEnvironmentName(name) {
			return errors.New("command env names must use letters, digits and underscores, without a leading digit")
		}
		if _, err := secret.IsReference(value); err != nil {
			return fmt.Errorf("command env: %w", err)
		}
		if strings.ContainsRune(value, 0) {
			return errors.New("command env values must not contain NUL")
		}
	}
	return nil
}

func ValidHost(host string) bool {
	if strings.HasPrefix(host, "-") || strings.IndexFunc(host, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) != -1 {
		return false
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		// ParseAddr accepts arbitrary zone text; apply the host character policy to it too.
		host = addr.Zone()
	}
	return !strings.ContainsAny(host, ":/\\[]@?#%${}")
}

func ValidEnvironmentName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if r != '_' && !(r >= 'A' && r <= 'Z') && !(r >= 'a' && r <= 'z') && !(i > 0 && r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func Environment(ctx context.Context, configured map[string]string) ([]string, error) {
	values := map[string]string{"PATH": DefaultPath}
	for name, raw := range configured {
		value, err := secret.Resolve(ctx, raw)
		if err != nil {
			return nil, fmt.Errorf("command env: %w", err)
		}
		if strings.ContainsRune(value, 0) {
			return nil, errors.New("command env values must not contain NUL")
		}
		values[name] = value
	}
	env := make([]string, 0, len(values))
	for name, value := range values {
		env = append(env, name+"="+value)
	}
	sort.Strings(env)
	return env, nil
}
