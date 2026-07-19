// SPDX-License-Identifier: GPL-3.0-or-later

package secretresolver

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

const (
	MaximumAtomicDepth         = 128
	MaximumAtomicResolvedBytes = 20 * 1024 * 1024
)

type AtomicErrorKind string

const (
	AtomicErrorCycle       AtomicErrorKind = "cycle"
	AtomicErrorDepth       AtomicErrorKind = "depth"
	AtomicErrorResultLimit AtomicErrorKind = "result-limit"
	AtomicErrorReference   AtomicErrorKind = "reference"
	AtomicErrorProvider    AtomicErrorKind = "provider"
	AtomicErrorScope       AtomicErrorKind = "scope"
	AtomicErrorShape       AtomicErrorKind = "shape"
)

type AtomicResolveError struct {
	Kind  AtomicErrorKind
	Cause error
}

func (err *AtomicResolveError) Error() string {
	if err == nil {
		return "secret resolver: unknown atomic failure"
	}
	message := "secret resolver: atomic failure"
	switch err.Kind {
	case AtomicErrorCycle:
		message = "secret resolver: container cycle"
	case AtomicErrorDepth:
		message = "secret resolver: maximum container depth exceeded"
	case AtomicErrorResultLimit:
		message = "secret resolver: resolved result exceeds maximum size"
	case AtomicErrorReference:
		message = "secret resolver: invalid secret reference"
	case AtomicErrorProvider:
		message = "secret resolver: provider failure"
	case AtomicErrorScope:
		message = "secret resolver: Store scope failure"
	case AtomicErrorShape:
		message = "secret resolver: unsupported mutable value"
	}
	if err.Cause != nil {
		return message + ": " + err.Cause.Error()
	}
	return message
}

