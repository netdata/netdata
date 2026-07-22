// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"
	"io"
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
	"github.com/stretchr/testify/require"
)

func TestProcessCoreSecretUpdateRestartsDependentAgainstNewGeneration(t *testing.T) {
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
				collector.InitFunc = func(context.Context) error {
					starts <- collector.Config.OptionStr
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
				return &runTestHandler{cleanup: func() {}}
			},
			JobConfigSchema: collectorapi.MockConfigSchema,
		},
	}
	jobConfig := confgroup.Config{
		"module": "module", "name": "job",
		"update_every": 1, "function_only": true,
		"option_str": "${store:vault:main:key}",
		"option_int": 1,
	}
	jobConfig.SetProvider(confgroup.TypeDyncfg)
	jobConfig.SetSourceType(confgroup.TypeDyncfg)
	jobConfig.SetSource("test")
	jobs := testRunJobServices(t)
	jobs.Defaults = confgroup.Registry{"module": {UpdateEvery: 1}}
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
		"name": "main", "kind": string(secretstore.KindVault),
		"value":           "initial",
		"__source__":      confgroup.TypeUser,
		"__source_type__": confgroup.TypeUser,
	}

	reader, writer := io.Pipe()
	defer func() { require.NoError(t, writer.Close()) }()
	output := newProcessSynchronizedBuffer()
	process, err := newProcessCore(processCoreConfig{
		Input: reader, Output: output,
		ShutdownTimeout: time.Second,
		Modules:         modules, Jobs: jobs,
		Secrets:   runSecretServices{Initial: []secretstore.Config{initialStore}},
		Discovery: testRunDiscoveryServices(t, jobConfig),
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

	_, writeStringErr := io.WriteString(
		writer,
		"FUNCTION_PAYLOAD secret-update 30 "+
			"\"config go.d:secretstore:vault:main update\" "+
			"0xFFFF \"user=test\" application/json\n"+
			"{\"value\":\"replacement\"}\n"+
			"FUNCTION_PAYLOAD_END\n",
	)
	require.NoError(t, writeStringErr)

	waitSecretStart(t, starts, "replacement")
	require.EqualValues(t, 1, cleanups.Load())
	output.waitContains(t, "FUNCTION_RESULT_BEGIN secret-update 200 application/json")

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
				return &runTestHandler{cleanup: func() {}}
			},
			JobConfigSchema: collectorapi.MockConfigSchema,
		},
	}
	jobConfig := confgroup.Config{
		"module": "module", "name": "job",
		"update_every": 1, "function_only": true,
		"option_str": "${store:vault:main:key}",
		"option_int": 1,
	}
	jobConfig.SetProvider(confgroup.TypeDyncfg)
	jobConfig.SetSourceType(confgroup.TypeDyncfg)
	jobConfig.SetSource("test")
	jobs := testRunJobServices(t)
	jobs.Defaults = confgroup.Registry{"module": {UpdateEvery: 1}}
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
		"name": "main", "kind": string(secretstore.KindVault),
		"value":           "initial",
		"__source__":      confgroup.TypeUser,
		"__source_type__": confgroup.TypeUser,
	}

	reader, writer := io.Pipe()
	defer func() { require.NoError(t, writer.Close()) }()
	output := newProcessSynchronizedBuffer()
	process, err := newProcessCore(processCoreConfig{
		Input: reader, Output: output,
		ShutdownTimeout: time.Second,
		Modules:         modules, Jobs: jobs,
		Secrets:   runSecretServices{Initial: []secretstore.Config{initialStore}},
		Discovery: testRunDiscoveryServices(t, jobConfig),
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
		Input: reader, Output: output,
		ShutdownTimeout: time.Second,
		Modules:         collectorapi.Registry{}, Jobs: jobs,
		Discovery: testRunDiscoveryServices(t),
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

func TestProcessCoreSecretUpdateHoldsJobGraphThroughRestart(t *testing.T) {
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
				return &runTestHandler{cleanup: func() {}}
			},
			JobConfigSchema: collectorapi.MockConfigSchema,
		},
	}
	jobConfig := confgroup.Config{
		"module": "module", "name": "job",
		"update_every": 1, "function_only": true,
		"option_str": "${store:vault:main:key}",
		"option_int": 1,
	}
	jobConfig.SetProvider(confgroup.TypeDyncfg)
	jobConfig.SetSourceType(confgroup.TypeDyncfg)
	jobConfig.SetSource("test")
	jobs := testRunJobServices(t)
	jobs.Defaults = confgroup.Registry{"module": {UpdateEvery: 1}}
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
		"name": "main", "kind": string(secretstore.KindVault),
		"value":           "initial",
		"__source__":      confgroup.TypeUser,
		"__source_type__": confgroup.TypeUser,
	}
	reader, writer := io.Pipe()
	defer func() { require.NoError(t, writer.Close()) }()
	output := newProcessSynchronizedBuffer()
	process, err := newProcessCore(processCoreConfig{
		Input: reader, Output: output,
		ShutdownTimeout: time.Second,
		Modules:         modules, Jobs: jobs,
		Secrets:   runSecretServices{Initial: []secretstore.Config{initialStore}},
		Discovery: testRunDiscoveryServices(t, jobConfig),
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

	waitActiveUIDs(t, process.uids, 2)

	require.NotContains(t, output.String(), "FUNCTION_RESULT_BEGIN job-add")

	releaseOnce.Do(func() { close(releaseRestart) })
	output.waitContains(t, "FUNCTION_RESULT_BEGIN secret-rotation 200 application/json")
	output.waitContains(t, "FUNCTION_RESULT_BEGIN job-add 202 application/json")
	output.waitContains(t, "CONFIG go.d:collector:module:other create accepted job")

	commands <- testProcessControl(processTerminate)
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		require.FailNow(t, "test failed", "process did not terminate")
	}
}

func waitActiveUIDs(t *testing.T, uids *lifecycle.UIDLedger, want int) {
	t.Helper()
	timeout := time.NewTimer(3 * time.Second)
	defer timeout.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		active, _, _ := uids.Census()
		if active >= want {
			return
		}
		select {
		case <-tick.C:
		case <-timeout.C:
			require.FailNowf(t, "test failed", "active UIDs=%d want at least %d", active, want)
		}
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
