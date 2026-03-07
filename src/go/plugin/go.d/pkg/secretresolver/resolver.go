// SPDX-License-Identifier: GPL-3.0-or-later

package secretresolver

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// reSecretRef matches ${...} references in strings.
var reSecretRef = regexp.MustCompile(`\$\{([^}]+)\}`)

// reUpperEnvVar matches uppercase environment variable names (shorthand form).
var reUpperEnvVar = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

// Resolve walks the config map and resolves all secret references in string values.
func Resolve(cfg map[string]any) error {
	return resolveMap(cfg)
}

func resolveMap(m map[string]any) error {
	for k, v := range m {
		// skip internal keys (__key__)
		if isInternalKey(k) {
			continue
		}
		resolved, err := resolveValue(v)
		if err != nil {
			return err
		}
		m[k] = resolved
	}
	return nil
}

func resolveMapAny(m map[any]any) error {
	for k, v := range m {
		s, ok := k.(string)
		if ok && isInternalKey(s) {
			continue
		}
		resolved, err := resolveValue(v)
		if err != nil {
			return err
		}
		m[k] = resolved
	}
	return nil
}

func resolveSlice(s []any) error {
	for i, v := range s {
		resolved, err := resolveValue(v)
		if err != nil {
			return err
		}
		s[i] = resolved
	}
	return nil
}

func resolveValue(v any) (any, error) {
	switch val := v.(type) {
	case string:
		return resolveString(val)
	case map[string]any:
		return val, resolveMap(val)
	case map[any]any:
		return val, resolveMapAny(val)
	case []any:
		return val, resolveSlice(val)
	default:
		return v, nil
	}
}

func resolveString(s string) (string, error) {
	if !strings.Contains(s, "${") {
		return s, nil
	}

	var resolveErr error

	result := reSecretRef.ReplaceAllStringFunc(s, func(match string) string {
		if resolveErr != nil {
			return match
		}

		// extract inner content between ${ and }
		inner := match[2 : len(match)-1]

		val, err := resolveRef(inner, match)
		if err != nil {
			resolveErr = err
			return match
		}
		return val
	})

	if resolveErr != nil {
		return "", resolveErr
	}
	return result, nil
}

func resolveRef(ref, original string) (string, error) {
	scheme, name, hasScheme := strings.Cut(ref, ":")

	if !hasScheme {
		// no scheme — only resolve if it looks like an uppercase env var
		if reUpperEnvVar.MatchString(ref) {
			return resolveEnv(ref, original)
		}
		// not an uppercase env var pattern — leave unchanged
		return original, nil
	}

	switch scheme {
	case "env":
		return resolveEnv(name, original)
	case "file":
		return resolveFile(name, original)
	default:
		return "", fmt.Errorf("resolving secret '%s': unknown secret provider '%s'", original, scheme)
	}
}

func resolveEnv(name, original string) (string, error) {
	val, ok := os.LookupEnv(name)
	if !ok {
		return "", fmt.Errorf("resolving secret '%s': environment variable '%s' is not set", original, name)
	}
	return val, nil
}

func resolveFile(path, original string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': %w", original, err)
	}
	return strings.TrimSpace(string(data)), nil
}

func isInternalKey(k string) bool {
	return strings.HasPrefix(k, "__") && strings.HasSuffix(k, "__")
}
