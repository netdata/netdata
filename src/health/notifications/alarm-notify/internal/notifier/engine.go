// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"context"
	"errors"
	"strings"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
)

// Sender represents one configured destination. Success means provider acceptance.
type Sender interface {
	Send(context.Context, event.Event) error
}

// Plan contains validated senders and the provider-independent routing rules.
type Plan struct {
	Destinations map[string]Sender
	Routing      Routing
}

type Result struct {
	Destination string
	SkipReason  string
	Err         error
}

// Deliver validates all selected policies before attempting destinations sequentially.
func (p Plan) Deliver(ctx context.Context, names []string, notification Notification, report func(Result)) error {
	// Missing required history must not leave an invocation partially delivered.
	skips := make([]string, len(names))
	for i, name := range names {
		if err := ctx.Err(); err != nil {
			return err
		}
		reason, err := p.Routing.Policies[name].SkipReason(notification)
		if err != nil {
			return err
		}
		skips[i] = reason
	}
	succeeded, attempted := 0, 0
	for i, name := range names {
		if err := ctx.Err(); err != nil {
			return err
		}
		if reason := skips[i]; reason != "" {
			report(Result{Destination: name, SkipReason: reason})
			continue
		}
		attempted++
		sender, ok := p.Destinations[name]
		if !ok {
			return errors.New("selected destination is not configured")
		}
		err := sender.Send(ctx, notification.Event)
		report(Result{Destination: name, Err: err})
		if err == nil {
			succeeded++
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if attempted > 0 && succeeded == 0 {
		return errors.New("all attempted destinations failed")
	}
	return nil
}

func (cfg Plan) Validate() error {
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
	for name, policy := range cfg.Routing.Policies {
		if err := validateTargets([]string{name}); err != nil {
			return err
		}
		if policy == nil {
			return errors.New("routing policy requires a mapping; use {} for no filters")
		}
	}
	return nil
}

func (cfg Plan) Select(destination string, roles []string) ([]string, error) {
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
