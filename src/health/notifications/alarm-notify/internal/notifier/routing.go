// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"errors"

	"gopkg.in/yaml.v3"
)

type Routing struct {
	Roles    map[string][]string           `yaml:"roles,omitempty"`
	Default  []string                      `yaml:"default,omitempty"`
	Policies map[string]*DestinationPolicy `yaml:"policies,omitempty"`
}

type DestinationPolicy struct {
	Critical bool `yaml:"critical,omitempty"`
	NoWarn   bool `yaml:"nowarn,omitempty"`
	NoClear  bool `yaml:"noclear,omitempty"`
}

// Decode policy flags strictly: nulls and legacy yes/no strings are not booleans.
func (policy *DestinationPolicy) UnmarshalYAML(node *yaml.Node) error {
	var fields map[string]any
	if err := node.Decode(&fields); err != nil {
		return err
	}
	var decoded DestinationPolicy
	for name, value := range fields {
		enabled, ok := value.(bool)
		if !ok {
			return errors.New("routing policy flags must be booleans")
		}
		switch name {
		case "critical":
			decoded.Critical = enabled
		case "nowarn":
			decoded.NoWarn = enabled
		case "noclear":
			decoded.NoClear = enabled
		default:
			return errors.New("unknown routing policy flag")
		}
	}
	*policy = decoded
	return nil
}

func (policy *DestinationPolicy) skipReason(notification Notification) (string, error) {
	if policy == nil {
		return "", nil
	}
	switch notification.Event.Status {
	case "WARNING":
		if policy.NoWarn {
			return "nowarn", nil
		}
	case "CLEAR":
		if policy.NoClear {
			return "noclear", nil
		}
	default:
		return "", nil
	}
	if !policy.Critical {
		return "", nil
	}
	if notification.CriticalSeenSinceClear == nil {
		return "", errors.New("critical_seen_since_clear is required by a selected critical policy")
	}
	if !*notification.CriticalSeenSinceClear {
		return "critical", nil
	}
	return "", nil
}

func reservedRole(role string) bool {
	return role == "silent" || role == "disabled"
}
