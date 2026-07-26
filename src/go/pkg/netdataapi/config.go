// SPDX-License-Identifier: GPL-3.0-or-later

package netdataapi

import "fmt"

// ValidBareProtocolField reports whether value can be emitted as one unquoted
// plugins.d protocol field.
func ValidBareProtocolField(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char <= ' ' || char == 0x7f {
			return false
		}
		switch char {
		case '=', '\'', '"', '\\':
			return false
		}
	}
	return true
}

// ValidSingleQuotedProtocolField reports whether value can be emitted inside
// one single-quoted plugins.d protocol field.
func ValidSingleQuotedProtocolField(value string) bool {
	trailingBackslashes := 0
	for _, char := range value {
		if char < ' ' || char == 0x7f || char == '\'' {
			return false
		}
		if char == '\\' {
			trailingBackslashes++
		} else {
			trailingBackslashes = 0
		}
	}
	return trailingBackslashes%2 == 0
}

// Validate checks whether all CONFIG CREATE fields can be encoded without
// changing plugins.d field boundaries.
func (opts ConfigOpts) Validate() error {
	bareFields := []struct {
		name  string
		value string
	}{
		{name: "id", value: opts.ID},
		{name: "status", value: opts.Status},
		{name: "config type", value: opts.ConfigType},
		{name: "path", value: opts.Path},
		{name: "source type", value: opts.SourceType},
	}
	for _, field := range bareFields {
		if !ValidBareProtocolField(field.value) {
			return fmt.Errorf("netdataapi: invalid CONFIG %s", field.name)
		}
	}
	if !ValidSingleQuotedProtocolField(opts.Source) {
		return fmt.Errorf("netdataapi: invalid CONFIG source")
	}
	if !ValidSingleQuotedProtocolField(opts.SupportedCommands) {
		return fmt.Errorf("netdataapi: invalid CONFIG supported commands")
	}
	return nil
}
