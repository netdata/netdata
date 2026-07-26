// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

func TestRunGenerationStartsWithIndividuallyValidatedFileSecretConfigs(t *testing.T) {
	root := filepath.Join(t.TempDir(), "etc", "netdata", "go.d")
	path := filepath.Join(root, "ss", "custom.conf")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(`
jobs:
  - name: valid
    kind: vault
    value: initial
  - name: inferred_unknown
    value: ignored
  - name: non_string_kind
    kind: 123
    value: ignored
`), 0o644))

	initial, loadErrs := secretstore.LoadFileConfigs([]string{root})
	started := make(chan struct{}, 1)
	modules := collectorapi.Registry{
		"module": {
			Create: func() collectorapi.CollectorV1 {
				return &collectorapi.MockCollectorV1{
					InitFunc: func(context.Context) error {
						started <- struct{}{}
						return nil
					},
					ChartsFunc: func() *collectorapi.Charts {
						return &collectorapi.Charts{
							&collectorapi.Chart{
								ID:    "chart",
								Title: "chart",
								Units: "value",
								Dims: collectorapi.Dims{&collectorapi.Dim{
									ID: "value",
								}},
							},
						}
					},
					CollectFunc: func(context.Context) map[string]int64 {
						return map[string]int64{"value": 1}
					},
				}
			},
			Config: func() any {
				return &collectorapi.MockConfiguration{}
			},
			JobConfigSchema: collectorapi.MockConfigSchema,
		},
	}
	jobConfig := confgroup.Config{
		"module":        "module",
		"name":          "unrelated",
		"update_every":  1,
		"function_only": false,
		"option_str":    "work",
		"option_int":    1,
	}
	jobConfig.SetProvider(confgroup.TypeUser)
	jobConfig.SetSourceType(confgroup.TypeUser)
	jobConfig.SetSource("file=test")
	creators, err := secretstore.NewCreatorCatalog(
		[]secretstore.Creator{{
			Kind:   secretstore.KindVault,
			Schema: `{}`,
			Create: func() secretstore.Store {
				return &processSecretStore{}
			},
		}},
	)
	require.NoError(t, err)
	jobs := testRunJobServices(t)
	jobs.Defaults = confgroup.Registry{
		"module": {UpdateEvery: 1},
	}
	jobs.StoreCreators = creators
	output := newProcessSynchronizedBuffer()
	frames, err := lifecycle.NewFrameOwner(output)
	require.NoError(t, err)
	uids := lifecycle.NewUIDLedger()
	generation, err := newRunGeneration(runGenerationConfig{
		Generation:      1,
		ShutdownTimeout: time.Second,
		UIDs:            uids,
		Frames:          frames,
		Modules:         modules,
		Jobs:            jobs,
		Secrets: runSecretServices{
			Initial: initial,
		},
		Discovery: testRunDiscoveryServices(t, jobConfig),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		generation.Stop()
		require.NoError(t, generation.Wait(context.Background()))
		closeRunTestUIDs(t, uids)
	})

	require.NoError(t, generation.start(context.Background()))
	select {
	case <-started:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "unrelated collector did not start")
	}
	require.Eventually(t, func() bool {
		record, ok := generation.vnodes.graph.Lookup(jobConfig.FullName())
		return ok && record.Status == dyncfg.StatusRunning.String()
	}, time.Second, time.Millisecond)

	require.Len(t, initial, 1)
	require.Len(t, loadErrs, 2)
	require.Contains(t, output.String(), "CONFIG go.d:secretstore:vault:valid create running job")
	require.Contains(t, output.String(), "CONFIG go.d:collector:module:unrelated create running job")
	require.NotContains(t, output.String(), "inferred_unknown")
	require.NotContains(t, output.String(), "non_string_kind")
	require.NoError(t, generation.run.DirtyCause())
}

func TestProcessCoreSecretUpdateDependentRestart(t *testing.T) {
	tests := map[string]struct {
		restartErr error
	}{
		"starts replacement generation": {},
		"reports replacement construction failure": {
			restartErr: errors.New("collector initialization exposed backend-sensitive-detail"),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			testProcessCoreSecretMutationDependentRestart(t, "update", test.restartErr)
		})
	}
}

func TestProcessCoreSecretAddReplayDependentRestart(t *testing.T) {
	testProcessCoreSecretMutationDependentRestart(t, "add", nil)
}

