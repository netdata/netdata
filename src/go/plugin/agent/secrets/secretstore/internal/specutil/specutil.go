// SPDX-License-Identifier: GPL-3.0-or-later

package specutil

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore/internal/envx"
)

var reStoreName = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

func NormalizeStoreName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("store name is required")
	}
	if !reStoreName.MatchString(name) {
		return "", fmt.Errorf("store name '%s' must match %s", name, reStoreName.String())
	}
	return name, nil
}

func ValidateStoreKind(key string, kind, expected secretstore.StoreKind) error {
	if !kind.IsValid() {
		return fmt.Errorf("store '%s': invalid kind '%s'", key, kind)
	}
	if kind != expected {
		return fmt.Errorf("store '%s': invalid kind '%s'", key, kind)
	}
	return nil
}

func RequireEnvSelector(v, path string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", fmt.Errorf("%s is required", path)
	}
	if err := envx.ValidateSelector(v, path); err != nil {
		return "", err
	}
	return v, nil
}

func OptionalEnvSelector(v, path string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", nil
	}
	if err := envx.ValidateSelector(v, path); err != nil {
		return "", err
	}
	return v, nil
}