func (err *AtomicResolveError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

type AtomicProvider interface {
	Resolve(context.Context, string) ([]byte, error)
}

type AtomicProviderFunc func(context.Context, string) ([]byte, error)

func (function AtomicProviderFunc) Resolve(ctx context.Context, operand string) ([]byte, error) {
	return function(ctx, operand)
}

type AtomicScope interface {
	Resolve(context.Context, string, string) ([]byte, error)
	Release(context.Context) error
}

type AtomicScopeAcquirer func([]string) (AtomicScope, error)

// AtomicResolver clones, validates, scopes, and resolves one value without
// exposing a partial postimage.
type AtomicResolver struct {
	providers map[string]AtomicProvider
}

func NewAtomicResolver(providers map[string]AtomicProvider) (*AtomicResolver, error) {
	cloned := make(map[string]AtomicProvider, len(providers))
	for scheme, provider := range providers {
		if scheme == "" || scheme == "store" || provider == nil ||
			strings.TrimSpace(scheme) != scheme || strings.ContainsAny(scheme, "+:") {
			return nil, errors.New("secret resolver: invalid atomic provider")
		}
		cloned[scheme] = provider
	}
	return &AtomicResolver{providers: cloned}, nil
}

// NewDefaultAtomicResolver preserves the shipped env/file/cmd schemes behind
// the new atomic call boundary.
func NewDefaultAtomicResolver() (*AtomicResolver, error) {
	legacy := New()
	return NewAtomicResolver(map[string]AtomicProvider{
		"env": AtomicProviderFunc(func(ctx context.Context, operand string) ([]byte, error) {
			value, err := legacy.resolveEnv(ctx, operand, "${env:"+operand+"}")
			return []byte(value), err
		}),
		"file": AtomicProviderFunc(func(ctx context.Context, operand string) ([]byte, error) {
			value, err := legacy.resolveFile(ctx, operand, "${file:"+operand+"}")
			return []byte(value), err
		}),
		"cmd": AtomicProviderFunc(func(ctx context.Context, operand string) ([]byte, error) {
			value, err := legacy.resolveCmd(ctx, operand, "${cmd:"+operand+"}")
			return []byte(value), err
		}),
	})
}

// StoreReferences returns the normalized SecretStore keys referenced by one
// configuration value. It uses the same bounded compiler as Resolve, so
// dependency indexing and runtime resolution cannot disagree on syntax,
// internal-field exclusions, depth, cycles, or size limits.
func StoreReferences(input any) ([]string, error) {
	compiler := atomicCompiler{
		active:    make(map[atomicContainerIdentity]struct{}),
		storeKeys: make(map[string]struct{}),
	}
	if _, err := compiler.clone(input, 0, true); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(compiler.storeKeys))
	for key := range compiler.storeKeys {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}

// EstimateRetainedBytes validates and conservatively estimates one cloned
// value without resolving references.
func EstimateRetainedBytes(input any) (int64, error) {
	compiler := atomicCompiler{
		active: make(map[atomicContainerIdentity]struct{}),
	}
	if _, err := compiler.clone(input, 0, false); err != nil {
		return 0, err
	}
	if compiler.resultBytes <= 0 {
		return 1, nil
	}
	return int64(compiler.resultBytes), nil
}

func (resolver *AtomicResolver) Resolve(
	ctx context.Context,
	input any,
	acquire AtomicScopeAcquirer,
) (any, error) {
	if resolver == nil {
		return nil, errors.New("secret resolver: nil atomic resolver")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	compiler := atomicCompiler{
		resolver: resolver, active: make(map[atomicContainerIdentity]struct{}),
		storeKeys: make(map[string]struct{}),
	}
	cloned, err := compiler.clone(input, 0, true)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(compiler.storeKeys))
	for key := range compiler.storeKeys {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var scope AtomicScope
	if len(keys) != 0 {
		if acquire == nil {
			return nil, &AtomicResolveError{
				Kind: AtomicErrorScope, Cause: errors.New("no Store scope acquirer"),
			}
		}
		scope, err = callAtomicAcquire(acquire, keys)
		if err != nil {
			if scope != nil {
				err = errors.Join(err, callAtomicRelease(ctx, scope))
			}
			return nil, &AtomicResolveError{Kind: AtomicErrorScope, Cause: err}
		}
		if scope == nil {
			return nil, &AtomicResolveError{
				Kind: AtomicErrorScope, Cause: errors.New("acquirer returned nil scope"),
			}
		}
	}

	call := atomicResolveCall{
		resolver: resolver, ctx: ctx, scope: scope,
		resolvedBytes: compiler.resultBytes,
	}
	resolved, resolveErr := call.resolveValue(cloned, true)
	var releaseErr error
	if scope != nil {
		releaseErr = callAtomicRelease(ctx, scope)
		if releaseErr != nil {
			releaseErr = &AtomicResolveError{Kind: AtomicErrorScope, Cause: releaseErr}
		}
	}
	if resolveErr != nil || releaseErr != nil {
		return nil, errors.Join(resolveErr, releaseErr)
	}
	return resolved, nil
}

func callAtomicAcquire(
	acquire AtomicScopeAcquirer,
	keys []string,
) (scope AtomicScope, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			scope = nil
			err = fmt.Errorf("Store scope acquire panic: %v", recovered)
		}
	}()
	return acquire(append([]string(nil), keys...))
}

func callAtomicRelease(ctx context.Context, scope AtomicScope) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("Store scope release panic: %v", recovered)
		}
	}()
	return scope.Release(context.WithoutCancel(ctx))
}

type atomicContainerIdentity struct {
	kind    reflect.Kind
	pointer uintptr
}

type atomicCompiler struct {
	resolver    *AtomicResolver
	active      map[atomicContainerIdentity]struct{}
	storeKeys   map[string]struct{}
	resultBytes int
}

const (
	atomicContainerBytes = 16
	atomicMemberBytes    = 16
	atomicScalarBytes    = 8
)

