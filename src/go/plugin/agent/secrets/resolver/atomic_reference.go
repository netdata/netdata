// SPDX-License-Identifier: GPL-3.0-or-later

package secretresolver

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type atomicReference struct {
	active    bool
	scheme    string
	modifier  string
	operand   string
	storeKey  string
	secretKey string
}

type atomicResolveCall struct {
	resolver      *AtomicResolver
	ctx           context.Context
	scope         AtomicScope
	resolvedBytes int
}

var errAtomicResultLimit = errors.New("resolved result limit")

func (call *atomicResolveCall) resolveValue(value any, resolveReferences bool) (any, error) {
	if err := call.ctx.Err(); err != nil {
		return nil, err
	}
	switch typed := value.(type) {
	case map[string]any:
		for key, member := range typed {
			resolved, err := call.resolveValue(member, resolveReferences && !atomicInternalKey(key))
			if err != nil {
				return nil, err
			}
			typed[key] = resolved
		}
		return typed, nil
	case map[any]any:
		for key, member := range typed {
			resolveChild := resolveReferences
			if text, ok := key.(string); ok && atomicInternalKey(text) {
				resolveChild = false
			}
			resolved, err := call.resolveValue(member, resolveChild)
			if err != nil {
				return nil, err
			}
			typed[key] = resolved
		}
		return typed, nil
	case []any:
		for index, member := range typed {
			resolved, err := call.resolveValue(member, resolveReferences)
			if err != nil {
				return nil, err
			}
			typed[index] = resolved
		}
		return typed, nil
	case map[string]string:
		for key, member := range typed {
			if !resolveReferences || atomicInternalKey(key) {
				continue
			}
			resolved, err := call.resolveString(member)
			if err != nil {
				return nil, err
			}
			typed[key] = resolved
		}
		return typed, nil
	case []string:
		for index, member := range typed {
			if !resolveReferences {
				continue
			}
			resolved, err := call.resolveString(member)
			if err != nil {
				return nil, err
			}
			typed[index] = resolved
		}
		return typed, nil
	case string:
		if !resolveReferences {
			return typed, nil
		}
		return call.resolveString(typed)
	default:
		return value, nil
	}
}

func (call *atomicResolveCall) resolveString(value string) (string, error) {
	var builder strings.Builder
	changed := false
	last := 0
	err := visitAtomicReferenceSpans(
		value,
		func(start, end int, reference atomicReference) error {
			if !reference.active {
				return nil
			}
			remaining := MaximumAtomicResolvedBytes - call.resolvedBytes
			var resolved []byte
			var err error
			if reference.scheme == "store" {
				if call.scope == nil {
					return &AtomicResolveError{
						Kind:  AtomicErrorScope,
						Cause: errors.New("Store reference has no scope"),
					}
				}
				resolved, err = callAtomicScopeResolve(
					call.ctx,
					call.scope,
					reference.storeKey,
					reference.secretKey,
					remaining,
				)
			} else {
				provider := call.resolver.providers[reference.scheme]
				if provider == nil {
					return &AtomicResolveError{
						Kind:  AtomicErrorReference,
						Cause: boundedAtomicSchemeError("unknown provider", reference.scheme),
					}
				}
				resolved, err = callAtomicProviderResolve(
					call.ctx,
					provider,
					reference.operand,
					remaining,
				)
			}
			if err != nil {
				if errors.Is(err, errAtomicResultLimit) {
					return &AtomicResolveError{Kind: AtomicErrorResultLimit}
				}
				return &AtomicResolveError{Kind: AtomicErrorProvider, Cause: err}
			}
			if err := call.ctx.Err(); err != nil {
				return err
			}
			text, err := applyAtomicModifier(reference.modifier, resolved, remaining)
			if err != nil {
				return &AtomicResolveError{Kind: AtomicErrorResultLimit}
			}
			call.resolvedBytes += len(text)
			if !changed {
				builder.Grow(len(value))
				builder.WriteString(value[:start])
				changed = true
			} else {
				builder.WriteString(value[last:start])
			}
			builder.WriteString(text)
			last = end
			return nil
		},
	)
	if err != nil {
		return "", err
	}
	if !changed {
		return value, nil
	}
	builder.WriteString(value[last:])
	return builder.String(), nil
}

