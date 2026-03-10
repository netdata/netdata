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
func (r *Resolver) Resolve(cfg map[string]any) error {
	if r == nil {
		return fmt.Errorf("secret resolver is nil")
	}
	r.ensureDefaults()
	return r.resolveMap(cfg)
}

func (r *Resolver) resolveMap(m map[string]any) error {
	for k, v := range m {
		// skip internal keys (__key__)
		if isInternalKey(k) {
			continue
		}
		resolved, err := r.resolveValue(v)
		if err != nil {
			return err
		}
		m[k] = resolved
	}
	return nil
}

func (r *Resolver) resolveMapAny(m map[any]any) error {
	for k, v := range m {
		s, ok := k.(string)
		if ok && isInternalKey(s) {
			continue
		}
		resolved, err := r.resolveValue(v)
		if err != nil {
			return err
		}
		m[k] = resolved
	}
	return nil
}

func (r *Resolver) resolveSlice(s []any) error {
	for i, v := range s {
		resolved, err := r.resolveValue(v)
		if err != nil {
			return err
		}
		s[i] = resolved
	}
	return nil
}

func (r *Resolver) resolveValue(v any) (any, error) {
	switch val := v.(type) {
	case string:
		return r.resolveString(val)
	case map[string]any:
		return val, r.resolveMap(val)
	case map[any]any:
		return val, r.resolveMapAny(val)
	case []any:
		return val, r.resolveSlice(val)
	default:
		return v, nil
	}
}

func (r *Resolver) resolveString(s string) (string, error) {
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

		val, err := r.resolveRef(inner, match)
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

func (r *Resolver) resolveRef(ref, original string) (string, error) {
	scheme, name, hasScheme := strings.Cut(ref, ":")

	if !hasScheme {
		// no scheme — only resolve if it looks like an uppercase env var
		if reUpperEnvVar.MatchString(ref) {
			return r.resolveEnv(ref, original)
		}
		// not an uppercase env var pattern — leave unchanged
		return original, nil
	}

	provider, ok := r.providers[scheme]
	if !ok {
		return "", fmt.Errorf("resolving secret '%s': unknown secret provider '%s'", original, scheme)
	}

	return provider(name, original)
}

func (r *Resolver) resolveEnv(name, original string) (string, error) {
	val, ok := os.LookupEnv(name)
	if !ok {
		return "", fmt.Errorf("resolving secret '%s': environment variable '%s' is not set", original, name)
	}
	return val, nil
}

func (r *Resolver) resolveFile(path, original string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': %w", original, err)
	}
	return strings.TrimSpace(string(data)), nil
}

func isInternalKey(k string) bool {
	return strings.HasPrefix(k, "__") && strings.HasSuffix(k, "__")
}
