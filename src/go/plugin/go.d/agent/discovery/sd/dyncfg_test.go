// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/safewriter"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/pipeline"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/functions"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type dyncfgSim struct {
	do func(sd *ServiceDiscovery)

	wantExposed    []wantExposedConfig
	wantRunning    []string
	wantDyncfg     string
	wantDyncfgFunc func(t *testing.T, got string)
}

type wantExposedConfig struct {
	discovererType string
	name           string
	sourceType     string
	status         dyncfg.Status
}

func (s *dyncfgSim) run(t *testing.T) {
	t.Helper()

	require.NotNil(t, s.do, "s.do is nil")

	var buf bytes.Buffer
	sd := &ServiceDiscovery{
		Logger:         logger.New(),
		dyncfgApi:      dyncfg.NewResponder(netdataapi.New(safewriter.New(&buf))),
		exposedConfigs: newExposedSDConfigs(),
		dyncfgCh:       make(chan dyncfg.Function),
		newPipeline: func(cfg pipeline.Config) (sdPipeline, error) {
			return newTestPipeline(cfg.Name), nil
		},
	}

	done := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())

	// Create output channel (we don't need to capture output for dyncfg tests)
	out := make(chan<- []*confgroup.Group)

	// Create send function
	send := func(ctx context.Context, groups []*confgroup.Group) {
		select {
		case <-ctx.Done():
		case out <- groups:
		}
	}

	sd.ctx = ctx
	sd.mgr = NewPipelineManager(sd.Logger, sd.newPipeline, send)

	// Register dyncfg templates (creates CONFIG entries for templates)
	sd.registerDyncfgTemplates(ctx)

	// Start processing dyncfg commands
	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			case fn := <-sd.dyncfgCh:
				sd.dyncfgSeqExec(fn)
			}
		}
	}()

	timeout := time.Second * 5

	// Run the test scenario
	s.do(sd)

	// Give a bit of time for async operations
	time.Sleep(100 * time.Millisecond)

	cancel()

	select {
	case <-done:
	case <-time.After(timeout):
		t.Errorf("failed to finish work in %s", timeout)
	}

	// Filter and normalize dyncfg output (same approach as jobmgr sim_test.go)
	var lines []string
	for _, line := range strings.Split(buf.String(), "\n") {
		// Skip template CONFIG lines (registered on startup)
		if strings.HasPrefix(line, "CONFIG") && strings.Contains(line, " template ") {
			continue
		}
		// Remove timestamp from FUNCTION_RESULT_BEGIN
		if strings.HasPrefix(line, "FUNCTION_RESULT_BEGIN") {
			parts := strings.Fields(line)
			line = strings.Join(parts[:len(parts)-1], " ")
		}
		lines = append(lines, line)
	}

	gotDyncfg := strings.TrimSpace(strings.Join(lines, "\n"))

	if s.wantDyncfgFunc != nil {
		s.wantDyncfgFunc(t, gotDyncfg)
	} else if s.wantDyncfg != "" {
		wantDyncfg := strings.TrimSpace(s.wantDyncfg)
		assert.Equal(t, wantDyncfg, gotDyncfg, "dyncfg commands")
	}

	// Verify exposed configs
	if s.wantExposed != nil {
		wantLen, gotLen := len(s.wantExposed), len(sd.exposedConfigs.items)
		require.Equalf(t, wantLen, gotLen, "exposedConfigs: different len (want %d got %d)", wantLen, gotLen)

		for _, want := range s.wantExposed {
			key := want.discovererType + ":" + want.name
			cfg, ok := sd.exposedConfigs.items[key]
			require.Truef(t, ok, "exposedConfigs: config '%s' not found", key)
			assert.Equal(t, want.sourceType, cfg.sourceType, "exposedConfigs: wrong sourceType for '%s'", key)
			assert.Equal(t, want.status, cfg.status, "exposedConfigs: wrong status for '%s'", key)
		}
	}

	// Verify running pipelines
	if s.wantRunning != nil {
		gotRunning := sd.mgr.Keys()
		assert.ElementsMatch(t, s.wantRunning, gotRunning, "running pipelines")
	}
}

// sendDyncfgCmd sends a dyncfg command and waits for processing
func sendDyncfgCmd(sd *ServiceDiscovery, uid string, args []string, payload []byte, source string) {
	fn := dyncfg.NewFunction(functions.Function{
		UID:         uid,
		Args:        args,
		Payload:     payload,
		Source:      source,
		ContentType: "application/json",
	})

	// Call dyncfgConfig directly for commands that are handled there (schema, get, userconfig)
	// and dyncfgSeqExec for state-changing commands
	cmd := ""
	if len(args) >= 2 {
		cmd = args[1]
	}

	switch cmd {
	case "schema", "get", "userconfig", "test":
		// These are handled directly in dyncfgConfig (read-only/validation commands)
		sd.dyncfgConfig(fn)
	default:
		// State-changing commands go through the channel
		select {
		case sd.dyncfgCh <- fn:
		case <-time.After(time.Second):
		}
	}

	// Give time for processing
	time.Sleep(50 * time.Millisecond)
}

