// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/safewriter"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/pipeline"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper functions to create test configs using pipeline.Config

// defaultTestServices returns a minimal valid service rule for tests.
func defaultTestServices() []pipeline.ServiceRuleConfig {
	return []pipeline.ServiceRuleConfig{
		{ID: "test-rule", Match: "true"},
	}
}

func mustDiscovererPayload(typ string, cfg any) pipeline.DiscovererPayload {
	p, err := pipeline.NewDiscovererPayload(typ, cfg)
	if err != nil {
		panic(err)
	}
	return p
}

func newTestNetListenersConfig(name string, interval confopt.LongDuration, timeout confopt.Duration, services []pipeline.ServiceRuleConfig) pipeline.Config {
	return pipeline.Config{
		Name: name,
		Discoverer: mustDiscovererPayload(testDiscovererTypeNetListeners, testNetListenersConfig{
			Interval: interval,
			Timeout:  timeout,
		}),
		Services: services,
	}
}

func newTestDockerConfig(name, address string, timeout confopt.Duration, services []pipeline.ServiceRuleConfig) pipeline.Config {
	return pipeline.Config{
		Name: name,
		Discoverer: mustDiscovererPayload(testDiscovererTypeDocker, testDockerConfig{
			Address: address,
			Timeout: timeout,
		}),
		Services: services,
	}
}

func newTestK8sConfig(name string, cfgs []testK8sConfig, services []pipeline.ServiceRuleConfig) pipeline.Config {
	return pipeline.Config{
		Name:       name,
		Discoverer: mustDiscovererPayload(testDiscovererTypeK8s, cfgs),
		Services:   services,
	}
}