func testProcessCoreSecretMutationDependentRestart(
	t *testing.T,
	command string,
	restartErr error,
) {
	t.Helper()
	starts := make(chan string, 4)
	var cleanups atomic.Int32
	modules := collectorapi.Registry{
		"module": {
			Create: func() collectorapi.CollectorV1 {
				collector := &collectorapi.MockCollectorV1{
					CleanupFunc: func(context.Context) {
						cleanups.Add(1)
					},
				}
				collector.InitFunc = func(ctx context.Context) error {
					starts <- collector.Config.OptionStr
					if collector.Config.OptionStr == "replacement" {
						if restartErr != nil {
							return restartErr
						}
					}
					return nil
				}
				return collector
			},
			Config: func() any {
				return &collectorapi.MockConfiguration{}
			},
			AgentFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: "method"}}
			},
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return &runTestHandler{
					cleanup: func() {},
				}
			},
			JobConfigSchema: collectorapi.MockConfigSchema,
		},
	}
	jobConfig := confgroup.Config{
		"module":        "module",
		"name":          "job",
		"update_every":  1,
		"function_only": true,
		"option_str":    "${store:vault:main:key}",
		"option_int":    1,
	}
	jobConfig.SetProvider(confgroup.TypeDyncfg)
	jobConfig.SetSourceType(confgroup.TypeDyncfg)
	jobConfig.SetSource("test")
	jobs := testRunJobServices(t)
	jobs.Defaults = confgroup.Registry{
		"module": {UpdateEvery: 1},
	}
	creators, err := secretstore.NewCreatorCatalog(
		[]secretstore.Creator{{
			Kind:   secretstore.KindVault,
			Schema: `{}`,
			Create: func() secretstore.Store {
				return &processSecretStore{}
			},
		}},
	)
	require.NoError(t, err)
	jobs.StoreCreators = creators
	initialStore := secretstore.Config{
		"name":            "main",
		"kind":            string(secretstore.KindVault),
		"value":           "initial",
		"__source__":      confgroup.TypeUser,
		"__source_type__": confgroup.TypeUser,
	}

	reader, writer := io.Pipe()
	defer func() { require.NoError(t, writer.Close()) }()
	output := newProcessSynchronizedBuffer()
	process, err := newProcessCore(processCoreConfig{
		Input:           reader,
		Output:          output,
		ShutdownTimeout: time.Second,
		Modules:         modules,
		Jobs:            jobs,
		Secrets: runSecretServices{
			Initial: []secretstore.Config{initialStore},
		},
		Discovery:   testRunDiscoveryServices(t, jobConfig),
		Diagnostics: testProcessDiagnostics(),
	})
	require.NoError(t, err)
	commands := make(chan processControl, 1)
	done := make(chan error, 1)
	go func() {
		done <- process.run(context.Background(), commands)
	}()
	select {
	case got := <-starts:
		require.EqualValues(t, "initial", got)
	case err := <-done:
		require.FailNowf(
			t,
			"test failed",
			"process stopped before initial collector start: %v; output=%q",
			err,
			output.String(),
		)
	case <-time.After(3 * time.Second):
		require.FailNowf(t, "test failed", "collector did not start with initial secret; output=%q", output.String())
	}

	uid := "secret-" + command
	target := "go.d:secretstore:vault:main update"
	if command == "add" {
		target = "go.d:secretstore:vault add main"
	}
	_, writeStringErr := io.WriteString(
		writer,
		"FUNCTION_PAYLOAD "+uid+" 30 "+
			"\"config "+target+"\" "+
			"0xFFFF \"user=test\" application/json\n"+
			"{\"value\":\"replacement\"}\n"+
			"FUNCTION_PAYLOAD_END\n",
	)
	require.NoError(t, writeStringErr)

	waitSecretStart(t, starts, "replacement")
	output.waitContains(t, "FUNCTION_RESULT_BEGIN "+uid+" 200 application/json")
	if restartErr == nil {
		require.EqualValues(t, 1, cleanups.Load())
	} else {
		output.waitContains(t, "CONFIG go.d:collector:module:job status failed")
		wire := output.String()
		require.Contains(t, wire, "dependent collector restarts failed for jobs: module:job")
		require.NotContains(t, wire, "backend-sensitive-detail")
	}
	select {
	case err := <-done:
		require.FailNowf(t, "test failed", "process stopped after operational restart failure: %v", err)
	default:
	}

	commands <- testProcessControl(processTerminate)
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		require.FailNow(t, "test failed", "process did not terminate")
	}
	require.EqualValues(t, 2, cleanups.Load())
}