func TestServiceDiscovery_DyncfgSchema(t *testing.T) {
	tests := map[string]struct {
		createSim func() *dyncfgSim
	}{
		"schema for net_listeners template": {
			createSim: func() *dyncfgSim {
				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						sendDyncfgCmd(sd, "1-schema",
							[]string{sd.dyncfgTemplateID(DiscovererNetListeners), "schema"},
							nil, "")
					},
					wantDyncfgFunc: func(t *testing.T, got string) {
						assert.Contains(t, got, "FUNCTION_RESULT_BEGIN 1-schema 200 application/json")
						assert.Contains(t, got, `"jsonSchema"`)
						assert.Contains(t, got, "FUNCTION_RESULT_END")
					},
				}
			},
		},
		"schema for unknown discoverer type": {
			createSim: func() *dyncfgSim {
				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						sendDyncfgCmd(sd, "1-schema",
							[]string{sd.dyncfgSDPrefixValue() + "unknown", "schema"},
							nil, "")
					},
					wantDyncfgFunc: func(t *testing.T, got string) {
						assert.Contains(t, got, "FUNCTION_RESULT_BEGIN 1-schema 404 application/json")
						assert.Contains(t, got, "Unknown discoverer type")
						assert.Contains(t, got, "FUNCTION_RESULT_END")
					},
				}
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			sim := tc.createSim()
			sim.run(t)
		})
	}
}

func TestServiceDiscovery_DyncfgAdd(t *testing.T) {
	tests := map[string]struct {
		createSim func() *dyncfgSim
	}{
		"add net_listeners job": {
			createSim: func() *dyncfgSim {
				cfg := DyncfgNetListenersConfig{
					Name: "test-job",
				}
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(DiscovererNetListeners), "add", "test-job"},
							payload, "type=dyncfg,user=test")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: DiscovererNetListeners,
							name:           "test-job",
							sourceType:     "dyncfg",
							status:         dyncfg.StatusAccepted,
						},
					},
					wantDyncfg: `
CONFIG test:sd:net_listeners:test-job create accepted job /collectors/test/ServiceDiscovery dyncfg 'type=dyncfg,user=test' 'schema get enable disable update userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END
`,
				}
			},
		},
		"add without payload fails": {
			createSim: func() *dyncfgSim {
				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(DiscovererNetListeners), "add", "test-job"},
							nil, "type=dyncfg,user=test")
					},
					wantExposed: []wantExposedConfig{},
					wantDyncfg: `
FUNCTION_RESULT_BEGIN 1-add 400 application/json
{"status":400,"errorMessage":"missing configuration payload"}
FUNCTION_RESULT_END
`,
				}
			},
		},
		"add duplicate job fails": {
			createSim: func() *dyncfgSim {
				cfg := DyncfgNetListenersConfig{Name: "test-job"}
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// First add
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(DiscovererNetListeners), "add", "test-job"},
							payload, "type=dyncfg,user=test")

						// Second add (duplicate)
						sendDyncfgCmd(sd, "2-add",
							[]string{sd.dyncfgTemplateID(DiscovererNetListeners), "add", "test-job"},
							payload, "type=dyncfg,user=test")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: DiscovererNetListeners,
							name:           "test-job",
							sourceType:     "dyncfg",
							status:         dyncfg.StatusAccepted,
						},
					},
					wantDyncfg: `
CONFIG test:sd:net_listeners:test-job create accepted job /collectors/test/ServiceDiscovery dyncfg 'type=dyncfg,user=test' 'schema get enable disable update userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

FUNCTION_RESULT_BEGIN 2-add 400 application/json
{"status":400,"errorMessage":"Config 'net_listeners:test-job' already exists."}
FUNCTION_RESULT_END
`,
				}
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			sim := tc.createSim()
			sim.run(t)
		})
	}
}

func TestServiceDiscovery_DyncfgGet(t *testing.T) {
	tests := map[string]struct {
		createSim func() *dyncfgSim
	}{
		"get existing job": {
			createSim: func() *dyncfgSim {
				cfg := DyncfgNetListenersConfig{Name: "test-job"}
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Add first
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(DiscovererNetListeners), "add", "test-job"},
							payload, "type=dyncfg,user=test")

						// Get
						sendDyncfgCmd(sd, "2-get",
							[]string{sd.dyncfgJobID(DiscovererNetListeners, "test-job"), "get"},
							nil, "")
					},
					wantDyncfg: `
CONFIG test:sd:net_listeners:test-job create accepted job /collectors/test/ServiceDiscovery dyncfg 'type=dyncfg,user=test' 'schema get enable disable update userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

FUNCTION_RESULT_BEGIN 2-get 200 application/json
{"name":"test-job"}
FUNCTION_RESULT_END
`,
				}
			},
		},
		"get non-existent job fails": {
			createSim: func() *dyncfgSim {
				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						sendDyncfgCmd(sd, "1-get",
							[]string{sd.dyncfgJobID(DiscovererNetListeners, "non-existent"), "get"},
							nil, "")
					},
					wantDyncfg: `
FUNCTION_RESULT_BEGIN 1-get 404 application/json
{"status":404,"errorMessage":"Config 'net_listeners:non-existent' not found."}
FUNCTION_RESULT_END
`,
				}
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			sim := tc.createSim()
			sim.run(t)
		})
	}
}

