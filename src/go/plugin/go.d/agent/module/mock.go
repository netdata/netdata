// SPDX-License-Identifier: GPL-3.0-or-later

package module

import "errors"

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

// MockModule MockModule.
type MockModule struct {
	Base

	Config MockConfiguration `yaml:",inline" json:""`

	FailOnInit bool

	InitFunc    func() error
	CheckFunc   func() error
	ChartsFunc  func() *Charts
	CollectFunc func() map[string]int64
	CleanupFunc func()
	CleanupDone bool
}

// Init invokes InitFunc.
func (m *MockModule) Init() error {
	if m.FailOnInit {
		return errors.New("mock init error")
	}
	if m.InitFunc == nil {
		return nil
	}
	return m.InitFunc()
}

// Check invokes CheckFunc.
func (m *MockModule) Check() error {
	if m.CheckFunc == nil {
		return nil
	}
	return m.CheckFunc()
}

// Charts invokes ChartsFunc.
func (m *MockModule) Charts() *Charts {
	if m.ChartsFunc == nil {
		return nil
	}
	return m.ChartsFunc()
}

// Collect invokes CollectDunc.
func (m *MockModule) Collect() map[string]int64 {
	if m.CollectFunc == nil {
		return nil
	}
	return m.CollectFunc()
}

// Cleanup sets CleanupDone to true.
func (m *MockModule) Cleanup() {
	if m.CleanupFunc != nil {
		m.CleanupFunc()
	}
	m.CleanupDone = true
}

func (m *MockModule) Configuration() any {
	return m.Config
}
