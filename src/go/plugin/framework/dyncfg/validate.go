// SPDX-License-Identifier: GPL-3.0-or-later

package dyncfg

import (
	"errors"
	"fmt"
	"unicode"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
)

// JobNameRuleStrict rejects characters that cannot be emitted safely as an
// unquoted plugins.d protocol field, plus dots.
// Use for collector job names, which must not conflict with dyncfg template/job
// ID separators (':') or module hierarchy ('.').
func JobNameRuleStrict(name string) error {
	if err := rejectUnsafeJobName(name); err != nil {
		return err
	}
	for _, r := range name {
		if r == '.' {
			return fmt.Errorf("contains '%c'", r)
		}
	}
	return nil
}

// JobNameRuleAllowDots rejects characters that cannot be emitted safely as an
// unquoted plugins.d protocol field but allows dots.
// Use for service discovery, vnode, and secretstore names where dotted identifiers
// are legitimate (e.g. hostnames, FQDNs).
func JobNameRuleAllowDots(name string) error {
	return rejectUnsafeJobName(name)
}

func rejectUnsafeJobName(name string) error {
	for _, r := range name {
		if unicode.IsSpace(r) {
			return errors.New("contains spaces")
		}
		if r < ' ' || r == 0x7f {
			return errors.New("contains control characters")
		}
		switch r {
		case ':', '=', '\'', '"', '\\':
			return fmt.Errorf("contains '%c'", r)
		}
	}
	return nil
}

// ValidBareProtocolField reports whether value can be emitted as one unquoted
// plugins.d protocol field.
//
// Deprecated: use netdataapi.ValidBareProtocolField.
func ValidBareProtocolField(value string) bool {
	return netdataapi.ValidBareProtocolField(value)
}

// ValidSingleQuotedProtocolField reports whether value can be emitted inside
// one single-quoted plugins.d protocol field.
//
// Deprecated: use netdataapi.ValidSingleQuotedProtocolField.
func ValidSingleQuotedProtocolField(value string) bool {
	return netdataapi.ValidSingleQuotedProtocolField(value)
}
