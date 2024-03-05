// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewManager(t *testing.T) {
	mgr := NewManager()

	assert.NotNilf(t, mgr.Input, "Input")
	assert.NotNilf(t, mgr.FunctionRegistry, "FunctionRegistry")
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
			for name := range mgr.FunctionRegistry {
				got = append(got, name)
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
			input: `
FUNCTION UID 1 "fn1 arg1 arg2" 0xFFFF "method=api,role=test"
`,
			expected: []Function{
				{
					key:         "FUNCTION",
					UID:         "UID",
					Timeout:     time.Second,
					Name:        "fn1",
					Args:        []string{"arg1", "arg2"},
					Permissions: "0xFFFF",
					Source:      "method=api,role=test",
					ContentType: "",
					Payload:     nil,
				},
			},
		},
		"valid function: multiple": {
			register: []string{"fn1", "fn2"},
			input: `
FUNCTION UID 1 "fn1 arg1 arg2" 0xFFFF "method=api,role=test"
FUNCTION UID 1 "fn2 arg1 arg2" 0xFFFF "method=api,role=test"
`,
			expected: []Function{
				{
					key:         "FUNCTION",
					UID:         "UID",
					Timeout:     time.Second,
					Name:        "fn1",
					Args:        []string{"arg1", "arg2"},
					Permissions: "0xFFFF",
					Source:      "method=api,role=test",
					ContentType: "",
					Payload:     nil,
				},
				{
					key:         "FUNCTION",
					UID:         "UID",
					Timeout:     time.Second,
					Name:        "fn2",
					Args:        []string{"arg1", "arg2"},
					Permissions: "0xFFFF",
					Source:      "method=api,role=test",
					ContentType: "",
					Payload:     nil,
				},
			},
		},
		"valid function: single with payload": {
			register: []string{"fn1", "fn2"},
			input: `
FUNCTION_PAYLOAD UID 1 "fn1 arg1 arg2" 0xFFFF "method=api,role=test" application/json
payload line1
payload line2
FUNCTION_PAYLOAD_END
`,
			expected: []Function{
				{
					key:         "FUNCTION_PAYLOAD",
					UID:         "UID",
					Timeout:     time.Second,
					Name:        "fn1",
					Args:        []string{"arg1", "arg2"},
					Permissions: "0xFFFF",
					Source:      "method=api,role=test",
					ContentType: "application/json",
					Payload:     []byte("payload line1\npayload line2"),
				},
			},
		},
		"valid function: multiple with payload": {
			register: []string{"fn1", "fn2"},
			input: `
FUNCTION_PAYLOAD UID 1 "fn1 arg1 arg2" 0xFFFF "method=api,role=test" application/json
payload line1
payload line2
FUNCTION_PAYLOAD_END

FUNCTION_PAYLOAD UID 1 "fn2 arg1 arg2" 0xFFFF "method=api,role=test" application/json
payload line3
payload line4
FUNCTION_PAYLOAD_END
`,
			expected: []Function{
				{
					key:         "FUNCTION_PAYLOAD",
					UID:         "UID",
					Timeout:     time.Second,
					Name:        "fn1",
					Args:        []string{"arg1", "arg2"},
					Permissions: "0xFFFF",
					Source:      "method=api,role=test",
					ContentType: "application/json",
					Payload:     []byte("payload line1\npayload line2"),
				},
				{
					key:         "FUNCTION_PAYLOAD",
					UID:         "UID",
					Timeout:     time.Second,
					Name:        "fn2",
					Args:        []string{"arg1", "arg2"},
					Permissions: "0xFFFF",
					Source:      "method=api,role=test",
					ContentType: "application/json",
					Payload:     []byte("payload line3\npayload line4"),
				},
			},
		},
		"valid function: multiple with and without payload": {
			register: []string{"fn1", "fn2", "fn3", "fn4"},
			input: `
FUNCTION_PAYLOAD UID 1 "fn1 arg1 arg2" 0xFFFF "method=api,role=test" application/json
payload line1
payload line2
FUNCTION_PAYLOAD_END

FUNCTION UID 1 "fn2 arg1 arg2" 0xFFFF "method=api,role=test"
FUNCTION UID 1 "fn3 arg1 arg2" 0xFFFF "method=api,role=test"

FUNCTION_PAYLOAD UID 1 "fn4 arg1 arg2" 0xFFFF "method=api,role=test" application/json
payload line3
payload line4
FUNCTION_PAYLOAD_END
`,
			expected: []Function{
				{
					key:         "FUNCTION_PAYLOAD",
					UID:         "UID",
					Timeout:     time.Second,
					Name:        "fn1",
					Args:        []string{"arg1", "arg2"},
					Permissions: "0xFFFF",
					Source:      "method=api,role=test",
					ContentType: "application/json",
					Payload:     []byte("payload line1\npayload line2"),
				},
				{
					key:         "FUNCTION",
					UID:         "UID",
					Timeout:     time.Second,
					Name:        "fn2",
					Args:        []string{"arg1", "arg2"},
					Permissions: "0xFFFF",
					Source:      "method=api,role=test",
					ContentType: "",
					Payload:     nil,
				},
				{
					key:         "FUNCTION",
					UID:         "UID",
					Timeout:     time.Second,
					Name:        "fn3",
					Args:        []string{"arg1", "arg2"},
					Permissions: "0xFFFF",
					Source:      "method=api,role=test",
					ContentType: "",
					Payload:     nil,
				},
				{
					key:         "FUNCTION_PAYLOAD",
					UID:         "UID",
					Timeout:     time.Second,
					Name:        "fn4",
					Args:        []string{"arg1", "arg2"},
					Permissions: "0xFFFF",
					Source:      "method=api,role=test",
					ContentType: "application/json",
					Payload:     []byte("payload line3\npayload line4"),
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			mgr := NewManager()

			mgr.Input = strings.NewReader(test.input)

			mock := &mockFunctionExecutor{}
			for _, v := range test.register {
				mgr.Register(v, mock.execute)
			}

			testTime := time.Second * 5
			ctx, cancel := context.WithTimeout(context.Background(), testTime)
			defer cancel()

			done := make(chan struct{})

			go func() { defer close(done); mgr.Run(ctx) }()

			timeout := testTime + time.Second*2
			tk := time.NewTimer(timeout)
			defer tk.Stop()

			select {
			case <-done:
				assert.Equal(t, test.expected, mock.executed)
			case <-tk.C:
				t.Errorf("timed out after %s", timeout)
			}
		})
	}
}

type mockFunctionExecutor struct {
	executed []Function
}

func (m *mockFunctionExecutor) execute(fn Function) {
	m.executed = append(m.executed, fn)
}