func TestProcessCoreCancelledSecretUpdateCompletesStartedReplacement(t *testing.T) {
	starts := make(chan string, 4)
	stopEntered := make(chan struct{})
	releaseStop := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(releaseStop) })
	})
	var cleanups atomic.Int32
	var armStop atomic.Bool
	modules := collectorapi.Registry{
		"module": {
			Create: func() collectorapi.CollectorV1 {
				collector := &collectorapi.MockCollectorV1{}
				collector.InitFunc = func(context.Context) error {
					starts <- collector.Config.OptionStr
					return nil
				}
				collector.CleanupFunc = func(context.Context) {
					if armStop.Load() && cleanups.Add(1) == 1 {
						close(stopEntered)
						<-releaseStop
					}
				}
				return collector
			},
			Config: func() any {
				return &collectorapi.MockConfiguration{}
			},
			AgentFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: "method"}}
			},
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return &runTestHandler{
					cleanup: func() {},
				}
			},
			JobConfigSchema: collectorapi.MockConfigSchema,
		},
	}
	jobConfig := confgroup.Config{
		"module":        "module",
		"name":          "job",
		"update_every":  1,
		"function_only": true,
		"option_str":    "${store:vault:main:key}",
		"option_int":    1,
	}
	jobConfig.SetProvider(confgroup.TypeDyncfg)
	jobConfig.SetSourceType(confgroup.TypeDyncfg)
	jobConfig.SetSource("test")
	jobs := testRunJobServices(t)
	jobs.Defaults = confgroup.Registry{
		"module": {UpdateEvery: 1},
	}
	creators, err := secretstore.NewCreatorCatalog(
		[]secretstore.Creator{{
			Kind:   secretstore.KindVault,
			Schema: `{}`,
			Create: func() secretstore.Store {
				return &processSecretStore{}
			},
		}},
	)
	require.NoError(t, err)
	jobs.StoreCreators = creators
	initialStore := secretstore.Config{
		"name":            "main",
		"kind":            string(secretstore.KindVault),
		"value":           "initial",
		"__source__":      confgroup.TypeUser,
		"__source_type__": confgroup.TypeUser,
	}

	reader, writer := io.Pipe()
	defer func() { require.NoError(t, writer.Close()) }()
	output := newProcessSynchronizedBuffer()
	process, err := newProcessCore(processCoreConfig{
		Input:           reader,
		Output:          output,
		ShutdownTimeout: time.Second,
		Modules:         modules,
		Jobs:            jobs,
		Secrets: runSecretServices{
			Initial: []secretstore.Config{initialStore},
		},
		Discovery:   testRunDiscoveryServices(t, jobConfig),
		Diagnostics: testProcessDiagnostics(),
	})
	require.NoError(t, err)
	commands := make(chan processControl, 1)
	done := make(chan error, 1)
	go func() {
		done <- process.run(context.Background(), commands)
	}()
	select {
	case got := <-starts:
		require.EqualValues(t, "initial", got)
	case err := <-done:
		require.FailNowf(
			t,
			"test failed",
			"process stopped before initial collector start: %v; output=%q",
			err,
			output.String(),
		)
	case <-time.After(3 * time.Second):
		require.FailNowf(t, "test failed", "collector did not start with initial secret; output=%q", output.String())
	}
	armStop.Store(true)

	_, writeStringErr := io.WriteString(
		writer,
		"FUNCTION_PAYLOAD secret-cancel 30 "+
			"\"config go.d:secretstore:vault:main update\" "+
			"0xFFFF \"user=test\" application/json\n"+
			"{\"value\":\"replacement\"}\n"+
			"FUNCTION_PAYLOAD_END\n",
	)
	require.NoError(t, writeStringErr)

	select {
	case <-stopEntered:
	case <-time.After(3 * time.Second):
		require.FailNow(t, "test failed", "dependent stop did not reach collector cleanup")
	}

	_, writeStringErr2 := io.WriteString(writer, "FUNCTION_CANCEL secret-cancel\n")
	require.NoError(t, writeStringErr2)

	releaseOnce.Do(func() { close(releaseStop) })
	waitSecretStart(t, starts, "replacement")
	output.waitContains(t, "FUNCTION_RESULT_BEGIN secret-cancel 499 application/json")

	require.EqualValues(t, 1, cleanups.Load())

	commands <- testProcessControl(processTerminate)
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		require.FailNow(t, "test failed", "process did not terminate")
	}
	require.EqualValues(t, 2, cleanups.Load())
}