func visitAtomicReferences(value string, visit func(atomicReference) error) error {
	return visitAtomicReferenceSpans(
		value,
		func(_, _ int, reference atomicReference) error {
			return visit(reference)
		},
	)
}

func visitAtomicReferenceSpans(
	value string,
	visit func(int, int, atomicReference) error,
) error {
	for offset := 0; offset < len(value); {
		openRelative := strings.Index(value[offset:], "${")
		if openRelative < 0 {
			return nil
		}
		open := offset + openRelative
		closeRelative := strings.IndexByte(value[open+2:], '}')
		if closeRelative < 0 {
			return nil
		}
		closeIndex := open + 2 + closeRelative
		reference, err := parseAtomicReference(value[open+2 : closeIndex])
		if err != nil {
			return err
		}
		if err := visit(open, closeIndex+1, reference); err != nil {
			return err
		}
		offset = closeIndex + 1
	}
	return nil
}

func parseAtomicReference(value string) (atomicReference, error) {
	token, operand, ok := strings.Cut(value, ":")
	if !ok {
		return atomicReference{}, nil
	}
	scheme, modifier, hasModifier := strings.Cut(token, "+")
	if scheme == "" {
		return atomicReference{}, &AtomicResolveError{
			Kind: AtomicErrorReference, Cause: errors.New("empty provider scheme"),
		}
	}
	if hasModifier && modifier != modifierURIEncode {
		return atomicReference{}, &AtomicResolveError{
			Kind: AtomicErrorReference, Cause: errors.New("unknown output modifier"),
		}
	}
	reference := atomicReference{
		active: true, scheme: scheme, modifier: modifier, operand: operand,
	}
	if scheme != "store" {
		return reference, nil
	}
	kind, remainder, ok := strings.Cut(operand, ":")
	if !ok {
		return atomicReference{}, invalidAtomicStoreReference()
	}
	name, secretKey, ok := strings.Cut(remainder, ":")
	kind = strings.TrimSpace(kind)
	name = strings.TrimSpace(name)
	secretKey = strings.TrimSpace(secretKey)
	if !ok || kind == "" || name == "" || secretKey == "" {
		return atomicReference{}, invalidAtomicStoreReference()
	}
	reference.storeKey = kind + ":" + name
	reference.secretKey = secretKey
	return reference, nil
}

func invalidAtomicStoreReference() error {
	return &AtomicResolveError{
		Kind:  AtomicErrorReference,
		Cause: errors.New("Store reference must be kind:name:operand"),
	}
}

func boundedAtomicSchemeError(message, scheme string) error {
	const maximum = 64
	if len(scheme) > maximum {
		scheme = scheme[:maximum]
	}
	return fmt.Errorf("%s %q", message, scheme)
}

func callAtomicProviderResolve(
	ctx context.Context,
	provider AtomicProvider,
	key string,
	maximum int,
) (value []byte, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			value = nil
			err = fmt.Errorf("provider Resolve panic: %v", recovered)
		}
	}()
	value, err = provider.Resolve(ctx, key)
	if err != nil {
		return nil, err
	}
	if len(value) > maximum {
		return nil, errAtomicResultLimit
	}
	return append([]byte(nil), value...), nil
}

func callAtomicScopeResolve(
	ctx context.Context,
	scope AtomicScope,
	storeKey string,
	secretKey string,
	maximum int,
) (value []byte, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			value = nil
			err = fmt.Errorf("Store Resolve panic: %v", recovered)
		}
	}()
	value, err = scope.Resolve(ctx, storeKey, secretKey)
	if err != nil {
		return nil, err
	}
	if len(value) > maximum {
		return nil, errAtomicResultLimit
	}
	return append([]byte(nil), value...), nil
}

func applyAtomicModifier(modifier string, value []byte, maximum int) (string, error) {
	if modifier != modifierURIEncode {
		if len(value) > maximum {
			return "", errAtomicResultLimit
		}
		return string(value), nil
	}
	encoded := 0
	for _, char := range value {
		step := 1
		if !isURIUnreserved(char) {
			step = 3
		}
		if step > maximum-encoded {
			return "", errAtomicResultLimit
		}
		encoded += step
	}
	return percentEncodeURIUnreserved(string(value)), nil
}
