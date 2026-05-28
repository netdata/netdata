// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"bufio"
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const (
	managerTestPermissions = "0xFFFF"
	managerTestSource      = "method=api,role=test"
)

func TestNewManager(t *testing.T) {
	mgr := NewManager()

	assert.NotNilf(t, mgr.input, "Input")
	assert.NotNilf(t, mgr.functionRegistry, "FunctionRegistry")
}

func TestManager_Register(t *testing.T) {
	type testInputFn struct {
		name    string
		invalid bool
	}
	tests := map[string]struct {
		input    []testInputFn
		expected []string
	}{
		"valid registration": {
			input: []testInputFn{
				{name: "fn1"},
				{name: "fn2"},
			},
			expected: []string{"fn1", "fn2"},
		},
		"registration with duplicates": {
			input: []testInputFn{
				{name: "fn1"},
				{name: "fn2"},
				{name: "fn1"},
			},
			expected: []string{"fn1", "fn2"},
		},
		"registration with nil functions": {
			input: []testInputFn{
				{name: "fn1"},
				{name: "fn2", invalid: true},
			},
			expected: []string{"fn1"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			mgr := NewManager()

			for _, v := range test.input {
				if v.invalid {
					mgr.Register(v.name, nil)
				} else {
					mgr.Register(v.name, func(Function) {})
				}
			}

			var got []string
			for name := range mgr.functionRegistry {
				got = append(got, name)
			}
			sort.Strings(got)
			sort.Strings(test.expected)

			assert.Equal(t, test.expected, got)
		})
	}
}

func TestManager_RegisterPrefix(t *testing.T) {
	type inputFn struct {
		name    string
		prefix  string
		invalid bool
	}

	tests := map[string]struct {
		input    []inputFn
		expected []string // flattened as "name:prefix"
	}{
		"valid registration (two prefixes under same name)": {
			input: []inputFn{
				{name: "config", prefix: "collector:"},
				{name: "config", prefix: "vnode:"},
			},
			expected: []string{"config:collector:", "config:vnode:"},
		},
		"registration with duplicates (same name+prefix)": {
			input: []inputFn{
				{name: "config", prefix: "collector:"},
				{name: "config", prefix: "collector:"}, // duplicate should overwrite, not duplicate
				{name: "config", prefix: "vnode:"},
			},
			expected: []string{"config:collector:", "config:vnode:"},
		},
		"registration across multiple names": {
			input: []inputFn{
				{name: "config", prefix: "collector:"},
				{name: "status", prefix: "node:"},
			},
			expected: []string{"config:collector:", "status:node:"},
		},
		"registration with nil functions is ignored": {
			input: []inputFn{
				{name: "config", prefix: "collector:"},
				{name: "config", prefix: "vnode:", invalid: true}, // nil fn -> ignored
			},
			expected: []string{"config:collector:"},
		},
		"overlapping prefix is rejected (short first)": {
			input: []inputFn{
				{name: "config", prefix: "collector:"},
				{name: "config", prefix: "collector:job:"},
			},
			expected: []string{"config:collector:"},
		},
		"overlapping prefix is rejected (long first)": {
			input: []inputFn{
				{name: "config", prefix: "collector:job:"},
				{name: "config", prefix: "collector:"},
			},
			expected: []string{"config:collector:job:"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			mgr := NewManager()

			for _, v := range test.input {
				if v.invalid {
					mgr.RegisterPrefix(v.name, v.prefix, nil)
				} else {
					mgr.RegisterPrefix(v.name, v.prefix, func(Function) {})
				}
			}

			var got []string
			for fname, fs := range mgr.functionRegistry {
				if fs == nil || len(fs.prefixes) == 0 {
					continue
				}
				for p := range fs.prefixes {
					got = append(got, fmt.Sprintf("%s:%s", fname, p))
				}
			}
			sort.Strings(got)
			sort.Strings(test.expected)

			assert.Equal(t, test.expected, got)
		})
	}
}

