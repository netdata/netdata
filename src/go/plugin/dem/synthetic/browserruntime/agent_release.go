// SPDX-License-Identifier: GPL-3.0-or-later

package browserruntime

import (
	"errors"
	"strings"
)

// AgentRelease identifies the Agent source and release lane paired with a runtime.
// Native package versions and execution-contract versions have separate owners.
type AgentRelease struct {
	Version string `json:"version"`
	Channel string `json:"channel"`
	Source  string `json:"source"`
}

func (a AgentRelease) Validate() error {
	// This identity is also a single URL path segment, shared with release-info.
	if len(a.Version) == 0 || len(a.Version) > 128 {
		return errors.New("Agent release version must be a path-safe identifier of 1 to 128 ASCII characters")
	}
	for i := range len(a.Version) {
		c := a.Version[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
			continue
		}
		if i > 0 && (c == '.' || c == '_' || c == '-') {
			continue
		}
		return errors.New(
			"Agent release version must start with an ASCII letter or digit and contain only letters, digits, '.', '_' or '-'",
		)
	}
	switch a.Channel {
	case "stable", "release-candidate", "nightly", "devel":
	default:
		return errors.New("Agent release channel must be stable, release-candidate, nightly or devel")
	}
	if len(a.Source) != 40 || strings.Trim(a.Source, "0123456789abcdef") != "" {
		return errors.New("Agent release source must be a full lowercase Git commit SHA")
	}
	return nil
}
