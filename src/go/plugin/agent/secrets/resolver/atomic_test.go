// SPDX-License-Identifier: GPL-3.0-or-later

package secretresolver

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestResolverAtomicRollback(t *testing.T) {
	tests := map[string]struct {
		input func() map[string]any
		kind  AtomicErrorKind
	}{
		"map cycle": {
			input: func() map[string]any {
				value := map[string]any{}
				value["self"] = value
				return value
			},
			kind: AtomicErrorCycle,
		},
		"provider failure": {
			input: func() map[string]any {
				return map[string]any{"a": "${test:ok}", "b": "${test:fail}"}
			},
			kind: AtomicErrorProvider,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			resolver, err := NewAtomicResolver(map[string]AtomicProvider{
				"test": AtomicProviderFunc(func(_ context.Context, key string) ([]byte, error) {
					if key == "fail" {
						return nil, errors.New("provider failed")
					}
					return []byte("resolved"), nil
				}),
			})
			if err != nil {
				t.Fatal(err)
			}
			input := test.input()
			before := input["a"]
			resolved, err := resolver.Resolve(context.Background(), input, nil)
			if resolved != nil {
				t.Fatalf("failure returned partial output: %#v", resolved)
			}
			requireAtomicErrorKind(t, err, test.kind)
			if name == "provider failure" && input["a"] != before {
				t.Fatal("provider failure mutated caller input")
			}
		})
	}
}