func TestProcessCoreSecretCRUDAndValidationRedaction(t *testing.T) {
	jobs := testRunJobServices(t)
	var err error
	creators, err := secretstore.NewCreatorCatalog(
		[]secretstore.Creator{{
			Kind:   secretstore.KindVault,
			Schema: `{}`,
			Create: func() secretstore.Store {
				return &processSecretStore{}
			},
		}},
	)
	require.NoError(t, err)
	jobs.StoreCreators = creators
	reader, writer := io.Pipe()
	defer func() { require.NoError(t, writer.Close()) }()
	output := newProcessSynchronizedBuffer()
	process, err := newProcessCore(processCoreConfig{
		Input:           reader,
		Output:          output,
		ShutdownTimeout: time.Second,
		Modules:         collectorapi.Registry{},
		Jobs:            jobs,
		Discovery:       testRunDiscoveryServices(t),
		Diagnostics:     testProcessDiagnostics(),
	})
	require.NoError(t, err)
	commands := make(chan processControl, 1)
	done := make(chan error, 1)
	go func() {
		done <- process.run(context.Background(), commands)
	}()
	output.waitContains(t, "CONFIG go.d:secretstore:vault create accepted template")

	steps := []struct {
		uid     string
		command string
		payload string
		status  int
	}{
		{
			uid:     "secret-add",
			command: "config go.d:secretstore:vault add main",
			payload: `{"value":"initial"}`, status: 200,
		},
		{uid: "secret-get", command: "config go.d:secretstore:vault:main get", status: 200},
		{uid: "secret-test", command: "config go.d:secretstore:vault:main test", status: 202},
		{
			uid:     "secret-update",
			command: "config go.d:secretstore:vault:main update",
			payload: `{"value":"replacement"}`, status: 200,
		},
		{uid: "secret-remove", command: "config go.d:secretstore:vault:main remove", status: 200},
		{uid: "secret-get-removed", command: "config go.d:secretstore:vault:main get", status: 404},
		{
			uid:     "secret-invalid",
			command: "config go.d:secretstore:vault add invalid",
			payload: `{"value":"backend-sensitive-detail"}`, status: 400,
		},
	}
	for _, step := range steps {
		if step.payload == "" {

			_, writeStringErr := io.WriteString(
				writer,
				"FUNCTION "+step.uid+" 30 \""+step.command+"\" 0xFFFF \"user=test\"\n",
			)
			require.NoError(t, writeStringErr)

		} else {
			_, err := io.WriteString(
				writer,
				"FUNCTION_PAYLOAD "+step.uid+" 30 \""+
					step.command+
					"\" 0xFFFF \"user=test\" application/json\n"+
					step.payload+"\nFUNCTION_PAYLOAD_END\n",
			)
			require.NoError(t, err)
		}
		output.waitContains(t, "FUNCTION_RESULT_BEGIN "+step.uid+" "+strconv.Itoa(step.status)+" application/json")
	}
	require.NotContains(t, output.String(), "backend-sensitive-detail")

	commands <- testProcessControl(processTerminate)
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		require.FailNow(t, "test failed", "process did not terminate")
	}
}

