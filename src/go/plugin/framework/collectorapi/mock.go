// SPDX-License-Identifier: GPL-3.0-or-later

package collectorapi

import (
	"context"
	"errors"
)

const MockConfigSchema = `
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "properties": {
    "option_str": {
      "type": "string",
      "description": "Option string value"
    },
    "option_int": {
      "type": "integer",
      "description": "Option integer value"
    }
  },
  "required": [
    "option_str",
    "option_int"
  ]
}
`

type MockConfiguration struct {
	OptionStr string `yaml:"option_str" json:"option_str"`
	OptionInt int    `yaml:"option_int" json:"option_int"`
}

// MockCollectorV1 MockCollectorV1.
type MockCollectorV1 struct {
	Base

	Config MockConfiguration `yaml:",inline" json:""`

	FailOnInit bool

	InitFunc    func(context.Context) error
	CheckFunc   func(context.Context) error
	ChartsFunc  func() *Charts
	CollectFunc func(context.Context) map[string]int64
	CleanupFunc func(context.Context)
	CleanupDone bool
}

// Init invokes InitFunc.
func (m *MockCollectorV1) Init(ctx context.Context) error {
	if m.FailOnInit {
		return errors.New("mock init error")
	}
	if m.InitFunc == nil {
		return nil
	}
	return m.InitFunc(ctx)
}

// Check invokes CheckFunc.
func (m *MockCollectorV1) Check(ctx context.Context) error {
	if m.CheckFunc == nil {
		return nil
	}
	return m.CheckFunc(ctx)
}

// Charts invokes ChartsFunc.
func (m *MockCollectorV1) Charts() *Charts {
	if m.ChartsFunc == nil {
		return nil
	}
	return m.ChartsFunc()
}

// Collect invokes CollectDunc.
func (m *MockCollectorV1) Collect(ctx context.Context) map[string]int64 {
	if m.CollectFunc == nil {
		return nil
	}
	return m.CollectFunc(ctx)
}

// Cleanup sets CleanupDone to true.
func (m *MockCollectorV1) Cleanup(ctx context.Context) {
	if m.CleanupFunc != nil {
		m.CleanupFunc(ctx)
	}
	m.CleanupDone = true
}

func (m *MockCollectorV1) Configuration() any {
	return m.Config
}
