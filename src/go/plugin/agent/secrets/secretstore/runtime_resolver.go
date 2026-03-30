// SPDX-License-Identifier: GPL-3.0-or-later

package secretstore

import (
	"context"
	"fmt"
	"strings"
)

// runtimeResolver resolves ${store:<kind>:<name>:<operand>} references against a captured snapshot.
type runtimeResolver struct{}

func newRuntimeResolver() *runtimeResolver { return &runtimeResolver{} }

func (r *runtimeResolver) resolveContext(ctx context.Context, snapshot *Snapshot, ref, original string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	kindPart, rest, ok := strings.Cut(ref, ":")
	if !ok {
		return "", fmt.Errorf("resolving secret '%s': store reference must be in format 'kind:name:operand'", original)
	}
	namePart, operand, ok := strings.Cut(rest, ":")
	if !ok {
		return "", fmt.Errorf("resolving secret '%s': store reference must be in format 'kind:name:operand'", original)
	}

	kind := StoreKind(strings.TrimSpace(kindPart))
	name := strings.TrimSpace(namePart)
	operand = strings.TrimSpace(operand)
	if !kind.IsValid() || name == "" || operand == "" {
		return "", fmt.Errorf("resolving secret '%s': store reference must be in format 'kind:name:operand'", original)
	}
	storeKey := StoreKey(kind, name)

	if snapshot == nil {
		return "", fmt.Errorf("resolving secret '%s': secretstore '%s' is not configured", original, storeKey)
	}
	store, ok := snapshot.lookupStore(storeKey)
	if !ok {
		return "", fmt.Errorf("resolving secret '%s': secretstore '%s' is not configured", original, storeKey)
	}
	if store.published == nil {
		return "", fmt.Errorf("resolving secret '%s': secretstore '%s' has no published resolver state", original, storeKey)
	}

	value, err := store.published.Resolve(ctx, ResolveRequest{
		StoreKey:  storeKey,
		StoreKind: kind,
		StoreName: name,
		Operand:   operand,
		Original:  original,
	})
	if err != nil {
		return "", err
	}
	return value, nil
}