func TestManager_UnregisterPrefix(t *testing.T) {
	type regFn struct {
		name   string
		prefix string
	}
	type unregFn struct {
		name   string
		prefix string
	}

	tests := map[string]struct {
		register []regFn
		unreg    []unregFn
		expected []string // flattened as "name:prefix"
	}{
		"remove one of multiple prefixes keeps the other": {
			register: []regFn{
				{name: "config", prefix: "collector:"},
				{name: "config", prefix: "vnode:"},
			},
			unreg: []unregFn{
				{name: "config", prefix: "collector:"},
			},
			expected: []string{"config:vnode:"},
		},
		"remove last prefix deletes the name entry": {
			register: []regFn{
				{name: "config", prefix: "collector:"},
			},
			unreg: []unregFn{
				{name: "config", prefix: "collector:"},
			},
			expected: nil,
		},
		"unregister non-existing prefix is a no-op": {
			register: []regFn{
				{name: "config", prefix: "collector:"},
			},
			unreg: []unregFn{
				{name: "config", prefix: "vnode:"}, // doesn't exist
			},
			expected: []string{"config:collector:"},
		},
		"unregister on unknown name is a no-op": {
			register: []regFn{
				{name: "config", prefix: "collector:"},
			},
			unreg: []unregFn{
				{name: "status", prefix: "node:"}, // name not present
			},
			expected: []string{"config:collector:"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			mgr := NewManager()

			// initial registrations
			for _, r := range test.register {
				mgr.RegisterPrefix(r.name, r.prefix, func(Function) {})
			}

			// perform unregistrations
			for _, u := range test.unreg {
				mgr.UnregisterPrefix(u.name, u.prefix)
			}

			var got []string
			for fname, fs := range mgr.functionRegistry {
				if fs == nil || len(fs.prefixes) == 0 {
					continue
				}
				for p := range fs.prefixes {
					got = append(got, fmt.Sprintf("%s:%s", fname, p))
				}
			}
			sort.Strings(got)
			sort.Strings(test.expected)

			assert.Equal(t, test.expected, got)
		})
	}
}

