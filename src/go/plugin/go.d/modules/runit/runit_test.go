// SPDX-License-Identifier: GPL-3.0-or-later

package runit

import (
	"os"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,
	} {
		require.NotNil(t, data, name)
	}
}

func TestRunit_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Runit{}, dataConfigJSON, dataConfigYAML)
}

func TestNew(t *testing.T) {
	// We want to ensure that module is a reference type, nothing more.

	assert.IsType(t, (*Runit)(nil), New())
}

func TestRunit_Init(t *testing.T) {
	os.Setenv("SVDIR", "testdata") // Required to get working New().Config.

	// 'Init() bool' initializes the module with an appropriate config, so to test it we need:
	// - provide the config.
	// - set module.Config field with the config.
	// - call Init() and compare its return value with the expected value.

	// 'test' map contains different test cases.
	tests := map[string]struct {
		wantFail bool
		config   Config
	}{
		"success on default config": {
			config: New().Config,
		},
		"fails if 'ndsudo' not found": {
			wantFail: true, // Triggered because test name contains "ndsudo".
			config:   New().Config,
		},
		"success when 'dir' is existing directory": {
			config: Config{
				Dir: "testdata",
			},
		},
		"fails when 'dir' is empty": {
			wantFail: true,
			config: Config{
				Dir: "",
			},
		},
		"fails when 'dir' is not a directory": {
			wantFail: true,
			config: Config{
				Dir: "runit_test.go",
			},
		},
		"fails when 'dir' is not exist": {
			wantFail: true,
			config: Config{
				Dir: "nosuch",
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			runit := New()
			runit.Config = test.config

			executable.Directory = "testdata"
			if strings.Contains(name, "ndsudo") {
				executable.Directory = ""
			}

			if test.wantFail {
				assert.Error(t, runit.Init())
			} else {
				assert.NoError(t, runit.Init())
			}
		})
	}
}