func TestServiceDiscovery_DyncfgEnableDisable(t *testing.T) {
	tests := map[string]struct {
		createSim func() *dyncfgSim
	}{
		"enable starts pipeline": {
			createSim: func() *dyncfgSim {
				cfg := DyncfgNetListenersConfig{Name: "test-job"}
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Add
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(DiscovererNetListeners), "add", "test-job"},
							payload, "type=dyncfg,user=test")

						// Enable
						sendDyncfgCmd(sd, "2-enable",
							[]string{sd.dyncfgJobID(DiscovererNetListeners, "test-job"), "enable"},
							nil, "")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: DiscovererNetListeners,
							name:           "test-job",
							sourceType:     "dyncfg",
							status:         dyncfg.StatusRunning,
						},
					},
					wantRunning: []string{"dyncfg:net_listeners:test-job"},
					wantDyncfg: `
CONFIG test:sd:net_listeners:test-job create accepted job /collectors/test/ServiceDiscovery dyncfg 'type=dyncfg,user=test' 'schema get enable disable update userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:test-job status running

FUNCTION_RESULT_BEGIN 2-enable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END
`,
				}
			},
		},
		"disable stops pipeline": {
			createSim: func() *dyncfgSim {
				cfg := DyncfgNetListenersConfig{Name: "test-job"}
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Add
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(DiscovererNetListeners), "add", "test-job"},
							payload, "type=dyncfg,user=test")

						// Enable
						sendDyncfgCmd(sd, "2-enable",
							[]string{sd.dyncfgJobID(DiscovererNetListeners, "test-job"), "enable"},
							nil, "")

						// Disable
						sendDyncfgCmd(sd, "3-disable",
							[]string{sd.dyncfgJobID(DiscovererNetListeners, "test-job"), "disable"},
							nil, "")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: DiscovererNetListeners,
							name:           "test-job",
							sourceType:     "dyncfg",
							status:         dyncfg.StatusDisabled,
						},
					},
					wantRunning: []string{},
					wantDyncfg: `
CONFIG test:sd:net_listeners:test-job create accepted job /collectors/test/ServiceDiscovery dyncfg 'type=dyncfg,user=test' 'schema get enable disable update userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:test-job status running

FUNCTION_RESULT_BEGIN 2-enable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:test-job status disabled

FUNCTION_RESULT_BEGIN 3-disable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END
`,
				}
			},
		},
		"enable non-existent job fails": {
			createSim: func() *dyncfgSim {
				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						sendDyncfgCmd(sd, "1-enable",
							[]string{sd.dyncfgJobID(DiscovererNetListeners, "non-existent"), "enable"},
							nil, "")
					},
					wantDyncfg: `
FUNCTION_RESULT_BEGIN 1-enable 404 application/json
{"status":404,"errorMessage":"Config 'net_listeners:non-existent' not found."}
FUNCTION_RESULT_END
`,
				}
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			sim := tc.createSim()
			sim.run(t)
		})
	}
}

func TestServiceDiscovery_DyncfgUpdate(t *testing.T) {
	tests := map[string]struct {
		createSim func() *dyncfgSim
	}{
		"update existing job": {
			createSim: func() *dyncfgSim {
				cfg := DyncfgNetListenersConfig{Name: "test-job"}
				payload, _ := json.Marshal(cfg)

				updatedCfg := DyncfgNetListenersConfig{Name: "test-job", Interval: confopt.Duration(10 * time.Second)}
				updatedPayload, _ := json.Marshal(updatedCfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Add
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(DiscovererNetListeners), "add", "test-job"},
							payload, "type=dyncfg,user=test")

						// Update
						sendDyncfgCmd(sd, "2-update",
							[]string{sd.dyncfgJobID(DiscovererNetListeners, "test-job"), "update"},
							updatedPayload, "type=dyncfg,user=test")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: DiscovererNetListeners,
							name:           "test-job",
							sourceType:     "dyncfg",
							status:         dyncfg.StatusAccepted,
						},
					},
					wantDyncfg: `
CONFIG test:sd:net_listeners:test-job create accepted job /collectors/test/ServiceDiscovery dyncfg 'type=dyncfg,user=test' 'schema get enable disable update userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:test-job status accepted

FUNCTION_RESULT_BEGIN 2-update 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END
`,
				}
			},
		},
		"update non-existent job fails": {
			createSim: func() *dyncfgSim {
				cfg := DyncfgNetListenersConfig{Name: "test-job"}
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						sendDyncfgCmd(sd, "1-update",
							[]string{sd.dyncfgJobID(DiscovererNetListeners, "non-existent"), "update"},
							payload, "type=dyncfg,user=test")
					},
					wantDyncfg: `
FUNCTION_RESULT_BEGIN 1-update 404 application/json
{"status":404,"errorMessage":"Config 'net_listeners:non-existent' not found."}
FUNCTION_RESULT_END
`,
				}
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			sim := tc.createSim()
			sim.run(t)
		})
	}
}

