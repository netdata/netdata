// SPDX-License-Identifier: GPL-3.0-or-later

package dyncfg

import (
	"errors"
	"fmt"
	"unicode"
)

// JobNameRuleStrict rejects spaces, dots, and colons.
// Use for collector job names, which must not conflict with dyncfg template/job
// ID separators (':') or module hierarchy ('.').
func JobNameRuleStrict(name string) error {
	if err := rejectSpacesAndColons(name); err != nil {
		return err
	}
	for _, r := range name {
		if r == '.' {
			return fmt.Errorf("contains '%c'", r)
		}
	}
	return nil
}

// JobNameRuleAllowDots rejects spaces and colons but allows dots.
// Use for service discovery, vnode, and secretstore names where dotted identifiers
// are legitimate (e.g. hostnames, FQDNs).
func JobNameRuleAllowDots(name string) error {
	return rejectSpacesAndColons(name)
}

func rejectSpacesAndColons(name string) error {
	for _, r := range name {
		if unicode.IsSpace(r) {
			return errors.New("contains spaces")
		}
		if r == ':' {
			return fmt.Errorf("contains '%c'", r)
		}
	}
	return nil
}