func TestManager_Run(t *testing.T) {
	tests := map[string]struct {
		register []string
		input    string
		expected []Function
	}{
		"valid function: single": {
			register: []string{"fn1"},
			input: fmt.Sprintf(`
FUNCTION UID 1 "fn1 arg1 arg2" %s "%s"
`, managerTestPermissions, managerTestSource),
			expected: []Function{
				{
					key:         lineFunction,
					UID:         "UID",
					Timeout:     time.Second,
					Name:        "fn1",
					Args:        []string{"arg1", "arg2"},
					Permissions: managerTestPermissions,
					Source:      managerTestSource,
					ContentType: "",
					Payload:     nil,
				},
			},
		},
		"valid function: multiple": {
			register: []string{"fn1", "fn2"},
			input: fmt.Sprintf(`
FUNCTION UID1 1 "fn1 arg1 arg2" %s "%s"
FUNCTION UID2 1 "fn2 arg1 arg2" %s "%s"
`, managerTestPermissions, managerTestSource, managerTestPermissions, managerTestSource),
			expected: []Function{
				{
					key:         lineFunction,
					UID:         "UID1",
					Timeout:     time.Second,
					Name:        "fn1",
					Args:        []string{"arg1", "arg2"},
					Permissions: managerTestPermissions,
					Source:      managerTestSource,
					ContentType: "",
					Payload:     nil,
				},
				{
					key:         lineFunction,
					UID:         "UID2",
					Timeout:     time.Second,
					Name:        "fn2",
					Args:        []string{"arg1", "arg2"},
					Permissions: managerTestPermissions,
					Source:      managerTestSource,
					ContentType: "",
					Payload:     nil,
				},
			},
		},
		"valid function: single with payload": {
			register: []string{"fn1", "fn2"},
			input: fmt.Sprintf(`
FUNCTION_PAYLOAD UID 1 "fn1 arg1 arg2" %s "%s" application/json
payload line1
payload line2
FUNCTION_PAYLOAD_END
`, managerTestPermissions, managerTestSource),
			expected: []Function{
				{
					key:         lineFunctionPayload,
					UID:         "UID",
					Timeout:     time.Second,
					Name:        "fn1",
					Args:        []string{"arg1", "arg2"},
					Permissions: managerTestPermissions,
					Source:      managerTestSource,
					ContentType: "application/json",
					Payload:     []byte("payload line1\npayload line2"),
				},
			},
		},
		"valid function: multiple with payload": {
			register: []string{"fn1", "fn2"},
			input: fmt.Sprintf(`
FUNCTION_PAYLOAD UID1 1 "fn1 arg1 arg2" %s "%s" application/json
payload line1
payload line2
FUNCTION_PAYLOAD_END

FUNCTION_PAYLOAD UID2 1 "fn2 arg1 arg2" %s "%s" application/json
payload line3
payload line4
FUNCTION_PAYLOAD_END
`, managerTestPermissions, managerTestSource, managerTestPermissions, managerTestSource),
			expected: []Function{
				{
					key:         lineFunctionPayload,
					UID:         "UID1",
					Timeout:     time.Second,
					Name:        "fn1",
					Args:        []string{"arg1", "arg2"},
					Permissions: managerTestPermissions,
					Source:      managerTestSource,
					ContentType: "application/json",
					Payload:     []byte("payload line1\npayload line2"),
				},
				{
					key:         lineFunctionPayload,
					UID:         "UID2",
					Timeout:     time.Second,
					Name:        "fn2",
					Args:        []string{"arg1", "arg2"},
					Permissions: managerTestPermissions,
					Source:      managerTestSource,
					ContentType: "application/json",
					Payload:     []byte("payload line3\npayload line4"),
				},
			},
		},
		"valid function: multiple with and without payload": {
			register: []string{"fn1", "fn2", "fn3", "fn4"},
			input: fmt.Sprintf(`
FUNCTION_PAYLOAD UID1 1 "fn1 arg1 arg2" %s "%s" application/json
payload line1
payload line2
FUNCTION_PAYLOAD_END

FUNCTION UID2 1 "fn2 arg1 arg2" %s "%s"
FUNCTION UID3 1 "fn3 arg1 arg2" %s "%s"

FUNCTION_PAYLOAD UID4 1 "fn4 arg1 arg2" %s "%s" application/json
payload line3
payload line4
FUNCTION_PAYLOAD_END
`, managerTestPermissions, managerTestSource,
				managerTestPermissions, managerTestSource,
				managerTestPermissions, managerTestSource,
				managerTestPermissions, managerTestSource),
			expected: []Function{
				{
					key:         lineFunctionPayload,
					UID:         "UID1",
					Timeout:     time.Second,
					Name:        "fn1",
					Args:        []string{"arg1", "arg2"},
					Permissions: managerTestPermissions,
					Source:      managerTestSource,
					ContentType: "application/json",
					Payload:     []byte("payload line1\npayload line2"),
				},
				{
					key:         lineFunction,
					UID:         "UID2",
					Timeout:     time.Second,
					Name:        "fn2",
					Args:        []string{"arg1", "arg2"},
					Permissions: managerTestPermissions,
					Source:      managerTestSource,
					ContentType: "",
					Payload:     nil,
				},
				{
					key:         lineFunction,
					UID:         "UID3",
					Timeout:     time.Second,
					Name:        "fn3",
					Args:        []string{"arg1", "arg2"},
					Permissions: managerTestPermissions,
					Source:      managerTestSource,
					ContentType: "",
					Payload:     nil,
				},
				{
					key:         lineFunctionPayload,
					UID:         "UID4",
					Timeout:     time.Second,
					Name:        "fn4",
					Args:        []string{"arg1", "arg2"},
					Permissions: managerTestPermissions,
					Source:      managerTestSource,
					ContentType: "application/json",
					Payload:     []byte("payload line3\npayload line4"),
				},
			},
		},
	}

	for name, test := range tests {
		for workerProfile, workerCount := range map[string]int{
			"single-worker": 1,
			"multi-worker":  4,
		} {
			t.Run(name+"/"+workerProfile, func(t *testing.T) {
				mgr := NewManager()
				mgr.workerCount = workerCount

				mgr.input = newMockInput(test.input)

				mock := &mockFunctionExecutor{}
				for _, v := range test.register {
					mgr.Register(v, mock.execute)
				}

				testTime := time.Second * 5
				ctx, cancel := context.WithTimeout(context.Background(), testTime)
				defer cancel()

				done := make(chan struct{})

				go func() { defer close(done); mgr.Run(ctx, nil) }()

				timeout := testTime + time.Second*2
				tk := time.NewTimer(timeout)
				defer tk.Stop()

				select {
				case <-done:
					assert.ElementsMatch(t, test.expected, mock.snapshot())
				case <-tk.C:
					t.Errorf("timed out after %s", timeout)
				}
			})
		}
	}
}

type mockFunctionExecutor struct {
	mu       sync.Mutex
	executed []Function
}

func (m *mockFunctionExecutor) execute(fn Function) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.executed = append(m.executed, fn)
}

func (m *mockFunctionExecutor) snapshot() []Function {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Function, len(m.executed))
	copy(out, m.executed)
	return out
}

func newMockInput(data string) *mockInput {
	m := &mockInput{chLines: make(chan string)}
	sc := bufio.NewScanner(strings.NewReader(data))
	go func() {
		for sc.Scan() {
			m.chLines <- sc.Text()
		}
		close(m.chLines)
	}()
	return m
}

type mockInput struct {
	chLines chan string
}

func (m *mockInput) lines() <-chan string {
	return m.chLines
}