func TestServiceDiscovery_DyncfgRemove(t *testing.T) {
	tests := map[string]struct {
		createSim func() *dyncfgSim
	}{
		"remove dyncfg job": {
			createSim: func() *dyncfgSim {
				cfg := DyncfgNetListenersConfig{Name: "test-job"}
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Add
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(DiscovererNetListeners), "add", "test-job"},
							payload, "type=dyncfg,user=test")

						// Remove
						sendDyncfgCmd(sd, "2-remove",
							[]string{sd.dyncfgJobID(DiscovererNetListeners, "test-job"), "remove"},
							nil, "")
					},
					wantExposed: []wantExposedConfig{},
					wantRunning: []string{},
					wantDyncfg: `
CONFIG test:sd:net_listeners:test-job create accepted job /collectors/test/ServiceDiscovery dyncfg 'type=dyncfg,user=test' 'schema get enable disable update userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:test-job delete

FUNCTION_RESULT_BEGIN 2-remove 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END
`,
				}
			},
		},
		"remove running job stops it first": {
			createSim: func() *dyncfgSim {
				cfg := DyncfgNetListenersConfig{Name: "test-job"}
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Add
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(DiscovererNetListeners), "add", "test-job"},
							payload, "type=dyncfg,user=test")

						// Enable
						sendDyncfgCmd(sd, "2-enable",
							[]string{sd.dyncfgJobID(DiscovererNetListeners, "test-job"), "enable"},
							nil, "")

						// Remove (should stop first)
						sendDyncfgCmd(sd, "3-remove",
							[]string{sd.dyncfgJobID(DiscovererNetListeners, "test-job"), "remove"},
							nil, "")
					},
					wantExposed: []wantExposedConfig{},
					wantRunning: []string{},
					wantDyncfg: `
CONFIG test:sd:net_listeners:test-job create accepted job /collectors/test/ServiceDiscovery dyncfg 'type=dyncfg,user=test' 'schema get enable disable update userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:test-job status running

FUNCTION_RESULT_BEGIN 2-enable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:test-job delete

FUNCTION_RESULT_BEGIN 3-remove 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END
`,
				}
			},
		},
		"remove non-existent job fails": {
			createSim: func() *dyncfgSim {
				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						sendDyncfgCmd(sd, "1-remove",
							[]string{sd.dyncfgJobID(DiscovererNetListeners, "non-existent"), "remove"},
							nil, "")
					},
					wantDyncfg: `
FUNCTION_RESULT_BEGIN 1-remove 404 application/json
{"status":404,"errorMessage":"Config 'net_listeners:non-existent' not found."}
FUNCTION_RESULT_END
`,
				}
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			sim := tc.createSim()
			sim.run(t)
		})
	}
}

func TestServiceDiscovery_DyncfgUserconfig(t *testing.T) {
	tests := map[string]struct {
		createSim func() *dyncfgSim
	}{
		"userconfig for template": {
			createSim: func() *dyncfgSim {
				cfg := DyncfgNetListenersConfig{Name: "test-job", Interval: confopt.Duration(5 * time.Second)}
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						sendDyncfgCmd(sd, "1-userconfig",
							[]string{sd.dyncfgTemplateID(DiscovererNetListeners), "userconfig"},
							payload, "")
					},
					wantDyncfgFunc: func(t *testing.T, got string) {
						assert.Contains(t, got, "FUNCTION_RESULT_BEGIN 1-userconfig 200 application/yaml")
						assert.Contains(t, got, "name: test-job")
						assert.Contains(t, got, "interval: 5")
						assert.Contains(t, got, "FUNCTION_RESULT_END")
					},
				}
			},
		},
		"userconfig for existing job": {
			createSim: func() *dyncfgSim {
				cfg := DyncfgNetListenersConfig{Name: "test-job", Interval: confopt.Duration(5 * time.Second)}
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Add first
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(DiscovererNetListeners), "add", "test-job"},
							payload, "type=dyncfg,user=test")

						// Userconfig
						sendDyncfgCmd(sd, "2-userconfig",
							[]string{sd.dyncfgJobID(DiscovererNetListeners, "test-job"), "userconfig"},
							nil, "")
					},
					wantDyncfgFunc: func(t *testing.T, got string) {
						assert.Contains(t, got, "FUNCTION_RESULT_BEGIN 1-add 202 application/json")
						assert.Contains(t, got, "FUNCTION_RESULT_BEGIN 2-userconfig 200 application/yaml")
						assert.Contains(t, got, "name: test-job")
						assert.Contains(t, got, "interval: 5")
					},
				}
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			sim := tc.createSim()
			sim.run(t)
		})
	}
}