func (compiler *atomicCompiler) clone(
	value any,
	depth int,
	resolveReferences bool,
) (any, error) {
	switch typed := value.(type) {
	case map[string]any:
		if typed == nil {
			return map[string]any(nil), nil
		}
		if depth > MaximumAtomicDepth {
			return nil, &AtomicResolveError{Kind: AtomicErrorDepth}
		}
		identity := atomicContainerIdentity{
			kind: reflect.Map, pointer: reflect.ValueOf(typed).Pointer(),
		}
		if err := compiler.enter(identity); err != nil {
			return nil, err
		}
		defer compiler.leave(identity)
		if err := compiler.addContainer(len(typed)); err != nil {
			return nil, err
		}
		cloned := make(map[string]any, len(typed))
		for key, member := range typed {
			if err := compiler.addResultBytes(len(key)); err != nil {
				return nil, err
			}
			child, err := compiler.clone(
				member,
				depth+1,
				resolveReferences && !atomicInternalKey(key),
			)
			if err != nil {
				return nil, err
			}
			cloned[key] = child
		}
		return cloned, nil
	case map[any]any:
		if typed == nil {
			return map[any]any(nil), nil
		}
		if depth > MaximumAtomicDepth {
			return nil, &AtomicResolveError{Kind: AtomicErrorDepth}
		}
		identity := atomicContainerIdentity{
			kind: reflect.Map, pointer: reflect.ValueOf(typed).Pointer(),
		}
		if err := compiler.enter(identity); err != nil {
			return nil, err
		}
		defer compiler.leave(identity)
		if err := compiler.addContainer(len(typed)); err != nil {
			return nil, err
		}
		cloned := make(map[any]any, len(typed))
		for key, member := range typed {
			resolveChild := resolveReferences
			if text, ok := key.(string); ok {
				if err := compiler.addResultBytes(len(text)); err != nil {
					return nil, err
				}
				if atomicInternalKey(text) {
					resolveChild = false
				}
			} else if err := compiler.addResultBytes(atomicScalarBytes); err != nil {
				return nil, err
			}
			child, err := compiler.clone(member, depth+1, resolveChild)
			if err != nil {
				return nil, err
			}
			cloned[key] = child
		}
		return cloned, nil
	case []any:
		if typed == nil {
			return []any(nil), nil
		}
		if depth > MaximumAtomicDepth {
			return nil, &AtomicResolveError{Kind: AtomicErrorDepth}
		}
		identity := atomicContainerIdentity{
			kind: reflect.Slice, pointer: reflect.ValueOf(typed).Pointer(),
		}
		if len(typed) != 0 {
			if err := compiler.enter(identity); err != nil {
				return nil, err
			}
			defer compiler.leave(identity)
		}
		if err := compiler.addContainer(len(typed)); err != nil {
			return nil, err
		}
		cloned := make([]any, len(typed))
		for index, member := range typed {
			child, err := compiler.clone(member, depth+1, resolveReferences)
			if err != nil {
				return nil, err
			}
			cloned[index] = child
		}
		return cloned, nil
	case map[string]string:
		if typed == nil {
			return map[string]string(nil), nil
		}
		if depth > MaximumAtomicDepth {
			return nil, &AtomicResolveError{Kind: AtomicErrorDepth}
		}
		identity := atomicContainerIdentity{
			kind: reflect.Map, pointer: reflect.ValueOf(typed).Pointer(),
		}
		if err := compiler.enter(identity); err != nil {
			return nil, err
		}
		defer compiler.leave(identity)
		if err := compiler.addContainer(len(typed)); err != nil {
			return nil, err
		}
		cloned := make(map[string]string, len(typed))
		for key, member := range typed {
			if err := compiler.addResultBytes(len(key)); err != nil {
				return nil, err
			}
			if resolveReferences && !atomicInternalKey(key) {
				if err := compiler.compileReferences(member); err != nil {
					return nil, err
				}
			} else if err := compiler.addResultBytes(len(member)); err != nil {
				return nil, err
			}
			cloned[key] = member
		}
		return cloned, nil
	case []string:
		if typed == nil {
			return []string(nil), nil
		}
		if depth > MaximumAtomicDepth {
			return nil, &AtomicResolveError{Kind: AtomicErrorDepth}
		}
		identity := atomicContainerIdentity{
			kind: reflect.Slice, pointer: reflect.ValueOf(typed).Pointer(),
		}
		if len(typed) != 0 {
			if err := compiler.enter(identity); err != nil {
				return nil, err
			}
			defer compiler.leave(identity)
		}
		if err := compiler.addContainer(len(typed)); err != nil {
			return nil, err
		}
		cloned := make([]string, len(typed))
		for index, member := range typed {
			if resolveReferences {
				if err := compiler.compileReferences(member); err != nil {
					return nil, err
				}
			} else if err := compiler.addResultBytes(len(member)); err != nil {
				return nil, err
			}
			cloned[index] = member
		}
		return cloned, nil
	case []byte:
		if err := compiler.addResultBytes(atomicContainerBytes); err != nil {
			return nil, err
		}
		if err := compiler.addResultBytes(len(typed)); err != nil {
			return nil, err
		}
		return append([]byte(nil), typed...), nil
	case string:
		if resolveReferences {
			if err := compiler.compileReferences(typed); err != nil {
				return nil, err
			}
		} else if err := compiler.addResultBytes(len(typed)); err != nil {
			return nil, err
		}
		return typed, nil
	default:
		if value == nil {
			if err := compiler.addResultBytes(1); err != nil {
				return nil, err
			}
			return nil, nil
		}
		switch reflect.TypeOf(value).Kind() {
		case reflect.Map, reflect.Slice, reflect.Pointer:
			return nil, &AtomicResolveError{Kind: AtomicErrorShape}
		}
		if err := compiler.addResultBytes(atomicScalarBytes); err != nil {
			return nil, err
		}
		return value, nil
	}
}

