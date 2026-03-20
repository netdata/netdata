// SPDX-License-Identifier: GPL-3.0-or-later

package secretresolver

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

type StoreRefResolver func(ctx context.Context, ref, original string) (string, error)

// reSecretRef matches ${...} references in strings.
var reSecretRef = regexp.MustCompile(`\$\{([^}]+)\}`)

// Resolve walks the config map and resolves all secret references in string values.
func (r *Resolver) Resolve(cfg map[string]any) error {
	return r.ResolveWithStoreResolver(context.Background(), cfg, nil)
}

func (r *Resolver) ResolveWithStoreResolver(ctx context.Context, cfg map[string]any, storeResolver StoreRefResolver) error {
	if r == nil {
		return fmt.Errorf("secret resolver is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r.ensureDefaults()
	return r.resolveMap(ctx, cfg, storeResolver)
}

func (r *Resolver) resolveMap(ctx context.Context, m map[string]any, storeResolver StoreRefResolver) error {
	for k, v := range m {
		// skip internal keys (__key__)
		if isInternalKey(k) {
			continue
		}
		resolved, err := r.resolveValue(ctx, v, storeResolver)
		if err != nil {
			return err
		}
		m[k] = resolved
	}
	return nil
}

func (r *Resolver) resolveMapAny(ctx context.Context, m map[any]any, storeResolver StoreRefResolver) error {
	for k, v := range m {
		s, ok := k.(string)
		if ok && isInternalKey(s) {
			continue
		}
		resolved, err := r.resolveValue(ctx, v, storeResolver)
		if err != nil {
			return err
		}
		m[k] = resolved
	}
	return nil
}

func (r *Resolver) resolveSlice(ctx context.Context, s []any, storeResolver StoreRefResolver) error {
	for i, v := range s {
		resolved, err := r.resolveValue(ctx, v, storeResolver)
		if err != nil {
			return err
		}
		s[i] = resolved
	}
	return nil
}

func (r *Resolver) resolveValue(ctx context.Context, v any, storeResolver StoreRefResolver) (any, error) {
	switch val := v.(type) {
	case string:
		return r.resolveString(ctx, val, storeResolver)
	case map[string]any:
		return val, r.resolveMap(ctx, val, storeResolver)
	case map[any]any:
		return val, r.resolveMapAny(ctx, val, storeResolver)
	case []any:
		return val, r.resolveSlice(ctx, val, storeResolver)
	default:
		return v, nil
	}
}

func (r *Resolver) resolveString(ctx context.Context, s string, storeResolver StoreRefResolver) (string, error) {
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

		val, err := r.resolveRef(ctx, inner, match, storeResolver)
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

func (r *Resolver) resolveRef(ctx context.Context, ref, original string, storeResolver StoreRefResolver) (string, error) {
	scheme, name, hasScheme := strings.Cut(ref, ":")

	if !hasScheme {
		// no scheme — leave unchanged
		return original, nil
	}

	if scheme == "store" {
		if storeResolver == nil {
			return "", fmt.Errorf("resolving secret '%s': secretstore resolver is not configured", original)
		}
		return storeResolver(ctx, name, original)
	}

	provider, ok := r.providers[scheme]
	if !ok {
		return "", fmt.Errorf("resolving secret '%s': unknown secret provider '%s'", original, scheme)
	}

	return provider(ctx, name, original)
}

func isInternalKey(k string) bool {
	return strings.HasPrefix(k, "__") && strings.HasSuffix(k, "__")
}