func TestServiceDiscovery_DyncfgFileConfig(t *testing.T) {
	tests := map[string]struct {
		createSim func() *dyncfgSim
	}{
		"file config cannot be removed via dyncfg": {
			createSim: func() *dyncfgSim {
				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Manually add a file-based config to exposedConfigs
						sd.exposedConfigs.add(&sdConfig{
							discovererType: DiscovererNetListeners,
							name:           "file-config",
							pipelineKey:    "/etc/netdata/sd/test.conf",
							source:         "/etc/netdata/sd/test.conf",
							sourceType:     "file",
							status:         dyncfg.StatusRunning,
							content:        []byte(`{"name":"file-config"}`),
						})

						// Try to remove
						sendDyncfgCmd(sd, "1-remove",
							[]string{sd.dyncfgJobID(DiscovererNetListeners, "file-config"), "remove"},
							nil, "")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: DiscovererNetListeners,
							name:           "file-config",
							sourceType:     "file",
							status:         dyncfg.StatusRunning,
						},
					},
					wantDyncfg: `
FUNCTION_RESULT_BEGIN 1-remove 405 application/json
{"status":405,"errorMessage":"Cannot remove non-dyncfg configs. Source type: file"}
FUNCTION_RESULT_END
`,
				}
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			sim := tc.createSim()
			sim.run(t)
		})
	}
}

func TestServiceDiscovery_DyncfgDockerConfig(t *testing.T) {
	tests := map[string]struct {
		createSim func() *dyncfgSim
	}{
		"add docker job": {
			createSim: func() *dyncfgSim {
				cfg := DyncfgDockerConfig{
					Name:    "docker-test",
					Address: "unix:///var/run/docker.sock",
					Timeout: confopt.Duration(5 * time.Second),
					Services: []DyncfgServiceRule{
						{ID: "nginx", Match: `{{ glob .Image "*nginx*" }}`},
					},
				}
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(DiscovererDocker), "add", "docker-test"},
							payload, "type=dyncfg,user=test")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: DiscovererDocker,
							name:           "docker-test",
							sourceType:     "dyncfg",
							status:         dyncfg.StatusAccepted,
						},
					},
					wantDyncfg: `
CONFIG test:sd:docker:docker-test create accepted job /collectors/test/ServiceDiscovery dyncfg 'type=dyncfg,user=test' 'schema get enable disable update userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END
`,
				}
			},
		},
		"add and enable docker job": {
			createSim: func() *dyncfgSim {
				cfg := DyncfgDockerConfig{
					Name:    "docker-test",
					Address: "tcp://localhost:2375",
				}
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Add
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(DiscovererDocker), "add", "docker-test"},
							payload, "type=dyncfg,user=test")

						// Enable
						sendDyncfgCmd(sd, "2-enable",
							[]string{sd.dyncfgJobID(DiscovererDocker, "docker-test"), "enable"},
							nil, "")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: DiscovererDocker,
							name:           "docker-test",
							sourceType:     "dyncfg",
							status:         dyncfg.StatusRunning,
						},
					},
					wantRunning: []string{"dyncfg:docker:docker-test"},
				}
			},
		},
		"get docker job config": {
			createSim: func() *dyncfgSim {
				cfg := DyncfgDockerConfig{
					Name:    "docker-test",
					Address: "unix:///var/run/docker.sock",
				}
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Add
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(DiscovererDocker), "add", "docker-test"},
							payload, "type=dyncfg,user=test")

						// Get
						sendDyncfgCmd(sd, "2-get",
							[]string{sd.dyncfgJobID(DiscovererDocker, "docker-test"), "get"},
							nil, "")
					},
					wantDyncfgFunc: func(t *testing.T, got string) {
						assert.Contains(t, got, "FUNCTION_RESULT_BEGIN 2-get 200 application/json")
						assert.Contains(t, got, `"name":"docker-test"`)
						assert.Contains(t, got, `"address":"unix:///var/run/docker.sock"`)
					},
				}
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			sim := tc.createSim()
			sim.run(t)
		})
	}
}