func (compiler *atomicCompiler) enter(identity atomicContainerIdentity) error {
	if _, ok := compiler.active[identity]; ok {
		return &AtomicResolveError{Kind: AtomicErrorCycle}
	}
	compiler.active[identity] = struct{}{}
	return nil
}

func (compiler *atomicCompiler) leave(identity atomicContainerIdentity) {
	delete(compiler.active, identity)
}

func (compiler *atomicCompiler) compileReferences(value string) error {
	last := 0
	err := visitAtomicReferenceSpans(value, func(start, end int, reference atomicReference) error {
		if !reference.active {
			return nil
		}
		if err := compiler.addResultBytes(start - last); err != nil {
			return err
		}
		last = end
		if reference.scheme == "store" {
			compiler.storeKeys[reference.storeKey] = struct{}{}
			return nil
		}
		if compiler.resolver.providers[reference.scheme] == nil {
			return &AtomicResolveError{
				Kind:  AtomicErrorReference,
				Cause: boundedAtomicSchemeError("unknown provider", reference.scheme),
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return compiler.addResultBytes(len(value) - last)
}

func (compiler *atomicCompiler) addContainer(members int) error {
	if members < 0 || members > (MaximumAtomicResolvedBytes-atomicContainerBytes)/atomicMemberBytes {
		return &AtomicResolveError{Kind: AtomicErrorResultLimit}
	}
	return compiler.addResultBytes(atomicContainerBytes + members*atomicMemberBytes)
}

func (compiler *atomicCompiler) addResultBytes(size int) error {
	if size < 0 || size > MaximumAtomicResolvedBytes-compiler.resultBytes {
		return &AtomicResolveError{Kind: AtomicErrorResultLimit}
	}
	compiler.resultBytes += size
	return nil
}

func atomicInternalKey(key string) bool {
	return len(key) >= 4 && key[0:2] == "__" && key[len(key)-2:] == "__"
}
