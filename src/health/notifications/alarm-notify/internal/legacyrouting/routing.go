// SPDX-License-Identifier: GPL-3.0-or-later

// Package legacyrouting resolves evaluated legacy recipient settings without
// constructing senders, resolving credentials or executing configuration code.
package legacyrouting

import (
	"errors"
	"fmt"
	"strings"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/legacyconfig"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/notifier"
)

// Target identifies an eligible recipient within a legacy notification method.
// Recipient can contain credentials and must not be used as a diagnostic label.
type Target struct {
	Method    string
	Recipient string
}

type candidate struct {
	target  Target
	allowed bool
	err     error
}

// Resolve selects recipients for the supplied applicable methods, in method/role
// first-appearance order. Methods are canonical suffixes such as email, pd or sms;
// provider support, SEND_* flags and prerequisites are the caller's responsibility.
// Only selected role entries (or their fallback defaults) are inspected. Errors
// return no partial targets. Settings, roles and notification history are not mutated.
func Resolve(settings legacyconfig.Settings, methods, roles []string, notification notifier.Notification) ([]Target, error) {
	switch notification.Event.Status {
	case "WARNING", "CRITICAL", "CLEAR":
	default:
		return nil, errors.New("legacy routing requires WARNING, CRITICAL or CLEAR status")
	}

	var selectedRoles []string
	seenRoles := make(map[string]bool)
	for _, list := range roles {
		for role := range strings.FieldsFuncSeq(list, separator) {
			if role != "silent" && role != "disabled" && !seenRoles[role] {
				selectedRoles = append(selectedRoles, role)
				seenRoles[role] = true
			}
		}
	}

	var candidates []candidate
	seen := make(map[Target]int)
	for methodIndex, method := range methods {
		entries := settings.Recipients["role_recipients_"+method]
		fallback := settings.Variables["DEFAULT_RECIPIENT_"+strings.ToUpper(method)]
		for _, role := range selectedRoles {
			list := entries[role]
			if list == "" {
				list = fallback
			}
			for token := range strings.FieldsFuncSeq(list, separator) {
				if token == "disabled" {
					continue
				}
				recipient, policy, err := parseRecipient(token)
				if err != nil {
					return nil, fmt.Errorf("legacy routing method %d: %w", methodIndex+1, err)
				}
				target := Target{Method: method, Recipient: recipient}
				index, ok := seen[target]
				if !ok {
					index = len(candidates)
					seen[target] = index
					candidates = append(candidates, candidate{target: target})
				}
				reason, err := policy.SkipReason(notification)
				if err != nil {
					candidates[index].err = err
				} else if reason == "" {
					candidates[index].allowed = true
				}
			}
		}
	}

	var targets []Target
	for _, candidate := range candidates {
		// Any permitting occurrence settles the union, even if another needs history.
		if candidate.allowed {
			targets = append(targets, candidate.target)
		} else if candidate.err != nil {
			return nil, fmt.Errorf("legacy routing: %w", candidate.err)
		}
	}
	return targets, nil
}

func separator(r rune) bool {
	// Match Bash's default IFS plus the legacy comma replacement, not Unicode space.
	return r == ',' || r == ' ' || r == '\t' || r == '\n'
}

func parseRecipient(token string) (string, notifier.DestinationPolicy, error) {
	recipient, modifiers, _ := strings.Cut(token, "|")
	var policy notifier.DestinationPolicy
	if recipient == "" {
		return "", policy, errors.New("recipient must not be empty before modifiers")
	}
	for modifier := range strings.SplitSeq(modifiers, "|") {
		switch strings.ToLower(modifier) {
		case "": // Bash ignores empty modifier segments.
		case "critical":
			policy.Critical = true
		case "nowarn":
			policy.NoWarn = true
		case "noclear":
			policy.NoClear = true
		default:
			return "", policy, errors.New("unknown recipient modifier; expected critical, nowarn or noclear")
		}
	}
	return recipient, policy, nil
}