func TestServiceDiscovery_DyncfgK8sConfig(t *testing.T) {
	tests := map[string]struct {
		createSim func() *dyncfgSim
	}{
		"add k8s job": {
			createSim: func() *dyncfgSim {
				cfg := DyncfgK8sConfig{
					Name:       "k8s-test",
					Role:       "pod",
					Namespaces: []string{"default", "kube-system"},
					Selector: &DyncfgK8sSelector{
						Label: "app=nginx",
					},
					Pod: &DyncfgK8sPodOptions{
						LocalMode: true,
					},
					Services: []DyncfgServiceRule{
						{ID: "nginx-pods", Match: `{{ eq .Namespace "default" }}`},
					},
				}
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(DiscovererK8s), "add", "k8s-test"},
							payload, "type=dyncfg,user=test")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: DiscovererK8s,
							name:           "k8s-test",
							sourceType:     "dyncfg",
							status:         dyncfg.StatusAccepted,
						},
					},
					wantDyncfg: `
CONFIG test:sd:k8s:k8s-test create accepted job /collectors/test/ServiceDiscovery dyncfg 'type=dyncfg,user=test' 'schema get enable disable update userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END
`,
				}
			},
		},
		"add k8s service role job": {
			createSim: func() *dyncfgSim {
				cfg := DyncfgK8sConfig{
					Name: "k8s-svc-test",
					Role: "service",
				}
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Add
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(DiscovererK8s), "add", "k8s-svc-test"},
							payload, "type=dyncfg,user=test")

						// Enable
						sendDyncfgCmd(sd, "2-enable",
							[]string{sd.dyncfgJobID(DiscovererK8s, "k8s-svc-test"), "enable"},
							nil, "")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: DiscovererK8s,
							name:           "k8s-svc-test",
							sourceType:     "dyncfg",
							status:         dyncfg.StatusRunning,
						},
					},
					wantRunning: []string{"dyncfg:k8s:k8s-svc-test"},
				}
			},
		},
		"get k8s job config": {
			createSim: func() *dyncfgSim {
				cfg := DyncfgK8sConfig{
					Name:       "k8s-test",
					Role:       "pod",
					Namespaces: []string{"default"},
				}
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Add
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(DiscovererK8s), "add", "k8s-test"},
							payload, "type=dyncfg,user=test")

						// Get
						sendDyncfgCmd(sd, "2-get",
							[]string{sd.dyncfgJobID(DiscovererK8s, "k8s-test"), "get"},
							nil, "")
					},
					wantDyncfgFunc: func(t *testing.T, got string) {
						assert.Contains(t, got, "FUNCTION_RESULT_BEGIN 2-get 200 application/json")
						assert.Contains(t, got, `"name":"k8s-test"`)
						assert.Contains(t, got, `"role":"pod"`)
						assert.Contains(t, got, `"namespaces":["default"]`)
					},
				}
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			sim := tc.createSim()
			sim.run(t)
		})
	}
}

func TestServiceDiscovery_DyncfgSNMPConfig(t *testing.T) {
	tests := map[string]struct {
		createSim func() *dyncfgSim
	}{
		"add snmp job": {
			createSim: func() *dyncfgSim {
				cfg := DyncfgSNMPConfig{
					Name:           "snmp-test",
					RescanInterval: confopt.Duration(30 * time.Minute),
					Timeout:        confopt.Duration(1 * time.Second),
					DeviceCacheTTL: confopt.Duration(12 * time.Hour),
					Credentials: []DyncfgSNMPCredential{
						{Name: "public-v2", Version: "2c", Community: "public"},
					},
					Networks: []DyncfgSNMPNetwork{
						{Subnet: "192.168.1.0/24", Credential: "public-v2"},
					},
				}
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(DiscovererSNMP), "add", "snmp-test"},
							payload, "type=dyncfg,user=test")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: DiscovererSNMP,
							name:           "snmp-test",
							sourceType:     "dyncfg",
							status:         dyncfg.StatusAccepted,
						},
					},
					wantDyncfg: `
CONFIG test:sd:snmp:snmp-test create accepted job /collectors/test/ServiceDiscovery dyncfg 'type=dyncfg,user=test' 'schema get enable disable update userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END
`,
				}
			},
		},
		"add snmp v3 job": {
			createSim: func() *dyncfgSim {
				cfg := DyncfgSNMPConfig{
					Name: "snmp-v3-test",
					Credentials: []DyncfgSNMPCredential{
						{
							Name:          "snmpv3-auth",
							Version:       "3",
							Username:      "admin",
							SecurityLevel: "authPriv",
							AuthProtocol:  "sha256",
							AuthPassword:  "authpass",
							PrivProtocol:  "aes",
							PrivPassword:  "privpass",
						},
					},
					Networks: []DyncfgSNMPNetwork{
						{Subnet: "10.0.0.0/24", Credential: "snmpv3-auth"},
					},
				}
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Add
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(DiscovererSNMP), "add", "snmp-v3-test"},
							payload, "type=dyncfg,user=test")

						// Enable
						sendDyncfgCmd(sd, "2-enable",
							[]string{sd.dyncfgJobID(DiscovererSNMP, "snmp-v3-test"), "enable"},
							nil, "")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: DiscovererSNMP,
							name:           "snmp-v3-test",
							sourceType:     "dyncfg",
							status:         dyncfg.StatusRunning,
						},
					},
					wantRunning: []string{"dyncfg:snmp:snmp-v3-test"},
				}
			},
		},
		"get snmp job config": {
			createSim: func() *dyncfgSim {
				cfg := DyncfgSNMPConfig{
					Name:           "snmp-test",
					RescanInterval: confopt.Duration(1 * time.Hour),
					Credentials: []DyncfgSNMPCredential{
						{Name: "v2-cred", Version: "2c", Community: "public"},
					},
					Networks: []DyncfgSNMPNetwork{
						{Subnet: "192.168.0.0/16", Credential: "v2-cred"},
					},
				}
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Add
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(DiscovererSNMP), "add", "snmp-test"},
							payload, "type=dyncfg,user=test")

						// Get
						sendDyncfgCmd(sd, "2-get",
							[]string{sd.dyncfgJobID(DiscovererSNMP, "snmp-test"), "get"},
							nil, "")
					},
					wantDyncfgFunc: func(t *testing.T, got string) {
						assert.Contains(t, got, "FUNCTION_RESULT_BEGIN 2-get 200 application/json")
						assert.Contains(t, got, `"name":"snmp-test"`)
						assert.Contains(t, got, `"rescan_interval":3600`) // 1 hour in seconds
						assert.Contains(t, got, `"subnet":"192.168.0.0/16"`)
					},
				}
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			sim := tc.createSim()
			sim.run(t)
		})
	}
}

