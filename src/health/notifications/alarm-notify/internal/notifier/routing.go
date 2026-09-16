// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"errors"
	"strings"
)

func (cfg Config) validateRouting() error {
	validateTargets := func(names []string) error {
		for _, name := range names {
			if _, ok := cfg.Destinations[name]; !ok {
				return errors.New("routing references an unconfigured destination")
			}
		}
		return nil
	}
	if err := validateTargets(cfg.Routing.Default); err != nil {
		return err
	}
	for role, names := range cfg.Routing.Roles {
		if strings.TrimSpace(role) == "" {
			return errors.New("routing role name must not be empty")
		}
		if reservedRole(role) {
			return errors.New("silent and disabled are reserved roles and cannot be configured")
		}
		if names == nil {
			return errors.New(
				"routing role requires a destination list; use [] to suppress delivery",
			)
		}
		if err := validateTargets(names); err != nil {
			return err
		}
	}
	return nil
}

func selectDestinations(cfg Config, destination string, roles []string) ([]string, error) {
	if destination != "" {
		if _, ok := cfg.Destinations[destination]; !ok {
			return nil, errors.New("selected destination is not configured")
		}
		return []string{destination}, nil
	}
	var selected []string
	seen := make(map[string]bool)
	for _, role := range roles {
		if reservedRole(role) {
			continue
		}
		names, ok := cfg.Routing.Roles[role]
		if !ok {
			names = cfg.Routing.Default
		}
		for _, name := range names {
			if !seen[name] {
				selected = append(selected, name)
				seen[name] = true
			}
		}
	}
	return selected, nil
}

func reservedRole(role string) bool {
	return role == "silent" || role == "disabled"
}
