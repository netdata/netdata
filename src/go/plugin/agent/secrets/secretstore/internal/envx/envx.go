// SPDX-License-Identifier: GPL-3.0-or-later

package envx

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

var reEnvVarID = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func Lookup(name string) (string, bool) {
	v, ok := os.LookupEnv(name)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(v), true
}

func ResolveRequired(selector, fieldPath string) (string, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return "", fmt.Errorf("%s selector is empty", fieldPath)
	}
	val, ok := Lookup(selector)
	if !ok || strings.TrimSpace(val) == "" {
		return "", fmt.Errorf("%s references env '%s' which is not set", fieldPath, selector)
	}
	return strings.TrimSpace(val), nil
}

func ResolveOptional(selector string) (string, bool) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return "", false
	}
	val, ok := Lookup(selector)
	if !ok || strings.TrimSpace(val) == "" {
		return "", false
	}
	return strings.TrimSpace(val), true
}

func ValidateSelector(name, fieldPath string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("%s is required", fieldPath)
	}
	if !reEnvVarID.MatchString(name) {
		return fmt.Errorf("%s must reference an environment variable name", fieldPath)
	}
	return nil
}

func ParseBool(name string) bool {
	v, ok := Lookup(name)
	if !ok {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}