func TestServiceDiscovery_DyncfgUpdateWhileRunning(t *testing.T) {
	tests := map[string]struct {
		createSim func() *dyncfgSim
	}{
		"update running pipeline restarts it": {
			createSim: func() *dyncfgSim {
				cfg := DyncfgNetListenersConfig{Name: "test-job", Interval: confopt.Duration(5 * time.Second)}
				payload, _ := json.Marshal(cfg)

				updatedCfg := DyncfgNetListenersConfig{Name: "test-job", Interval: confopt.Duration(10 * time.Second)}
				updatedPayload, _ := json.Marshal(updatedCfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Add
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(DiscovererNetListeners), "add", "test-job"},
							payload, "type=dyncfg,user=test")

						// Enable (starts pipeline)
						sendDyncfgCmd(sd, "2-enable",
							[]string{sd.dyncfgJobID(DiscovererNetListeners, "test-job"), "enable"},
							nil, "")

						// Update while running (should restart pipeline)
						sendDyncfgCmd(sd, "3-update",
							[]string{sd.dyncfgJobID(DiscovererNetListeners, "test-job"), "update"},
							updatedPayload, "type=dyncfg,user=test")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: DiscovererNetListeners,
							name:           "test-job",
							sourceType:     "dyncfg",
							status:         dyncfg.StatusRunning,
						},
					},
					wantRunning: []string{"dyncfg:net_listeners:test-job"},
					wantDyncfg: `
CONFIG test:sd:net_listeners:test-job create accepted job /collectors/test/ServiceDiscovery dyncfg 'type=dyncfg,user=test' 'schema get enable disable update userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:test-job status running

FUNCTION_RESULT_BEGIN 2-enable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:test-job status running

FUNCTION_RESULT_BEGIN 3-update 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END
`,
				}
			},
		},
		"update running docker pipeline": {
			createSim: func() *dyncfgSim {
				cfg := DyncfgDockerConfig{Name: "docker-job", Address: "unix:///var/run/docker.sock"}
				payload, _ := json.Marshal(cfg)

				updatedCfg := DyncfgDockerConfig{Name: "docker-job", Address: "tcp://localhost:2375"}
				updatedPayload, _ := json.Marshal(updatedCfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Add
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(DiscovererDocker), "add", "docker-job"},
							payload, "type=dyncfg,user=test")

						// Enable
						sendDyncfgCmd(sd, "2-enable",
							[]string{sd.dyncfgJobID(DiscovererDocker, "docker-job"), "enable"},
							nil, "")

						// Update while running
						sendDyncfgCmd(sd, "3-update",
							[]string{sd.dyncfgJobID(DiscovererDocker, "docker-job"), "update"},
							updatedPayload, "type=dyncfg,user=test")

						// Verify config was updated
						sendDyncfgCmd(sd, "4-get",
							[]string{sd.dyncfgJobID(DiscovererDocker, "docker-job"), "get"},
							nil, "")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: DiscovererDocker,
							name:           "docker-job",
							sourceType:     "dyncfg",
							status:         dyncfg.StatusRunning,
						},
					},
					wantRunning: []string{"dyncfg:docker:docker-job"},
					wantDyncfgFunc: func(t *testing.T, got string) {
						// Verify update response
						assert.Contains(t, got, "FUNCTION_RESULT_BEGIN 3-update 200 application/json")
						// Verify config was updated to new address
						assert.Contains(t, got, "FUNCTION_RESULT_BEGIN 4-get 200 application/json")
						assert.Contains(t, got, `"address":"tcp://localhost:2375"`)
					},
				}
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			sim := tc.createSim()
			sim.run(t)
		})
	}
}

