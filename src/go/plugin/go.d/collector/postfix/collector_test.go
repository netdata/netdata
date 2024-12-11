// SPDX-License-Identifier: GPL-3.0-or-later

package postfix

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataPostqueue, _ = os.ReadFile("testdata/postqueue.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,
		"dataPostqueue":  dataPostqueue,
	} {
		require.NotNil(t, data, name)
	}
}

func TestCollector_Configuration(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"fails if 'binary_path' is not set": {
			wantFail: true,
			config: Config{
				BinaryPath: "",
			},
		},
		"fails if failed to find binary": {
			wantFail: true,
			config: Config{
				BinaryPath: "postqueue!!!",
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.Config = test.config

			if test.wantFail {
				assert.Error(t, collr.Init(context.Background()))
			} else {
				assert.NoError(t, collr.Init(context.Background()))
			}
		})
	}
}

func TestCollector_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare func() *Collector
	}{
		"not initialized exec": {
			prepare: func() *Collector {
				return New()
			},
		},
		"after check": {
			prepare: func() *Collector {
				pf := New()
				pf.exec = prepareMockOK()
				_ = pf.Check(context.Background())
				return pf
			},
		},
		"after collect": {
			prepare: func() *Collector {
				pf := New()
				pf.exec = prepareMockOK()
				_ = pf.Collect(context.Background())
				return pf
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := test.prepare()

			assert.NotPanics(t, func() { collr.Cleanup(context.Background()) })
		})
	}
}

func TestCollector_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *mockPostqueueExec
		wantFail    bool
	}{
		"success case": {
			wantFail:    false,
			prepareMock: prepareMockOK,
		},
		"mail queue is empty": {
			wantFail:    false,
			prepareMock: prepareMockEmptyMailQueue,
		},
		"error on list call": {
			wantFail:    true,
			prepareMock: prepareMockErrOnList,
		},
		"empty response": {
			wantFail:    true,
			prepareMock: prepareMockEmptyResponse,
		},
		"unexpected response": {
			wantFail:    true,
			prepareMock: prepareMockUnexpectedResponse,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			mock := test.prepareMock()
			collr.exec = mock

			if test.wantFail {
				assert.Error(t, collr.Check(context.Background()))
			} else {
				assert.NoError(t, collr.Check(context.Background()))
			}
		})
	}
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *mockPostqueueExec
		wantMetrics map[string]int64
	}{
		"success case": {
			prepareMock: prepareMockOK,
			wantMetrics: map[string]int64{
				"emails": 12991,
				"size":   132422,
			},
		},
		"mail queue is empty": {
			prepareMock: prepareMockEmptyMailQueue,
			wantMetrics: map[string]int64{
				"emails": 0,
				"size":   0,
			},
		},
		"error on list call": {
			prepareMock: prepareMockErrOnList,
			wantMetrics: nil,
		},
		"empty response": {
			prepareMock: prepareMockEmptyResponse,
			wantMetrics: nil,
		},
		"unexpected response": {
			prepareMock: prepareMockUnexpectedResponse,
			wantMetrics: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			mock := test.prepareMock()
			collr.exec = mock

			mx := collr.Collect(context.Background())

			assert.Equal(t, test.wantMetrics, mx)
		})
	}
}

func prepareMockOK() *mockPostqueueExec {
	return &mockPostqueueExec{
		listData: dataPostqueue,
	}
}

func prepareMockEmptyMailQueue() *mockPostqueueExec {
	return &mockPostqueueExec{
		listData: []byte("Mail queue is empty"),
	}
}

func prepareMockErrOnList() *mockPostqueueExec {
	return &mockPostqueueExec{
		errOnList: true,
	}
}

func prepareMockEmptyResponse() *mockPostqueueExec {
	return &mockPostqueueExec{}
}

func prepareMockUnexpectedResponse() *mockPostqueueExec {
	return &mockPostqueueExec{
		listData: []byte(`
Lorem ipsum dolor sit amet, consectetur adipiscing elit.
Nulla malesuada erat id magna mattis, eu viverra tellus rhoncus.
Fusce et felis pulvinar, posuere sem non, porttitor eros.
`),
	}
}

type mockPostqueueExec struct {
	errOnList bool
	listData  []byte
}

func (m *mockPostqueueExec) list() ([]byte, error) {
	if m.errOnList {
		return nil, errors.New("mock.list() error")
	}

	return m.listData, nil
}