func newTestSNMPConfig(name string, cfg testSNMPConfig, services []pipeline.ServiceRuleConfig) pipeline.Config {
	return pipeline.Config{
		Name:       name,
		Discoverer: mustDiscovererPayload(testDiscovererTypeSNMP, cfg),
		Services:   services,
	}
}

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
		Logger:      logger.New(),
		pluginName:  testPluginName,
		dyncfgApi:   dyncfg.NewResponder(netdataapi.New(safewriter.New(&buf))),
		seen:        dyncfg.NewSeenCache[sdConfig](),
		exposed:     dyncfg.NewExposedCache[sdConfig](),
		dyncfgCh:    make(chan dyncfg.Function, 1),
		discoverers: testDiscovererRegistry(),
		newPipeline: func(cfg pipeline.Config) (sdPipeline, error) {
			return newTestPipeline(cfg.Name), nil
		},
	}
	sd.sdCb = &sdCallbacks{sd: sd}
	sd.handler = dyncfg.NewHandler(dyncfg.HandlerOpts[sdConfig]{
		Logger:    sd.Logger,
		API:       sd.dyncfgApi,
		Seen:      sd.seen,
		Exposed:   sd.exposed,
		Callbacks: sd.sdCb,

		Path:           fmt.Sprintf(dyncfgSDPath, testPluginName),
		EnableFailCode: 422,
		JobCommands: []dyncfg.Command{
			dyncfg.CommandSchema,
			dyncfg.CommandGet,
			dyncfg.CommandEnable,
			dyncfg.CommandDisable,
			dyncfg.CommandUpdate,
			dyncfg.CommandTest,
			dyncfg.CommandUserconfig,
		},
	})

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
		wantLen, gotLen := len(s.wantExposed), sd.exposed.Count()
		require.Equalf(t, wantLen, gotLen, "exposedConfigs: different len (want %d got %d)", wantLen, gotLen)

		for _, want := range s.wantExposed {
			entry, ok := sd.exposed.LookupByKey(want.discovererType + ":" + want.name)
			require.Truef(t, ok, "exposedConfigs: config '%s:%s' not found", want.discovererType, want.name)
			assert.Equal(t, want.sourceType, entry.Cfg.SourceType(), "exposedConfigs: wrong sourceType for '%s:%s'", want.discovererType, want.name)
			assert.Equal(t, want.status, entry.Status, "exposedConfigs: wrong status for '%s:%s'", want.discovererType, want.name)
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
							[]string{sd.dyncfgTemplateID(testDiscovererTypeNetListeners), "schema"},
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
				cfg := newTestNetListenersConfig("test-job", 0, 0, defaultTestServices())
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeNetListeners), "add", "test-job"},
							payload, "type=dyncfg,user=test")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: testDiscovererTypeNetListeners,
							name:           "test-job",
							sourceType:     "dyncfg",
							status:         dyncfg.StatusAccepted,
						},
					},
					wantDyncfg: `
FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:test-job create accepted job /collectors/test/ServiceDiscovery dyncfg 'type=dyncfg,user=test' 'schema get enable disable update test userconfig remove' 0x0000 0x0000
`,
				}
			},
		},
		"add without payload fails": {
			createSim: func() *dyncfgSim {
				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeNetListeners), "add", "test-job"},
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
		"add duplicate job replaces existing": {
			createSim: func() *dyncfgSim {
				cfg := newTestNetListenersConfig("test-job", 0, 0, defaultTestServices())
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// First add
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeNetListeners), "add", "test-job"},
							payload, "type=dyncfg,user=test")

						// Second add (replaces first - matching jobmgr pattern)
						sendDyncfgCmd(sd, "2-add",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeNetListeners), "add", "test-job"},
							payload, "type=dyncfg,user=test")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: testDiscovererTypeNetListeners,
							name:           "test-job",
							sourceType:     "dyncfg",
							status:         dyncfg.StatusAccepted,
						},
					},
					wantDyncfg: `
FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:test-job create accepted job /collectors/test/ServiceDiscovery dyncfg 'type=dyncfg,user=test' 'schema get enable disable update test userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 2-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:test-job create accepted job /collectors/test/ServiceDiscovery dyncfg 'type=dyncfg,user=test' 'schema get enable disable update test userconfig remove' 0x0000 0x0000
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
				cfg := newTestNetListenersConfig("test-job", 0, 0, defaultTestServices())
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Add first
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeNetListeners), "add", "test-job"},
							payload, "type=dyncfg,user=test")

						// Get
						sendDyncfgCmd(sd, "2-get",
							[]string{sd.dyncfgJobID(testDiscovererTypeNetListeners, "test-job"), "get"},
							nil, "")
					},
					wantDyncfgFunc: func(t *testing.T, got string) {
						// Check that CONFIG and FUNCTION_RESULT lines are present
						assert.Contains(t, got, "CONFIG test:sd:net_listeners:test-job create accepted job")
						assert.Contains(t, got, "FUNCTION_RESULT_BEGIN 1-add 202 application/json")
						assert.Contains(t, got, "FUNCTION_RESULT_BEGIN 2-get 200 application/json")
						// JSON key order may vary, so check for presence of expected fields
						assert.Contains(t, got, `"name":"test-job"`)
						assert.Contains(t, got, `"discoverer":{`)
						assert.Contains(t, got, `"net_listeners":{}`)
					},
				}
			},
		},
		"get non-existent job fails": {
			createSim: func() *dyncfgSim {
				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						sendDyncfgCmd(sd, "1-get",
							[]string{sd.dyncfgJobID(testDiscovererTypeNetListeners, "non-existent"), "get"},
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
				cfg := newTestNetListenersConfig("test-job", 0, 0, defaultTestServices())
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Add
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeNetListeners), "add", "test-job"},
							payload, "type=dyncfg,user=test")

						// Enable
						sendDyncfgCmd(sd, "2-enable",
							[]string{sd.dyncfgJobID(testDiscovererTypeNetListeners, "test-job"), "enable"},
							nil, "")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: testDiscovererTypeNetListeners,
							name:           "test-job",
							sourceType:     "dyncfg",
							status:         dyncfg.StatusRunning,
						},
					},
					wantRunning: []string{"dyncfg:net_listeners:test-job"},
					wantDyncfg: `
FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:test-job create accepted job /collectors/test/ServiceDiscovery dyncfg 'type=dyncfg,user=test' 'schema get enable disable update test userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 2-enable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:test-job status running
`,
				}
			},
		},
		"disable stops pipeline": {
			createSim: func() *dyncfgSim {
				cfg := newTestNetListenersConfig("test-job", 0, 0, defaultTestServices())
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Add
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeNetListeners), "add", "test-job"},
							payload, "type=dyncfg,user=test")

						// Enable
						sendDyncfgCmd(sd, "2-enable",
							[]string{sd.dyncfgJobID(testDiscovererTypeNetListeners, "test-job"), "enable"},
							nil, "")

						// Disable
						sendDyncfgCmd(sd, "3-disable",
							[]string{sd.dyncfgJobID(testDiscovererTypeNetListeners, "test-job"), "disable"},
							nil, "")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: testDiscovererTypeNetListeners,
							name:           "test-job",
							sourceType:     "dyncfg",
							status:         dyncfg.StatusDisabled,
						},
					},
					wantRunning: []string{},
					wantDyncfg: `
FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:test-job create accepted job /collectors/test/ServiceDiscovery dyncfg 'type=dyncfg,user=test' 'schema get enable disable update test userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 2-enable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:test-job status running

FUNCTION_RESULT_BEGIN 3-disable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:test-job status disabled
`,
				}
			},
		},
		"enable non-existent job fails": {
			createSim: func() *dyncfgSim {
				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						sendDyncfgCmd(sd, "1-enable",
							[]string{sd.dyncfgJobID(testDiscovererTypeNetListeners, "non-existent"), "enable"},
							nil, "")
					},
					wantDyncfg: `
FUNCTION_RESULT_BEGIN 1-enable 404 application/json
{"status":404,"errorMessage":"job not found."}
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
		"update disabled job": {
			createSim: func() *dyncfgSim {
				cfg := newTestNetListenersConfig("test-job", 0, 0, defaultTestServices())
				payload, _ := json.Marshal(cfg)

				updatedCfg := newTestNetListenersConfig("test-job", confopt.LongDuration(10*time.Second), 0, defaultTestServices())
				updatedPayload, _ := json.Marshal(updatedCfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Add
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeNetListeners), "add", "test-job"},
							payload, "type=dyncfg,user=test")

						// Enable then disable to get to Disabled state
						sendDyncfgCmd(sd, "2-enable",
							[]string{sd.dyncfgJobID(testDiscovererTypeNetListeners, "test-job"), "enable"},
							nil, "")

						sendDyncfgCmd(sd, "3-disable",
							[]string{sd.dyncfgJobID(testDiscovererTypeNetListeners, "test-job"), "disable"},
							nil, "")

						// Update (should work in Disabled state)
						sendDyncfgCmd(sd, "4-update",
							[]string{sd.dyncfgJobID(testDiscovererTypeNetListeners, "test-job"), "update"},
							updatedPayload, "type=dyncfg,user=test")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: testDiscovererTypeNetListeners,
							name:           "test-job",
							sourceType:     "dyncfg",
							status:         dyncfg.StatusDisabled,
						},
					},
					wantDyncfg: `
FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:test-job create accepted job /collectors/test/ServiceDiscovery dyncfg 'type=dyncfg,user=test' 'schema get enable disable update test userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 2-enable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:test-job status running

FUNCTION_RESULT_BEGIN 3-disable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:test-job status disabled

FUNCTION_RESULT_BEGIN 4-update 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:test-job status disabled
`,
				}
			},
		},
		"update in accepted state fails": {
			createSim: func() *dyncfgSim {
				cfg := newTestNetListenersConfig("test-job", 0, 0, defaultTestServices())
				payload, _ := json.Marshal(cfg)

				updatedCfg := newTestNetListenersConfig("test-job", confopt.LongDuration(10*time.Second), 0, defaultTestServices())
				updatedPayload, _ := json.Marshal(updatedCfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Add (creates in Accepted state)
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeNetListeners), "add", "test-job"},
							payload, "type=dyncfg,user=test")

						// Update in Accepted state should fail with 403
						sendDyncfgCmd(sd, "2-update",
							[]string{sd.dyncfgJobID(testDiscovererTypeNetListeners, "test-job"), "update"},
							updatedPayload, "type=dyncfg,user=test")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: testDiscovererTypeNetListeners,
							name:           "test-job",
							sourceType:     "dyncfg",
							status:         dyncfg.StatusAccepted,
						},
					},
					wantDyncfg: `
FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:test-job create accepted job /collectors/test/ServiceDiscovery dyncfg 'type=dyncfg,user=test' 'schema get enable disable update test userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 2-update 403 application/json
{"status":403,"errorMessage":"updating is not allowed in 'accepted' state."}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:test-job status accepted
`,
				}
			},
		},
		"update non-existent job fails": {
			createSim: func() *dyncfgSim {
				cfg := newTestNetListenersConfig("test-job", 0, 0, defaultTestServices())
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						sendDyncfgCmd(sd, "1-update",
							[]string{sd.dyncfgJobID(testDiscovererTypeNetListeners, "non-existent"), "update"},
							payload, "type=dyncfg,user=test")
					},
					wantDyncfg: `
FUNCTION_RESULT_BEGIN 1-update 404 application/json
{"status":404,"errorMessage":"job not found."}
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
				cfg := newTestNetListenersConfig("test-job", 0, 0, defaultTestServices())
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Add
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeNetListeners), "add", "test-job"},
							payload, "type=dyncfg,user=test")

						// Remove
						sendDyncfgCmd(sd, "2-remove",
							[]string{sd.dyncfgJobID(testDiscovererTypeNetListeners, "test-job"), "remove"},
							nil, "")
					},
					wantExposed: []wantExposedConfig{},
					wantRunning: []string{},
					wantDyncfg: `
FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:test-job create accepted job /collectors/test/ServiceDiscovery dyncfg 'type=dyncfg,user=test' 'schema get enable disable update test userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 2-remove 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:test-job delete
`,
				}
			},
		},
		"remove running job stops it first": {
			createSim: func() *dyncfgSim {
				cfg := newTestNetListenersConfig("test-job", 0, 0, defaultTestServices())
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Add
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeNetListeners), "add", "test-job"},
							payload, "type=dyncfg,user=test")

						// Enable
						sendDyncfgCmd(sd, "2-enable",
							[]string{sd.dyncfgJobID(testDiscovererTypeNetListeners, "test-job"), "enable"},
							nil, "")

						// Remove (should stop first)
						sendDyncfgCmd(sd, "3-remove",
							[]string{sd.dyncfgJobID(testDiscovererTypeNetListeners, "test-job"), "remove"},
							nil, "")
					},
					wantExposed: []wantExposedConfig{},
					wantRunning: []string{},
					wantDyncfg: `
FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:test-job create accepted job /collectors/test/ServiceDiscovery dyncfg 'type=dyncfg,user=test' 'schema get enable disable update test userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 2-enable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:test-job status running

FUNCTION_RESULT_BEGIN 3-remove 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:test-job delete
`,
				}
			},
		},
		"remove non-existent job fails": {
			createSim: func() *dyncfgSim {
				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						sendDyncfgCmd(sd, "1-remove",
							[]string{sd.dyncfgJobID(testDiscovererTypeNetListeners, "non-existent"), "remove"},
							nil, "")
					},
					wantDyncfg: `
FUNCTION_RESULT_BEGIN 1-remove 404 application/json
{"status":404,"errorMessage":"job not found."}
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
				cfg := newTestNetListenersConfig("test-job", confopt.LongDuration(5*time.Second), 0, defaultTestServices())
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						sendDyncfgCmd(sd, "1-userconfig",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeNetListeners), "userconfig"},
							payload, "")
					},
					wantDyncfgFunc: func(t *testing.T, got string) {
						assert.Contains(t, got, "FUNCTION_RESULT_BEGIN 1-userconfig 200 application/yaml")
						assert.Contains(t, got, "name: test-job")
						assert.Contains(t, got, "discoverer:")
						assert.Contains(t, got, "net_listeners:")
						assert.Contains(t, got, "interval: 5")
						assert.Contains(t, got, "FUNCTION_RESULT_END")
					},
				}
			},
		},
		"userconfig for existing job": {
			createSim: func() *dyncfgSim {
				cfg := newTestNetListenersConfig("test-job", confopt.LongDuration(5*time.Second), 0, defaultTestServices())
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Add first
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeNetListeners), "add", "test-job"},
							payload, "type=dyncfg,user=test")

						// Userconfig - must provide payload (matching jobmgr pattern)
						sendDyncfgCmd(sd, "2-userconfig",
							[]string{sd.dyncfgJobID(testDiscovererTypeNetListeners, "test-job"), "userconfig"},
							payload, "")
					},
					wantDyncfgFunc: func(t *testing.T, got string) {
						assert.Contains(t, got, "FUNCTION_RESULT_BEGIN 1-add 202 application/json")
						assert.Contains(t, got, "FUNCTION_RESULT_BEGIN 2-userconfig 200 application/yaml")
						assert.Contains(t, got, "name: test-job")
						assert.Contains(t, got, "discoverer:")
						assert.Contains(t, got, "net_listeners:")
						assert.Contains(t, got, "interval: 5")
					},
				}
			},
		},
		"userconfig for docker template": {
			createSim: func() *dyncfgSim {
				cfg := newTestDockerConfig("docker-test", "unix:///var/run/docker.sock", confopt.Duration(5*time.Second), defaultTestServices())
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						sendDyncfgCmd(sd, "1-userconfig",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeDocker), "userconfig"},
							payload, "")
					},
					wantDyncfgFunc: func(t *testing.T, got string) {
						assert.Contains(t, got, "FUNCTION_RESULT_BEGIN 1-userconfig 200 application/yaml")
						assert.Contains(t, got, "name: docker-test")
						assert.Contains(t, got, "discoverer:")
						assert.Contains(t, got, "docker:")
						assert.Contains(t, got, "address: unix:///var/run/docker.sock")
					},
				}
			},
		},
		"userconfig for k8s template": {
			createSim: func() *dyncfgSim {
				cfg := newTestK8sConfig("k8s-test", []testK8sConfig{{
					Role:       "pod",
					Namespaces: []string{"default"},
				}}, defaultTestServices())
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						sendDyncfgCmd(sd, "1-userconfig",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeK8s), "userconfig"},
							payload, "")
					},
					wantDyncfgFunc: func(t *testing.T, got string) {
						assert.Contains(t, got, "FUNCTION_RESULT_BEGIN 1-userconfig 200 application/yaml")
						assert.Contains(t, got, "name: k8s-test")
						assert.Contains(t, got, "discoverer:")
						assert.Contains(t, got, "k8s:")
						assert.Contains(t, got, "role: pod")
						assert.Contains(t, got, "- default")
					},
				}
			},
		},
		"userconfig for snmp template": {
			createSim: func() *dyncfgSim {
				cfg := newTestSNMPConfig("snmp-test", testSNMPConfig{
					RescanInterval: confopt.LongDuration(1 * time.Hour),
					Credentials:    []testSNMPCredentialConfig{{Name: "public-v2", Version: "2c", Community: "public"}},
					Networks:       []testSNMPNetworkConfig{{Subnet: "192.168.1.0/24", Credential: "public-v2"}},
				}, defaultTestServices())
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						sendDyncfgCmd(sd, "1-userconfig",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeSNMP), "userconfig"},
							payload, "")
					},
					wantDyncfgFunc: func(t *testing.T, got string) {
						assert.Contains(t, got, "FUNCTION_RESULT_BEGIN 1-userconfig 200 application/yaml")
						assert.Contains(t, got, "name: snmp-test")
						assert.Contains(t, got, "discoverer:")
						assert.Contains(t, got, "snmp:")
						assert.Contains(t, got, "rescan_interval: 1h")
						assert.Contains(t, got, "subnet: 192.168.1.0/24")
					},
				}
			},
		},
		"userconfig fails on mismatched discoverer type": {
			createSim: func() *dyncfgSim {
				cfg := newTestDockerConfig("docker-test", "unix:///var/run/docker.sock", confopt.Duration(5*time.Second), defaultTestServices())
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						sendDyncfgCmd(sd, "1-userconfig",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeNetListeners), "userconfig"},
							payload, "")
					},
					wantDyncfgFunc: func(t *testing.T, got string) {
						assert.Contains(t, got, "FUNCTION_RESULT_BEGIN 1-userconfig 400 application/json")
						assert.Contains(t, got, "Failed to parse config")
						assert.Contains(t, got, "expected")
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
						// Manually add a file-based config to exposed cache
						cfg := sdConfig{
							"name":             "file-config",
							ikeyDiscovererType: testDiscovererTypeNetListeners,
							ikeyPipelineKey:    "/etc/netdata/sd/test.conf",
							ikeySource:         "/etc/netdata/sd/test.conf",
							ikeySourceType:     "file",
						}
						sd.exposed.Add(&dyncfg.Entry[sdConfig]{Cfg: cfg, Status: dyncfg.StatusRunning})

						// Try to remove
						sendDyncfgCmd(sd, "1-remove",
							[]string{sd.dyncfgJobID(testDiscovererTypeNetListeners, "file-config"), "remove"},
							nil, "")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: testDiscovererTypeNetListeners,
							name:           "file-config",
							sourceType:     "file",
							status:         dyncfg.StatusRunning,
						},
					},
					wantDyncfg: `
FUNCTION_RESULT_BEGIN 1-remove 405 application/json
{"status":405,"errorMessage":"removing jobs of type 'file' is not supported, only 'dyncfg' jobs can be removed."}
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
				cfg := newTestDockerConfig("docker-test", "unix:///var/run/docker.sock", confopt.Duration(5*time.Second), []pipeline.ServiceRuleConfig{
					{ID: "nginx", Match: `{{ glob .Image "*nginx*" }}`},
				})
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeDocker), "add", "docker-test"},
							payload, "type=dyncfg,user=test")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: testDiscovererTypeDocker,
							name:           "docker-test",
							sourceType:     "dyncfg",
							status:         dyncfg.StatusAccepted,
						},
					},
					wantDyncfg: `
FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:docker:docker-test create accepted job /collectors/test/ServiceDiscovery dyncfg 'type=dyncfg,user=test' 'schema get enable disable update test userconfig remove' 0x0000 0x0000
`,
				}
			},
		},
		"add and enable docker job": {
			createSim: func() *dyncfgSim {
				cfg := newTestDockerConfig("docker-test", "tcp://localhost:2375", 0, defaultTestServices())
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Add
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeDocker), "add", "docker-test"},
							payload, "type=dyncfg,user=test")

						// Enable
						sendDyncfgCmd(sd, "2-enable",
							[]string{sd.dyncfgJobID(testDiscovererTypeDocker, "docker-test"), "enable"},
							nil, "")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: testDiscovererTypeDocker,
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
				cfg := newTestDockerConfig("docker-test", "unix:///var/run/docker.sock", 0, defaultTestServices())
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Add
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeDocker), "add", "docker-test"},
							payload, "type=dyncfg,user=test")

						// Get
						sendDyncfgCmd(sd, "2-get",
							[]string{sd.dyncfgJobID(testDiscovererTypeDocker, "docker-test"), "get"},
							nil, "")
					},
					wantDyncfgFunc: func(t *testing.T, got string) {
						assert.Contains(t, got, "FUNCTION_RESULT_BEGIN 2-get 200 application/json")
						assert.Contains(t, got, `"name":"docker-test"`)
						assert.Contains(t, got, `"discoverer":{`)
						assert.Contains(t, got, `"docker":{`)
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
				k8sCfg := testK8sConfig{
					Role:       "pod",
					Namespaces: []string{"default", "kube-system"},
				}
				k8sCfg.Selector.Label = "app=nginx"
				k8sCfg.Pod.LocalMode = true
				cfg := newTestK8sConfig("k8s-test", []testK8sConfig{k8sCfg}, []pipeline.ServiceRuleConfig{
					{ID: "nginx-pods", Match: `{{ eq .Namespace "default" }}`},
				})
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeK8s), "add", "k8s-test"},
							payload, "type=dyncfg,user=test")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: testDiscovererTypeK8s,
							name:           "k8s-test",
							sourceType:     "dyncfg",
							status:         dyncfg.StatusAccepted,
						},
					},
					wantDyncfg: `
FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:k8s:k8s-test create accepted job /collectors/test/ServiceDiscovery dyncfg 'type=dyncfg,user=test' 'schema get enable disable update test userconfig remove' 0x0000 0x0000
`,
				}
			},
		},
		"add k8s service role job": {
			createSim: func() *dyncfgSim {
				cfg := newTestK8sConfig("k8s-svc-test", []testK8sConfig{{Role: "service"}}, defaultTestServices())
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Add
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeK8s), "add", "k8s-svc-test"},
							payload, "type=dyncfg,user=test")

						// Enable
						sendDyncfgCmd(sd, "2-enable",
							[]string{sd.dyncfgJobID(testDiscovererTypeK8s, "k8s-svc-test"), "enable"},
							nil, "")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: testDiscovererTypeK8s,
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
				cfg := newTestK8sConfig("k8s-test", []testK8sConfig{{Role: "pod", Namespaces: []string{"default"}}}, defaultTestServices())
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Add
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeK8s), "add", "k8s-test"},
							payload, "type=dyncfg,user=test")

						// Get
						sendDyncfgCmd(sd, "2-get",
							[]string{sd.dyncfgJobID(testDiscovererTypeK8s, "k8s-test"), "get"},
							nil, "")
					},
					wantDyncfgFunc: func(t *testing.T, got string) {
						assert.Contains(t, got, "FUNCTION_RESULT_BEGIN 2-get 200 application/json")
						assert.Contains(t, got, `"name":"k8s-test"`)
						assert.Contains(t, got, `"discoverer":{`)
						assert.Contains(t, got, `"k8s":[`)
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
				cfg := newTestSNMPConfig("snmp-test", testSNMPConfig{
					RescanInterval: confopt.LongDuration(30 * time.Minute),
					Timeout:        confopt.Duration(1 * time.Second),
					DeviceCacheTTL: confopt.LongDuration(12 * time.Hour),
					Credentials:    []testSNMPCredentialConfig{{Name: "public-v2", Version: "2c", Community: "public"}},
					Networks:       []testSNMPNetworkConfig{{Subnet: "192.168.1.0/24", Credential: "public-v2"}},
				}, defaultTestServices())
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeSNMP), "add", "snmp-test"},
							payload, "type=dyncfg,user=test")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: testDiscovererTypeSNMP,
							name:           "snmp-test",
							sourceType:     "dyncfg",
							status:         dyncfg.StatusAccepted,
						},
					},
					wantDyncfg: `
FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:snmp:snmp-test create accepted job /collectors/test/ServiceDiscovery dyncfg 'type=dyncfg,user=test' 'schema get enable disable update test userconfig remove' 0x0000 0x0000
`,
				}
			},
		},
		"add snmp v3 job": {
			createSim: func() *dyncfgSim {
				cfg := newTestSNMPConfig("snmp-v3-test", testSNMPConfig{
					Credentials: []testSNMPCredentialConfig{{
						Name:              "snmpv3-auth",
						Version:           "3",
						UserName:          "admin",
						SecurityLevel:     "authPriv",
						AuthProtocol:      "sha256",
						AuthPassphrase:    "authpass",
						PrivacyProtocol:   "aes",
						PrivacyPassphrase: "privpass",
					}},
					Networks: []testSNMPNetworkConfig{{Subnet: "10.0.0.0/24", Credential: "snmpv3-auth"}},
				}, defaultTestServices())
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Add
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeSNMP), "add", "snmp-v3-test"},
							payload, "type=dyncfg,user=test")

						// Enable
						sendDyncfgCmd(sd, "2-enable",
							[]string{sd.dyncfgJobID(testDiscovererTypeSNMP, "snmp-v3-test"), "enable"},
							nil, "")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: testDiscovererTypeSNMP,
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
				cfg := newTestSNMPConfig("snmp-test", testSNMPConfig{
					RescanInterval: confopt.LongDuration(1 * time.Hour),
					Credentials:    []testSNMPCredentialConfig{{Name: "v2-cred", Version: "2c", Community: "public"}},
					Networks:       []testSNMPNetworkConfig{{Subnet: "192.168.0.0/16", Credential: "v2-cred"}},
				}, defaultTestServices())
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Add
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeSNMP), "add", "snmp-test"},
							payload, "type=dyncfg,user=test")

						// Get
						sendDyncfgCmd(sd, "2-get",
							[]string{sd.dyncfgJobID(testDiscovererTypeSNMP, "snmp-test"), "get"},
							nil, "")
					},
					wantDyncfgFunc: func(t *testing.T, got string) {
						assert.Contains(t, got, "FUNCTION_RESULT_BEGIN 2-get 200 application/json")
						assert.Contains(t, got, `"name":"snmp-test"`)
						assert.Contains(t, got, `"discoverer":{`)
						assert.Contains(t, got, `"snmp":{`)
						assert.Contains(t, got, `"rescan_interval":"1h"`)
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
				cfg := newTestNetListenersConfig("test-job", confopt.LongDuration(5*time.Second), 0, defaultTestServices())
				payload, _ := json.Marshal(cfg)

				updatedCfg := newTestNetListenersConfig("test-job", confopt.LongDuration(10*time.Second), 0, defaultTestServices())
				updatedPayload, _ := json.Marshal(updatedCfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Add
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeNetListeners), "add", "test-job"},
							payload, "type=dyncfg,user=test")

						// Enable (starts pipeline)
						sendDyncfgCmd(sd, "2-enable",
							[]string{sd.dyncfgJobID(testDiscovererTypeNetListeners, "test-job"), "enable"},
							nil, "")

						// Update while running (should restart pipeline)
						sendDyncfgCmd(sd, "3-update",
							[]string{sd.dyncfgJobID(testDiscovererTypeNetListeners, "test-job"), "update"},
							updatedPayload, "type=dyncfg,user=test")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: testDiscovererTypeNetListeners,
							name:           "test-job",
							sourceType:     "dyncfg",
							status:         dyncfg.StatusRunning,
						},
					},
					wantRunning: []string{"dyncfg:net_listeners:test-job"},
					wantDyncfg: `
FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:test-job create accepted job /collectors/test/ServiceDiscovery dyncfg 'type=dyncfg,user=test' 'schema get enable disable update test userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 2-enable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:test-job status running

FUNCTION_RESULT_BEGIN 3-update 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:test-job status running
`,
				}
			},
		},
		"update running docker pipeline": {
			createSim: func() *dyncfgSim {
				cfg := newTestDockerConfig("docker-job", "unix:///var/run/docker.sock", 0, defaultTestServices())
				payload, _ := json.Marshal(cfg)

				updatedCfg := newTestDockerConfig("docker-job", "tcp://localhost:2375", 0, defaultTestServices())
				updatedPayload, _ := json.Marshal(updatedCfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Add
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeDocker), "add", "docker-job"},
							payload, "type=dyncfg,user=test")

						// Enable
						sendDyncfgCmd(sd, "2-enable",
							[]string{sd.dyncfgJobID(testDiscovererTypeDocker, "docker-job"), "enable"},
							nil, "")

						// Update while running
						sendDyncfgCmd(sd, "3-update",
							[]string{sd.dyncfgJobID(testDiscovererTypeDocker, "docker-job"), "update"},
							updatedPayload, "type=dyncfg,user=test")

						// Verify config was updated
						sendDyncfgCmd(sd, "4-get",
							[]string{sd.dyncfgJobID(testDiscovererTypeDocker, "docker-job"), "get"},
							nil, "")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: testDiscovererTypeDocker,
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
				cfg := newTestNetListenersConfig("test-job", confopt.LongDuration(5*time.Second), 0, defaultTestServices())
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						sendDyncfgCmd(sd, "1-test",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeNetListeners), "test"},
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
							[]string{sd.dyncfgTemplateID(testDiscovererTypeNetListeners), "test"},
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
				cfg := newTestDockerConfig("docker-test", "unix:///var/run/docker.sock", 0, defaultTestServices())
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						sendDyncfgCmd(sd, "1-test",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeDocker), "test"},
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
				cfg := newTestK8sConfig("k8s-test", []testK8sConfig{{Role: "pod"}}, defaultTestServices())
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						sendDyncfgCmd(sd, "1-test",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeK8s), "test"},
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
				cfg := newTestSNMPConfig("snmp-test", testSNMPConfig{
					Credentials: []testSNMPCredentialConfig{{Name: "v2-cred", Version: "2c"}},
					Networks:    []testSNMPNetworkConfig{{Subnet: "192.168.1.0/24", Credential: "v2-cred"}},
				}, defaultTestServices())
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						sendDyncfgCmd(sd, "1-test",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeSNMP), "test"},
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
		"test existing job config succeeds": {
			createSim: func() *dyncfgSim {
				cfg := newTestNetListenersConfig("test-job", confopt.LongDuration(5*time.Second), 0, defaultTestServices())
				payload, _ := json.Marshal(cfg)

				updatedCfg := newTestNetListenersConfig("test-job", confopt.LongDuration(10*time.Second), 0, defaultTestServices())
				updatedPayload, _ := json.Marshal(updatedCfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// First add the job
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeNetListeners), "add", "test-job"},
							payload, "type=dyncfg,user=test")

						// Test with new config (validates without applying)
						sendDyncfgCmd(sd, "2-test",
							[]string{sd.dyncfgJobID(testDiscovererTypeNetListeners, "test-job"), "test"},
							updatedPayload, "")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: testDiscovererTypeNetListeners,
							name:           "test-job",
							sourceType:     "dyncfg",
							status:         dyncfg.StatusAccepted, // Still accepted, not changed by test
						},
					},
					wantDyncfgFunc: func(t *testing.T, got string) {
						assert.Contains(t, got, "FUNCTION_RESULT_BEGIN 1-add 202 application/json")
						assert.Contains(t, got, "FUNCTION_RESULT_BEGIN 2-test 200 application/json")
					},
				}
			},
		},
		"test job with invalid config fails": {
			createSim: func() *dyncfgSim {
				cfg := newTestNetListenersConfig("test-job", 0, 0, defaultTestServices())
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// First add the job
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeNetListeners), "add", "test-job"},
							payload, "type=dyncfg,user=test")

						// Test with invalid JSON
						sendDyncfgCmd(sd, "2-test",
							[]string{sd.dyncfgJobID(testDiscovererTypeNetListeners, "test-job"), "test"},
							[]byte("{invalid json}"), "")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: testDiscovererTypeNetListeners,
							name:           "test-job",
							sourceType:     "dyncfg",
							status:         dyncfg.StatusAccepted,
						},
					},
					wantDyncfgFunc: func(t *testing.T, got string) {
						assert.Contains(t, got, "FUNCTION_RESULT_BEGIN 1-add 202 application/json")
						assert.Contains(t, got, "FUNCTION_RESULT_BEGIN 2-test 400 application/json")
						assert.Contains(t, got, "Failed to parse config")
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

func TestServiceDiscovery_DyncfgMultipleJobs(t *testing.T) {
	tests := map[string]struct {
		createSim func() *dyncfgSim
	}{
		"multiple jobs lifecycle": {
			createSim: func() *dyncfgSim {
				cfg1 := newTestNetListenersConfig("job1", 0, 0, defaultTestServices())
				cfg2 := newTestNetListenersConfig("job2", 0, 0, defaultTestServices())
				payload1, _ := json.Marshal(cfg1)
				payload2, _ := json.Marshal(cfg2)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Add job1
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeNetListeners), "add", "job1"},
							payload1, "type=dyncfg,user=test")

						// Add job2
						sendDyncfgCmd(sd, "2-add",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeNetListeners), "add", "job2"},
							payload2, "type=dyncfg,user=test")

						// Enable both
						sendDyncfgCmd(sd, "3-enable",
							[]string{sd.dyncfgJobID(testDiscovererTypeNetListeners, "job1"), "enable"},
							nil, "")
						sendDyncfgCmd(sd, "4-enable",
							[]string{sd.dyncfgJobID(testDiscovererTypeNetListeners, "job2"), "enable"},
							nil, "")

						// Disable job1
						sendDyncfgCmd(sd, "5-disable",
							[]string{sd.dyncfgJobID(testDiscovererTypeNetListeners, "job1"), "disable"},
							nil, "")

						// Remove job2
						sendDyncfgCmd(sd, "6-remove",
							[]string{sd.dyncfgJobID(testDiscovererTypeNetListeners, "job2"), "remove"},
							nil, "")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: testDiscovererTypeNetListeners,
							name:           "job1",
							sourceType:     "dyncfg",
							status:         dyncfg.StatusDisabled,
						},
					},
					wantRunning: []string{},
					wantDyncfg: `
FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:job1 create accepted job /collectors/test/ServiceDiscovery dyncfg 'type=dyncfg,user=test' 'schema get enable disable update test userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 2-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:job2 create accepted job /collectors/test/ServiceDiscovery dyncfg 'type=dyncfg,user=test' 'schema get enable disable update test userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 3-enable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:job1 status running

FUNCTION_RESULT_BEGIN 4-enable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:job2 status running

FUNCTION_RESULT_BEGIN 5-disable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:job1 status disabled

FUNCTION_RESULT_BEGIN 6-remove 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:job2 delete
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

func TestServiceDiscovery_DyncfgPriority(t *testing.T) {
	tests := map[string]struct {
		createSim func() *dyncfgSim
	}{
		"dyncfg add replaces running file config": {
			// File config is running, dyncfg add with same name should replace it
			createSim: func() *dyncfgSim {
				cfg := newTestNetListenersConfig("test-job", 0, 0, defaultTestServices())
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Simulate a running file config
						fileCfg := sdConfig{
							"name":             "test-job",
							ikeyDiscovererType: testDiscovererTypeNetListeners,
							ikeyPipelineKey:    "/etc/netdata/sd.d/test.conf",
							ikeySource:         "/etc/netdata/sd.d/test.conf",
							ikeySourceType:     confgroup.TypeUser,
						}
						sd.seen.Add(fileCfg)
						sd.exposed.Add(&dyncfg.Entry[sdConfig]{Cfg: fileCfg, Status: dyncfg.StatusRunning})

						// Start the pipeline to simulate running state
						pipelineCfg := pipeline.Config{Name: "test-job"}
						_ = sd.mgr.Start(sd.ctx, fileCfg.PipelineKey(), pipelineCfg)

						// Dyncfg add with same name - should replace file config
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeNetListeners), "add", "test-job"},
							payload, "type=dyncfg,user=test")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: testDiscovererTypeNetListeners,
							name:           "test-job",
							sourceType:     confgroup.TypeDyncfg, // dyncfg replaces file
							status:         dyncfg.StatusAccepted,
						},
					},
					wantRunning: []string{}, // old pipeline stopped
					wantDyncfgFunc: func(t *testing.T, got string) {
						// Should see: create new dyncfg config (no delete - CONFIG create updates existing)
						assert.Contains(t, got, "CONFIG test:sd:net_listeners:test-job create accepted job")
						assert.Contains(t, got, "FUNCTION_RESULT_BEGIN 1-add 202 application/json")
					},
				}
			},
		},
		"dyncfg add replaces stock file config": {
			// Stock file config exists, dyncfg add should replace it
			createSim: func() *dyncfgSim {
				cfg := newTestNetListenersConfig("test-job", 0, 0, defaultTestServices())
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Simulate a stock file config (not running)
						fileCfg := sdConfig{
							"name":             "test-job",
							ikeyDiscovererType: testDiscovererTypeNetListeners,
							ikeyPipelineKey:    "/usr/lib/netdata/conf.d/sd/test.conf",
							ikeySource:         "/usr/lib/netdata/conf.d/sd/test.conf",
							ikeySourceType:     confgroup.TypeStock,
						}
						sd.seen.Add(fileCfg)
						sd.exposed.Add(&dyncfg.Entry[sdConfig]{Cfg: fileCfg, Status: dyncfg.StatusAccepted})

						// Dyncfg add with same name - should replace stock config
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeNetListeners), "add", "test-job"},
							payload, "type=dyncfg,user=test")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: testDiscovererTypeNetListeners,
							name:           "test-job",
							sourceType:     confgroup.TypeDyncfg,
							status:         dyncfg.StatusAccepted,
						},
					},
					wantDyncfgFunc: func(t *testing.T, got string) {
						// Should see: create new dyncfg config (no delete - CONFIG create updates existing)
						assert.Contains(t, got, "CONFIG test:sd:net_listeners:test-job create accepted job")
						assert.Contains(t, got, "FUNCTION_RESULT_BEGIN 1-add 202 application/json")
						assert.Contains(t, got, "dyncfg")
					},
				}
			},
		},
		"dyncfg add replaces existing dyncfg config": {
			// Dyncfg config exists, another dyncfg add with same name should replace it (matching jobmgr pattern)
			createSim: func() *dyncfgSim {
				cfg := newTestNetListenersConfig("test-job", 0, 0, defaultTestServices())
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Simulate existing dyncfg config
						dyncfgCfg := sdConfig{
							"name":             "test-job",
							ikeyDiscovererType: testDiscovererTypeNetListeners,
							ikeyPipelineKey:    "dyncfg:net_listeners:test-job",
							ikeySource:         "type=dyncfg,user=admin",
							ikeySourceType:     confgroup.TypeDyncfg,
						}
						sd.seen.Add(dyncfgCfg)
						sd.exposed.Add(&dyncfg.Entry[sdConfig]{Cfg: dyncfgCfg, Status: dyncfg.StatusRunning})

						// Start the pipeline to simulate running state
						pipelineCfg := pipeline.Config{Name: "test-job"}
						_ = sd.mgr.Start(sd.ctx, dyncfgCfg.PipelineKey(), pipelineCfg)

						// Another dyncfg add with same name - should replace (matching jobmgr pattern)
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeNetListeners), "add", "test-job"},
							payload, "type=dyncfg,user=test")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: testDiscovererTypeNetListeners,
							name:           "test-job",
							sourceType:     confgroup.TypeDyncfg,
							status:         dyncfg.StatusAccepted, // new config in accepted state
						},
					},
					wantRunning: []string{}, // old pipeline stopped
					wantDyncfgFunc: func(t *testing.T, got string) {
						assert.Contains(t, got, "FUNCTION_RESULT_BEGIN 1-add 202 application/json")
						assert.Contains(t, got, "CONFIG test:sd:net_listeners:test-job create accepted job")
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

func TestServiceDiscovery_DyncfgUpdateSameConfig(t *testing.T) {
	tests := map[string]struct {
		createSim func() *dyncfgSim
	}{
		"update running pipeline with same config skips restart": {
			createSim: func() *dyncfgSim {
				cfg := newTestNetListenersConfig("test-job", confopt.LongDuration(5*time.Second), 0, defaultTestServices())
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Add
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeNetListeners), "add", "test-job"},
							payload, "type=dyncfg,user=test")

						// Enable
						sendDyncfgCmd(sd, "2-enable",
							[]string{sd.dyncfgJobID(testDiscovererTypeNetListeners, "test-job"), "enable"},
							nil, "")

						// Update with exact same config (should return 200 without restart)
						sendDyncfgCmd(sd, "3-update",
							[]string{sd.dyncfgJobID(testDiscovererTypeNetListeners, "test-job"), "update"},
							payload, "type=dyncfg,user=test")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: testDiscovererTypeNetListeners,
							name:           "test-job",
							sourceType:     "dyncfg",
							status:         dyncfg.StatusRunning,
						},
					},
					wantRunning: []string{"dyncfg:net_listeners:test-job"},
					wantDyncfg: `
FUNCTION_RESULT_BEGIN 1-add 202 application/json
{"status":202,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:test-job create accepted job /collectors/test/ServiceDiscovery dyncfg 'type=dyncfg,user=test' 'schema get enable disable update test userconfig remove' 0x0000 0x0000

FUNCTION_RESULT_BEGIN 2-enable 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:test-job status running

FUNCTION_RESULT_BEGIN 3-update 200 application/json
{"status":200,"message":""}
FUNCTION_RESULT_END

CONFIG test:sd:net_listeners:test-job status running
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

func TestServiceDiscovery_DyncfgUpdateFailedState(t *testing.T) {
	tests := map[string]struct {
		createSim func() *dyncfgSim
	}{
		"update config in failed state restarts pipeline": {
			createSim: func() *dyncfgSim {
				updatedCfg := newTestNetListenersConfig("test-job", confopt.LongDuration(10*time.Second), 0, defaultTestServices())
				updatedPayload, _ := json.Marshal(updatedCfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Manually add a failed config
						failedCfg := sdConfig{
							"name":             "test-job",
							ikeyDiscovererType: testDiscovererTypeNetListeners,
							ikeyPipelineKey:    "dyncfg:net_listeners:test-job",
							ikeySource:         "type=dyncfg,user=test",
							ikeySourceType:     confgroup.TypeDyncfg,
						}
						sd.seen.Add(failedCfg)
						sd.exposed.Add(&dyncfg.Entry[sdConfig]{Cfg: failedCfg, Status: dyncfg.StatusFailed})

						// Update should restart the pipeline
						sendDyncfgCmd(sd, "1-update",
							[]string{sd.dyncfgJobID(testDiscovererTypeNetListeners, "test-job"), "update"},
							updatedPayload, "type=dyncfg,user=test")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: testDiscovererTypeNetListeners,
							name:           "test-job",
							sourceType:     "dyncfg",
							status:         dyncfg.StatusRunning,
						},
					},
					wantRunning: []string{"dyncfg:net_listeners:test-job"},
					wantDyncfgFunc: func(t *testing.T, got string) {
						assert.Contains(t, got, "FUNCTION_RESULT_BEGIN 1-update 200 application/json")
						assert.Contains(t, got, "CONFIG test:sd:net_listeners:test-job status running")
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

func TestServiceDiscovery_DyncfgEnableFromFailed(t *testing.T) {
	tests := map[string]struct {
		createSim func() *dyncfgSim
	}{
		"enable config from failed state starts pipeline": {
			createSim: func() *dyncfgSim {
				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Manually add a failed config with valid pipeline config data
						failedCfg := sdConfig{
							"name":             "test-job",
							ikeyDiscovererType: testDiscovererTypeNetListeners,
							ikeyPipelineKey:    "dyncfg:net_listeners:test-job",
							ikeySource:         "type=dyncfg,user=test",
							ikeySourceType:     confgroup.TypeDyncfg,
							"discoverer": map[string]any{
								"net_listeners": map[string]any{},
							},
							"services": []any{
								map[string]any{"id": "test-rule", "match": "true"},
							},
						}
						sd.seen.Add(failedCfg)
						sd.exposed.Add(&dyncfg.Entry[sdConfig]{Cfg: failedCfg, Status: dyncfg.StatusFailed})

						// Enable should start the pipeline
						sendDyncfgCmd(sd, "1-enable",
							[]string{sd.dyncfgJobID(testDiscovererTypeNetListeners, "test-job"), "enable"},
							nil, "")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: testDiscovererTypeNetListeners,
							name:           "test-job",
							sourceType:     "dyncfg",
							status:         dyncfg.StatusRunning,
						},
					},
					wantRunning: []string{"dyncfg:net_listeners:test-job"},
					wantDyncfgFunc: func(t *testing.T, got string) {
						assert.Contains(t, got, "FUNCTION_RESULT_BEGIN 1-enable 200 application/json")
						assert.Contains(t, got, "CONFIG test:sd:net_listeners:test-job status running")
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

func TestServiceDiscovery_DyncfgConversionUpdate(t *testing.T) {
	tests := map[string]struct {
		createSim func() *dyncfgSim
	}{
		"update file config converts to dyncfg": {
			createSim: func() *dyncfgSim {
				updatedCfg := newTestNetListenersConfig("test-job", confopt.LongDuration(10*time.Second), 0, defaultTestServices())
				updatedPayload, _ := json.Marshal(updatedCfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Simulate running file config
						fileCfg := sdConfig{
							"name":             "test-job",
							ikeyDiscovererType: testDiscovererTypeNetListeners,
							ikeyPipelineKey:    "/etc/netdata/sd.d/test.conf",
							ikeySource:         "/etc/netdata/sd.d/test.conf",
							ikeySourceType:     confgroup.TypeUser,
							"discoverer": map[string]any{
								"net_listeners": map[string]any{},
							},
							"services": []any{
								map[string]any{"id": "test-rule", "match": "true"},
							},
						}
						sd.seen.Add(fileCfg)
						sd.exposed.Add(&dyncfg.Entry[sdConfig]{Cfg: fileCfg, Status: dyncfg.StatusRunning})

						// Start the file pipeline
						pipelineCfg := pipeline.Config{Name: "test-job"}
						_ = sd.mgr.Start(sd.ctx, fileCfg.PipelineKey(), pipelineCfg)

						// Update via dyncfg - should convert to dyncfg source
						sendDyncfgCmd(sd, "1-update",
							[]string{sd.dyncfgJobID(testDiscovererTypeNetListeners, "test-job"), "update"},
							updatedPayload, "type=dyncfg,user=admin")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: testDiscovererTypeNetListeners,
							name:           "test-job",
							sourceType:     confgroup.TypeDyncfg, // Converted to dyncfg!
							status:         dyncfg.StatusRunning,
						},
					},
					wantRunning: []string{"dyncfg:net_listeners:test-job"}, // New pipeline key
					wantDyncfgFunc: func(t *testing.T, got string) {
						// ConfigCreate acts as upsert (no delete needed)
						assert.Contains(t, got, "CONFIG test:sd:net_listeners:test-job create running job")
						assert.Contains(t, got, "dyncfg") // New source type
						assert.Contains(t, got, "FUNCTION_RESULT_BEGIN 1-update 200 application/json")
					},
				}
			},
		},
		"update disabled file config converts to dyncfg without starting": {
			createSim: func() *dyncfgSim {
				updatedCfg := newTestNetListenersConfig("test-job", confopt.LongDuration(10*time.Second), 0, defaultTestServices())
				updatedPayload, _ := json.Marshal(updatedCfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Simulate disabled file config
						fileCfg := sdConfig{
							"name":             "test-job",
							ikeyDiscovererType: testDiscovererTypeNetListeners,
							ikeyPipelineKey:    "/etc/netdata/sd.d/test.conf",
							ikeySource:         "/etc/netdata/sd.d/test.conf",
							ikeySourceType:     confgroup.TypeUser,
							"discoverer": map[string]any{
								"net_listeners": map[string]any{},
							},
							"services": []any{
								map[string]any{"id": "test-rule", "match": "true"},
							},
						}
						sd.seen.Add(fileCfg)
						sd.exposed.Add(&dyncfg.Entry[sdConfig]{Cfg: fileCfg, Status: dyncfg.StatusDisabled})

						// Update via dyncfg - should convert but stay disabled
						sendDyncfgCmd(sd, "1-update",
							[]string{sd.dyncfgJobID(testDiscovererTypeNetListeners, "test-job"), "update"},
							updatedPayload, "type=dyncfg,user=admin")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: testDiscovererTypeNetListeners,
							name:           "test-job",
							sourceType:     confgroup.TypeDyncfg,
							status:         dyncfg.StatusDisabled, // Stays disabled
						},
					},
					wantRunning: []string{}, // Not running
					wantDyncfgFunc: func(t *testing.T, got string) {
						// ConfigCreate acts as upsert (no delete needed)
						assert.Contains(t, got, "CONFIG test:sd:net_listeners:test-job create disabled job")
						assert.Contains(t, got, "FUNCTION_RESULT_BEGIN 1-update 200 application/json")
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

func TestServiceDiscovery_DyncfgRestartErrorHandling(t *testing.T) {
	tests := map[string]struct {
		createSim func() *dyncfgSim
	}{
		"restart with invalid config keeps old pipeline running": {
			createSim: func() *dyncfgSim {
				cfg := newTestNetListenersConfig("test-job", 0, 0, defaultTestServices())
				payload, _ := json.Marshal(cfg)

				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Override newPipeline to fail on second call
						callCount := 0
						sd.newPipeline = func(cfg pipeline.Config) (sdPipeline, error) {
							callCount++
							if callCount > 1 {
								return nil, errors.New("simulated pipeline creation failure")
							}
							return newTestPipeline(cfg.Name), nil
						}
						// Also update mgr's newPipeline
						sd.mgr.newPipeline = sd.newPipeline

						// Add and enable
						sendDyncfgCmd(sd, "1-add",
							[]string{sd.dyncfgTemplateID(testDiscovererTypeNetListeners), "add", "test-job"},
							payload, "type=dyncfg,user=test")

						sendDyncfgCmd(sd, "2-enable",
							[]string{sd.dyncfgJobID(testDiscovererTypeNetListeners, "test-job"), "enable"},
							nil, "")

						// Update - pipeline creation will fail
						updatedCfg := newTestNetListenersConfig("test-job", confopt.LongDuration(10*time.Second), 0, defaultTestServices())
						updatedPayload, _ := json.Marshal(updatedCfg)

						sendDyncfgCmd(sd, "3-update",
							[]string{sd.dyncfgJobID(testDiscovererTypeNetListeners, "test-job"), "update"},
							updatedPayload, "type=dyncfg,user=test")
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: testDiscovererTypeNetListeners,
							name:           "test-job",
							sourceType:     "dyncfg",
							status:         dyncfg.StatusFailed,
						},
					},
					// NOTE: When Restart fails validation (newPipeline fails), the old pipeline
					// keeps running. This is the intended Restart behavior - validate before stopping.
					// The status shows Failed but old pipeline continues collecting data.
					wantRunning: []string{"dyncfg:net_listeners:test-job"},
					wantDyncfgFunc: func(t *testing.T, got string) {
						assert.Contains(t, got, "FUNCTION_RESULT_BEGIN 3-update 200 application/json")
						assert.Contains(t, got, "CONFIG test:sd:net_listeners:test-job status failed")
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

func TestServiceDiscovery_DyncfgFileRemovalWithDyncfgOverride(t *testing.T) {
	tests := map[string]struct {
		createSim func() *dyncfgSim
	}{
		"file config removal does not affect dyncfg override": {
			createSim: func() *dyncfgSim {
				return &dyncfgSim{
					do: func(sd *ServiceDiscovery) {
						// Add file config to seen cache (simulating it was seen from file)
						fileCfg := sdConfig{
							"name":             "test-job",
							ikeyDiscovererType: testDiscovererTypeNetListeners,
							ikeyPipelineKey:    "/etc/netdata/sd.d/test.conf",
							ikeySource:         "/etc/netdata/sd.d/test.conf",
							ikeySourceType:     confgroup.TypeUser,
						}
						sd.seen.Add(fileCfg)

						// Add dyncfg override (higher priority) to both caches
						dyncfgCfg := sdConfig{
							"name":             "test-job",
							ikeyDiscovererType: testDiscovererTypeNetListeners,
							ikeyPipelineKey:    "dyncfg:net_listeners:test-job",
							ikeySource:         "type=dyncfg,user=test",
							ikeySourceType:     confgroup.TypeDyncfg,
						}
						sd.seen.Add(dyncfgCfg)
						sd.exposed.Add(&dyncfg.Entry[sdConfig]{Cfg: dyncfgCfg, Status: dyncfg.StatusRunning})

						// Start the dyncfg pipeline
						pipelineCfg := pipeline.Config{Name: "test-job"}
						_ = sd.mgr.Start(sd.ctx, dyncfgCfg.PipelineKey(), pipelineCfg)

						// Simulate file removal by calling removePipeline
						sd.removePipeline(confFile{source: "/etc/netdata/sd.d/test.conf"})
					},
					wantExposed: []wantExposedConfig{
						{
							discovererType: testDiscovererTypeNetListeners,
							name:           "test-job",
							sourceType:     confgroup.TypeDyncfg, // Dyncfg still exposed
							status:         dyncfg.StatusRunning,
						},
					},
					wantRunning: []string{"dyncfg:net_listeners:test-job"}, // Dyncfg pipeline still running
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