func TestServiceDiscovery_DyncfgTest(t *testing.T) {
	tests := map[string]struct {
		createSim func() *dyncfgSim
	}{
		"test valid config succeeds": {
			createSim: func() *dyncfgSim {
				cfg := DyncfgNetListenersConfig{Name: "test-job", Interval: confopt.Duration(5 * time.Second)}
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						sendDyncfgCmd(sd, "1-test",
							[]string{sd.dyncfgTemplateID(DiscovererNetListeners), "test"},
							payload, "")
					},
					wantExposed: []wantExposedConfig{},
					wantRunning: []string{},
					wantDyncfg: `
FUNCTION_RESULT_BEGIN 1-test 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END
`,
				}
			},
		},
		"test without payload fails": {
			createSim: func() *dyncfgSim {
				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						sendDyncfgCmd(sd, "1-test",
							[]string{sd.dyncfgTemplateID(DiscovererNetListeners), "test"},
							nil, "")
					},
					wantExposed: []wantExposedConfig{},
					wantRunning: []string{},
					wantDyncfg: `
FUNCTION_RESULT_BEGIN 1-test 400 application/json
{"status":400,"errorMessage":"missing configuration payload"}
FUNCTION_RESULT_END
`,
				}
			},
		},
		"test valid docker config": {
			createSim: func() *dyncfgSim {
				cfg := DyncfgDockerConfig{
					Name:    "docker-test",
					Address: "unix:///var/run/docker.sock",
				}
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						sendDyncfgCmd(sd, "1-test",
							[]string{sd.dyncfgTemplateID(DiscovererDocker), "test"},
							payload, "")
					},
					wantExposed: []wantExposedConfig{},
					wantRunning: []string{},
					wantDyncfg: `
FUNCTION_RESULT_BEGIN 1-test 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END
`,
				}
			},
		},
		"test valid k8s config": {
			createSim: func() *dyncfgSim {
				cfg := DyncfgK8sConfig{
					Name: "k8s-test",
					Role: "pod",
				}
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						sendDyncfgCmd(sd, "1-test",
							[]string{sd.dyncfgTemplateID(DiscovererK8s), "test"},
							payload, "")
					},
					wantExposed: []wantExposedConfig{},
					wantRunning: []string{},
					wantDyncfg: `
FUNCTION_RESULT_BEGIN 1-test 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END
`,
				}
			},
		},
		"test valid snmp config": {
			createSim: func() *dyncfgSim {
				cfg := DyncfgSNMPConfig{
					Name: "snmp-test",
					Credentials: []DyncfgSNMPCredential{
						{Name: "v2-cred", Version: "2c"},
					},
					Networks: []DyncfgSNMPNetwork{
						{Subnet: "192.168.1.0/24", Credential: "v2-cred"},
					},
				}
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						sendDyncfgCmd(sd, "1-test",
							[]string{sd.dyncfgTemplateID(DiscovererSNMP), "test"},
							payload, "")
					},
					wantExposed: []wantExposedConfig{},
					wantRunning: []string{},
					wantDyncfg: `
FUNCTION_RESULT_BEGIN 1-test 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END
`,
				}
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			sim := tc.createSim()
			sim.run(t)
		})
	}
}

func TestServiceDiscovery_DyncfgMultipleJobs(t *testing.T) {
	tests := map[string]struct {
		createSim func() *dyncfgSim
	}{
		"multiple jobs lifecycle": {
			createSim: func() *dyncfgSim {
				cfg1 := DyncfgNetListenersConfig{Name: "job1"}
				cfg2 := DyncfgNetListenersConfig{Name: "job2"}
				payload1, _ := json.Marshal(cfg1)
				payload2, _ := json.Marshal(cfg2)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Add job1
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(DiscovererNetListeners), "add", "job1"},
							payload1, "type=dyncfg,user=test")

						// Add job2
						sendDyncfgCmd(sd, "2-add",
							[]string{sd.dyncfgTemplateID(DiscovererNetListeners), "add", "job2"},
							payload2, "type=dyncfg,user=test")

						// Enable both
						sendDyncfgCmd(sd, "3-enable",
							[]string{sd.dyncfgJobID(DiscovererNetListeners, "job1"), "enable"},
							nil, "")
						sendDyncfgCmd(sd, "4-enable",
							[]string{sd.dyncfgJobID(DiscovererNetListeners, "job2"), "enable"},
							nil, "")

						// Disable job1
						sendDyncfgCmd(sd, "5-disable",
							[]string{sd.dyncfgJobID(DiscovererNetListeners, "job1"), "disable"},
							nil, "")

						// Remove job2
						sendDyncfgCmd(sd, "6-remove",
							[]string{sd.dyncfgJobID(DiscovererNetListeners, "job2"), "remove"},
							nil, "")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: DiscovererNetListeners,
							name:           "job1",
							sourceType:     "dyncfg",
							status:         dyncfg.StatusDisabled,
						},
					},
					wantRunning: []string{},
					wantDyncfg: `
CONFIG test:sd:net_listeners:job1 create accepted job /collectors/test/ServiceDiscovery dyncfg 'type=dyncfg,user=test' 'schema get enable disable update userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:job2 create accepted job /collectors/test/ServiceDiscovery dyncfg 'type=dyncfg,user=test' 'schema get enable disable update userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 2-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:job1 status running

FUNCTION_RESULT_BEGIN 3-enable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:job2 status running

FUNCTION_RESULT_BEGIN 4-enable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:job1 status disabled

FUNCTION_RESULT_BEGIN 5-disable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:job2 delete

FUNCTION_RESULT_BEGIN 6-remove 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END
`,
				}
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			sim := tc.createSim()
			sim.run(t)
		})
	}
}

// testPipeline is a simple pipeline for testing that just waits for cancellation.
type testPipeline struct {
	name string
}

func newTestPipeline(name string) *testPipeline {
	return &testPipeline{name: name}
}

func (p *testPipeline) Run(ctx context.Context, out chan<- []*confgroup.Group) {
	<-ctx.Done()
}