func TestProcessCoreSecretUpdateYieldsJobGraphDuringRestartProbe(t *testing.T) {
	restartEntered := make(chan struct{})
	releaseRestart := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(releaseRestart) })
	})
	var starts atomic.Int32
	modules := collectorapi.Registry{
		"module": {
			Create: func() collectorapi.CollectorV1 {
				collector := &collectorapi.MockCollectorV1{}
				collector.InitFunc = func(context.Context) error {
					if starts.Add(1) == 2 {
						close(restartEntered)
						<-releaseRestart
					}
					return nil
				}
				return collector
			},
			Config: func() any {
				return &collectorapi.MockConfiguration{}
			},
			AgentFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: "method"}}
			},
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return &runTestHandler{
					cleanup: func() {},
				}
			},
			JobConfigSchema: collectorapi.MockConfigSchema,
		},
	}
	jobConfig := confgroup.Config{
		"module":        "module",
		"name":          "job",
		"update_every":  1,
		"function_only": true,
		"option_str":    "${store:vault:main:key}",
		"option_int":    1,
	}
	jobConfig.SetProvider(confgroup.TypeDyncfg)
	jobConfig.SetSourceType(confgroup.TypeDyncfg)
	jobConfig.SetSource("test")
	jobs := testRunJobServices(t)
	jobs.Defaults = confgroup.Registry{
		"module": {UpdateEvery: 1},
	}
	creators, err := secretstore.NewCreatorCatalog(
		[]secretstore.Creator{{
			Kind:   secretstore.KindVault,
			Schema: `{}`,
			Create: func() secretstore.Store {
				return &processSecretStore{}
			},
		}},
	)
	require.NoError(t, err)
	jobs.StoreCreators = creators
	initialStore := secretstore.Config{
		"name":            "main",
		"kind":            string(secretstore.KindVault),
		"value":           "initial",
		"__source__":      confgroup.TypeUser,
		"__source_type__": confgroup.TypeUser,
	}
	reader, writer := io.Pipe()
	defer func() { require.NoError(t, writer.Close()) }()
	output := newProcessSynchronizedBuffer()
	process, err := newProcessCore(processCoreConfig{
		Input:           reader,
		Output:          output,
		ShutdownTimeout: time.Second,
		Modules:         modules,
		Jobs:            jobs,
		Secrets: runSecretServices{
			Initial: []secretstore.Config{initialStore},
		},
		Discovery:   testRunDiscoveryServices(t, jobConfig),
		Diagnostics: testProcessDiagnostics(),
	})
	require.NoError(t, err)
	commands := make(chan processControl, 1)
	done := make(chan error, 1)
	go func() {
		done <- process.run(context.Background(), commands)
	}()
	output.waitContains(t, "CONFIG go.d:collector:module:job create running job")

	_, writeStringErr := io.WriteString(
		writer,
		"FUNCTION_PAYLOAD secret-rotation 30 "+
			"\"config go.d:secretstore:vault:main update\" "+
			"0xFFFF \"user=test\" application/json\n"+
			"{\"value\":\"replacement\"}\n"+
			"FUNCTION_PAYLOAD_END\n",
	)
	require.NoError(t, writeStringErr)

	select {
	case <-restartEntered:
	case <-time.After(3 * time.Second):
		require.FailNow(t, "test failed", "dependent restart did not reach the blocking start")
	}

	_, writeStringErr2 := io.WriteString(
		writer,
		"FUNCTION_PAYLOAD job-add 30 "+
			"\"config go.d:collector:module add other\" "+
			"0xFFFF \"user=test\" application/json\n"+
			"{\"option_str\":\"plain\",\"option_int\":1}\n"+
			"FUNCTION_PAYLOAD_END\n",
	)
	require.NoError(t, writeStringErr2)

	output.waitContains(t, "FUNCTION_RESULT_BEGIN job-add 202 application/json")
	output.waitContains(t, "CONFIG go.d:collector:module:other create accepted job")

	releaseOnce.Do(func() { close(releaseRestart) })
	output.waitContains(t, "FUNCTION_RESULT_BEGIN secret-rotation 200 application/json")

	commands <- testProcessControl(processTerminate)
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		require.FailNow(t, "test failed", "process did not terminate")
	}
}

func waitSecretStart(t *testing.T, starts <-chan string, want string) {
	t.Helper()
	select {
	case got := <-starts:
		require.EqualValues(t, want, got)
	case <-time.After(3 * time.Second):
		require.FailNowf(t, "test failed", "collector did not start with secret %q", want)
	}
}

type processSecretStore struct {
	config struct {
		Value string `yaml:"value"`
	}
}

func (pss *processSecretStore) Configuration() any {
	return &pss.config
}

func (pss *processSecretStore) Init(context.Context) error {
	if pss.config.Value == "backend-sensitive-detail" {
		return errors.New("backend rejected backend-sensitive-detail")
	}
	return nil
}

func (pss *processSecretStore) Publish() secretstore.PublishedStore {
	return processPublishedSecret(pss.config.Value)
}

type processPublishedSecret string

func (pps processPublishedSecret) Resolve(context.Context, secretstore.ResolveRequest) (string, error) {
	return string(pps), nil
}
