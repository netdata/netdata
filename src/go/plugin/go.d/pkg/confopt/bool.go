// SPDX-License-Identifier: GPL-3.0-or-later

package confopt

import (
	"fmt"
	"strconv"
	"strings"
)

// FlexBool is a boolean that unmarshals from many truthy/falsey forms,
// but marshals as a strict boolean.
type FlexBool bool

// UnmarshalYAML allows accepting yes/no, on/off, 1/0, etc.
// Compatible with github.com/goccy/go-yaml (YAML 1.2).
func (b *FlexBool) UnmarshalYAML(unmarshal func(any) error) error {
	var vb bool
	if err := unmarshal(&vb); err == nil {
		*b = FlexBool(vb)
		return nil
	}

	var vs string
	if err := unmarshal(&vs); err == nil {
		got, ok := parseBoolString(vs)
		if !ok {
			return fmt.Errorf("invalid boolean string %q", vs)
		}
		*b = FlexBool(got)
		return nil
	}

	var vi int
	if err := unmarshal(&vi); err == nil {
		switch vi {
		case 0:
			*b = false
			return nil
		case 1:
			*b = true
			return nil
		default:
			return fmt.Errorf("invalid boolean number %d (want 0 or 1)", vi)
		}
	}

	return fmt.Errorf("invalid boolean value")
}

// MarshalYAML emits a strict boolean (true/false).
func (b FlexBool) MarshalYAML() (any, error) {
	return bool(b), nil
}

func parseBoolString(s string) (bool, bool) {
	v := strings.Trim(strings.TrimSpace(strings.ToLower(s)), "\"'")
	switch v {
	case "true", "t", "yes", "y", "on", "1":
		return true, true
	case "false", "f", "no", "n", "off", "0":
		return false, true
	default:
		if bv, err := strconv.ParseBool(v); err == nil {
			return bv, true
		}
		return false, false
	}
}