func TestResolverBounds(t *testing.T) {
	depthTests := map[string]struct {
		edges   int
		wantErr bool
	}{
		"below": {edges: MaximumAtomicDepth - 1},
		"exact": {edges: MaximumAtomicDepth},
		"over":  {edges: MaximumAtomicDepth + 1, wantErr: true},
	}
	resolver, err := NewAtomicResolver(nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, test := range depthTests {
		t.Run("depth "+name, func(t *testing.T) {
			resolved, err := resolver.Resolve(
				context.Background(),
				atomicNestedMap(test.edges),
				nil,
			)
			if test.wantErr {
				if resolved != nil {
					t.Fatal("depth failure returned a result")
				}
				requireAtomicErrorKind(t, err, AtomicErrorDepth)
				return
			}
			if err != nil || resolved == nil {
				t.Fatalf("depth rejected: %v", err)
			}
		})
	}

	sizeTests := map[string]struct {
		size    int
		wantErr bool
	}{
		"exact": {size: MaximumAtomicResolvedBytes},
		"over":  {size: MaximumAtomicResolvedBytes + 1, wantErr: true},
	}
	for name, test := range sizeTests {
		t.Run("result "+name, func(t *testing.T) {
			value := []byte(strings.Repeat("x", test.size))
			resolver, err := NewAtomicResolver(map[string]AtomicProvider{
				"test": AtomicProviderFunc(func(context.Context, string) ([]byte, error) {
					return value, nil
				}),
			})
			if err != nil {
				t.Fatal(err)
			}
			resolved, err := resolver.Resolve(
				context.Background(),
				"${test:key}",
				nil,
			)
			if test.wantErr {
				if resolved != nil {
					t.Fatal("result limit returned a partial result")
				}
				requireAtomicErrorKind(t, err, AtomicErrorResultLimit)
				return
			}
			if err != nil || len(resolved.(string)) != test.size {
				t.Fatalf("exact result rejected: %v", err)
			}
		})
	}

	wholeResultTests := map[string]struct {
		input   func() any
		wantErr bool
	}{
		"plain exact": {
			input: func() any {
				const overhead = atomicContainerBytes + atomicMemberBytes + len("value")
				return map[string]any{
					"value": strings.Repeat("x", MaximumAtomicResolvedBytes-overhead),
				}
			},
		},
		"plain over": {
			input: func() any {
				const overhead = atomicContainerBytes + atomicMemberBytes + len("value")
				return map[string]any{
					"value": strings.Repeat("x", MaximumAtomicResolvedBytes-overhead+1),
				}
			},
			wantErr: true,
		},
		"mixed literal and secret exact": {
			input: func() any {
				return "prefix:${test:key}"
			},
		},
	}
	for name, test := range wholeResultTests {
		t.Run("whole result "+name, func(t *testing.T) {
			resolver, err := NewAtomicResolver(map[string]AtomicProvider{
				"test": AtomicProviderFunc(func(context.Context, string) ([]byte, error) {
					return []byte(strings.Repeat(
						"x",
						MaximumAtomicResolvedBytes-len("prefix:"),
					)), nil
				}),
			})
			if err != nil {
				t.Fatal(err)
			}
			resolved, err := resolver.Resolve(context.Background(), test.input(), nil)
			if test.wantErr {
				if resolved != nil {
					t.Fatal("whole-result limit returned a partial result")
				}
				requireAtomicErrorKind(t, err, AtomicErrorResultLimit)
				return
			}
			if err != nil || resolved == nil {
				t.Fatalf("whole-result exact bound rejected: %v", err)
			}
		})
	}
}

func TestResolverClonesMutableLeaves(t *testing.T) {
	input := map[string]any{
		"bytes":   []byte("secret"),
		"strings": []string{"${test:key}"},
	}
	resolver, err := NewAtomicResolver(map[string]AtomicProvider{
		"test": AtomicProviderFunc(func(context.Context, string) ([]byte, error) {
			return []byte("resolved"), nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := resolver.Resolve(context.Background(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	output := resolved.(map[string]any)
	output["bytes"].([]byte)[0] = 'X'
	output["strings"].([]string)[0] = "changed"
	if string(input["bytes"].([]byte)) != "secret" ||
		input["strings"].([]string)[0] != "${test:key}" {
		t.Fatal("resolved result aliases caller-owned mutable leaves")
	}
}

func TestResolverStoreScopeLinear(t *testing.T) {
	resolver, err := NewAtomicResolver(nil)
	if err != nil {
		t.Fatal(err)
	}
	scope := &atomicTestScope{values: map[string]string{
		"aws:prod/key":   "first",
		"vault:main/key": "second",
	}}
	input := map[string]any{
		"a": "${store:vault:main:key}",
		"b": "${store:aws:prod:key}",
	}
	acquires := 0
	resolved, err := resolver.Resolve(
		context.Background(),
		input,
		func(keys []string) (AtomicScope, error) {
			acquires++
			if !reflect.DeepEqual(keys, []string{"aws:prod", "vault:main"}) {
				t.Fatalf("keys=%v", keys)
			}
			return scope, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.(map[string]any)["a"] != "second" ||
		resolved.(map[string]any)["b"] != "first" {
		t.Fatalf("resolved=%#v", resolved)
	}
	if acquires != 1 || scope.resolves != 2 || scope.releases != 1 {
		t.Fatalf(
			"acquires=%d resolves=%d releases=%d",
			acquires,
			scope.resolves,
			scope.releases,
		)
	}
	if input["a"] != "${store:vault:main:key}" {
		t.Fatal("successful resolution mutated caller input")
	}
}

func BenchmarkBResolverTraversal(b *testing.B) {
	resolver, err := NewAtomicResolver(map[string]AtomicProvider{
		"test": AtomicProviderFunc(func(context.Context, string) ([]byte, error) {
			return []byte("secret"), nil
		}),
	})
	if err != nil {
		b.Fatal(err)
	}
	input := atomicNestedMap(32)
	leaf := input
	for range 32 {
		leaf = leaf["next"].(map[string]any)
	}
	leaf["value"] = "${test:key}"
	b.ReportAllocs()
	for b.Loop() {
		if _, err := resolver.Resolve(context.Background(), input, nil); err != nil {
			b.Fatal(err)
		}
	}
}

type atomicTestScope struct {
	values   map[string]string
	resolves int
	releases int
}

func (scope *atomicTestScope) Resolve(
	_ context.Context,
	storeKey string,
	secretKey string,
) ([]byte, error) {
	scope.resolves++
	value, ok := scope.values[storeKey+"/"+secretKey]
	if !ok {
		return nil, errors.New("missing secret")
	}
	return []byte(value), nil
}

func (scope *atomicTestScope) Release(context.Context) error {
	scope.releases++
	return nil
}

func atomicNestedMap(edges int) map[string]any {
	root := map[string]any{}
	current := root
	for range edges {
		next := map[string]any{}
		current["next"] = next
		current = next
	}
	current["value"] = strconv.Itoa(edges)
	return root
}

func requireAtomicErrorKind(t *testing.T, err error, want AtomicErrorKind) {
	t.Helper()
	var resolveErr *AtomicResolveError
	if !errors.As(err, &resolveErr) || resolveErr.Kind != want {
		t.Fatalf("error=%v want AtomicResolveError kind %s", err, want)
	}
}
