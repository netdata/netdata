// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"gopkg.in/yaml.v2"
)

func TestProcessCoreSecretUpdateRestartsDependentAgainstNewGeneration(
	t *testing.T,
) {
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
			MethodHandler: func(
				collectorapi.RuntimeJob,
			) funcapi.MethodHandler {
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
	jobPayload, err := yaml.Marshal(jobConfig)
	if err != nil {
		t.Fatal(err)
	}
	jobs := testRunJobServices(t)
	jobs.Defaults = confgroup.Registry{
		"module": {UpdateEvery: 1},
	}
	jobs.Graph = []dyncfg.GraphConfig{{
		ID: jobConfig.FullName(), Module: jobConfig.Module(),
		Name: jobConfig.Name(), Status: dyncfg.StatusRunning.String(),
		Payload: jobPayload,
	}}
	jobs.StoreCreators, err = secretstore.NewCreatorCatalog(
		[]secretstore.Creator{{
			Kind:        secretstore.KindVault,
			DisplayName: "Vault",
			Schema:      `{}`,
			Create: func() secretstore.Store {
				return &processSecretStore{}
			},
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	initialStore := secretstore.Config{
		"name": "main", "kind": string(secretstore.KindVault),
		"value":           "initial",
		"__source__":      confgroup.TypeUser,
		"__source_type__": confgroup.TypeUser,
	}

	reader, writer := io.Pipe()
	defer writer.Close()
	output := newProcessSynchronizedBuffer()
	var graph *dyncfg.Graph
	process, err := newProcessCore(processCoreConfig{
		Input: reader, Output: output, FirstGeneration: 1,
		ShutdownTimeout: time.Second, Clock: lifecycle.RealClock{},
		Modules: modules, Jobs: jobs,
		Secrets: runSecretServices{
			Initial: []secretstore.Config{initialStore},
		},
		Discovery: testRunDiscoveryServices(t),
		Planner: func(
			capabilities runPlannerCapabilities,
		) (jobmgr.Planner, jobmgr.RunFinalizer, error) {
			graph = capabilities.Graph
			return runRejectingPlanner{},
				jobmgr.RunFinalizerFunc(
					func(context.Context, uint64) error { return nil },
				),
				nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	commands := make(chan processControl, 1)
	done := make(chan error, 1)
	go func() {
		done <- process.run(context.Background(), commands)
	}()
	select {
	case got := <-starts:
		if got != "initial" {
			t.Fatalf("collector secret=%q want=%q", got, "initial")
		}
	case err := <-done:
		t.Fatalf(
			"process stopped before initial collector start: %v; output=%q",
			err,
			output.String(),
		)
	case <-time.After(3 * time.Second):
		t.Fatalf(
			"collector did not start with initial secret; graph=%v output=%q",
			graph.IDs(),
			output.String(),
		)
	}

	if _, err := io.WriteString(
		writer,
		"FUNCTION_PAYLOAD secret-update 30 "+
			"\"config go.d:secretstore:vault:main update\" "+
			"0xFFFF \"user=test\" application/json\n"+
			"{\"value\":\"replacement\"}\n"+
			"FUNCTION_PAYLOAD_END\n",
	); err != nil {
		t.Fatal(err)
	}
	waitSecretStart(t, starts, "replacement")
	if cleanups.Load() != 1 {
		t.Fatalf(
			"dependent cleanup count=%d before replacement start",
			cleanups.Load(),
		)
	}
	output.waitContains(
		t,
		"FUNCTION_RESULT_BEGIN secret-update 200 application/json",
	)

	commands <- testProcessControl(processTerminate)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("process did not terminate")
	}
	if cleanups.Load() != 2 {
		t.Fatalf(
			"dependent cleanup count=%d after shutdown",
			cleanups.Load(),
		)
	}
}

func TestProcessCoreCancelledSecretUpdateRestoresStoppedDependent(
	t *testing.T,
) {
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
					if armStop.Load() &&
						cleanups.Add(1) == 1 {
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
			MethodHandler: func(
				collectorapi.RuntimeJob,
			) funcapi.MethodHandler {
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
	jobPayload, err := yaml.Marshal(jobConfig)
	if err != nil {
		t.Fatal(err)
	}
	jobs := testRunJobServices(t)
	jobs.Defaults = confgroup.Registry{
		"module": {UpdateEvery: 1},
	}
	jobs.Graph = []dyncfg.GraphConfig{{
		ID: jobConfig.FullName(), Module: jobConfig.Module(),
		Name: jobConfig.Name(), Status: dyncfg.StatusRunning.String(),
		Payload: jobPayload,
	}}
	jobs.StoreCreators, err = secretstore.NewCreatorCatalog(
		[]secretstore.Creator{{
			Kind:        secretstore.KindVault,
			DisplayName: "Vault",
			Schema:      `{}`,
			Create: func() secretstore.Store {
				return &processSecretStore{}
			},
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	initialStore := secretstore.Config{
		"name": "main", "kind": string(secretstore.KindVault),
		"value":           "initial",
		"__source__":      confgroup.TypeUser,
		"__source_type__": confgroup.TypeUser,
	}

	reader, writer := io.Pipe()
	defer writer.Close()
	output := newProcessSynchronizedBuffer()
	var storeScope func(
		[]string,
	) (secretresolver.AtomicScope, error)
	var storeCensus func() secretstore.SecretStoreCensus
	process, err := newProcessCore(processCoreConfig{
		Input: reader, Output: output, FirstGeneration: 1,
		ShutdownTimeout: time.Second, Clock: lifecycle.RealClock{},
		Modules: modules, Jobs: jobs,
		Secrets: runSecretServices{
			Initial: []secretstore.Config{initialStore},
		},
		Discovery: testRunDiscoveryServices(t),
		Planner: func(
			capabilities runPlannerCapabilities,
		) (jobmgr.Planner, jobmgr.RunFinalizer, error) {
			storeScope = capabilities.StoreScope
			storeCensus = capabilities.StoreCensus
			return runRejectingPlanner{},
				jobmgr.RunFinalizerFunc(
					func(context.Context, uint64) error { return nil },
				),
				nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	commands := make(chan processControl, 1)
	done := make(chan error, 1)
	go func() {
		done <- process.run(context.Background(), commands)
	}()
	select {
	case got := <-starts:
		if got != "initial" {
			t.Fatalf(
				"collector secret=%q want=%q",
				got,
				"initial",
			)
		}
	case err := <-done:
		t.Fatalf(
			"process stopped before initial collector start: %v; output=%q",
			err,
			output.String(),
		)
	case <-time.After(3 * time.Second):
		t.Fatalf(
			"collector did not start with initial secret; output=%q",
			output.String(),
		)
	}
	armStop.Store(true)

	if _, err := io.WriteString(
		writer,
		"FUNCTION_PAYLOAD secret-cancel 30 "+
			"\"config go.d:secretstore:vault:main update\" "+
			"0xFFFF \"user=test\" application/json\n"+
			"{\"value\":\"replacement\"}\n"+
			"FUNCTION_PAYLOAD_END\n",
	); err != nil {
		t.Fatal(err)
	}
	select {
	case <-stopEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("dependent stop did not reach collector cleanup")
	}
	if _, err := io.WriteString(
		writer,
		"FUNCTION_CANCEL secret-cancel\n",
	); err != nil {
		t.Fatal(err)
	}
	releaseOnce.Do(func() { close(releaseStop) })
	waitSecretStart(t, starts, "initial")
	output.waitContains(
		t,
		"FUNCTION_RESULT_BEGIN secret-cancel 499 application/json",
	)

	key := secretstore.StoreKey(secretstore.KindVault, "main")
	scope, err := storeScope([]string{key})
	if err != nil {
		t.Fatal(err)
	}
	value, resolveErr := scope.Resolve(
		t.Context(),
		key,
		"key",
	)
	releaseErr := scope.Release(t.Context())
	if resolveErr != nil || releaseErr != nil ||
		string(value) != "initial" {
		t.Fatalf(
			"restored Store value=%q resolve=%v release=%v",
			value,
			resolveErr,
			releaseErr,
		)
	}
	if census := storeCensus(); census.Current != 1 ||
		census.Generations != 1 ||
		census.Preparations != 0 ||
		census.Readers != 0 ||
		census.Scopes != 0 {
		t.Fatalf(
			"cancelled Store update retained ownership: %+v",
			census,
		)
	}
	if cleanups.Load() != 1 {
		t.Fatalf(
			"dependent cleanups=%d want=1 before shutdown",
			cleanups.Load(),
		)
	}

	commands <- testProcessControl(processTerminate)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("process did not terminate")
	}
	if cleanups.Load() != 2 {
		t.Fatalf(
			"dependent cleanups=%d want=2 after shutdown",
			cleanups.Load(),
		)
	}
}

func TestProcessCoreSecretCRUDAndValidationRedaction(t *testing.T) {
	jobs := testRunJobServices(t)
	var err error
	jobs.StoreCreators, err = secretstore.NewCreatorCatalog(
		[]secretstore.Creator{{
			Kind:        secretstore.KindVault,
			DisplayName: "Vault",
			Schema:      `{}`,
			Create: func() secretstore.Store {
				return &processSecretStore{}
			},
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	reader, writer := io.Pipe()
	defer writer.Close()
	output := newProcessSynchronizedBuffer()
	process, err := newProcessCore(processCoreConfig{
		Input: reader, Output: output, FirstGeneration: 1,
		ShutdownTimeout: time.Second, Clock: lifecycle.RealClock{},
		Modules: collectorapi.Registry{}, Jobs: jobs,
		Discovery: testRunDiscoveryServices(t),
		Planner: func(
			runPlannerCapabilities,
		) (jobmgr.Planner, jobmgr.RunFinalizer, error) {
			return runRejectingPlanner{},
				jobmgr.RunFinalizerFunc(
					func(context.Context, uint64) error { return nil },
				),
				nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	commands := make(chan processControl, 1)
	done := make(chan error, 1)
	go func() {
		done <- process.run(context.Background(), commands)
	}()
	output.waitContains(
		t,
		"CONFIG go.d:secretstore:vault create accepted template",
	)

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
		{
			uid:     "secret-get",
			command: "config go.d:secretstore:vault:main get",
			status:  200,
		},
		{
			uid:     "secret-test",
			command: "config go.d:secretstore:vault:main test",
			status:  202,
		},
		{
			uid:     "secret-update",
			command: "config go.d:secretstore:vault:main update",
			payload: `{"value":"replacement"}`, status: 200,
		},
		{
			uid:     "secret-remove",
			command: "config go.d:secretstore:vault:main remove",
			status:  200,
		},
		{
			uid:     "secret-invalid",
			command: "config go.d:secretstore:vault add invalid",
			payload: `{"value":"backend-sensitive-detail"}`, status: 400,
		},
	}
	for _, step := range steps {
		if step.payload == "" {
			if _, err := io.WriteString(
				writer,
				"FUNCTION "+step.uid+" 30 \""+
					step.command+
					"\" 0xFFFF \"user=test\"\n",
			); err != nil {
				t.Fatal(err)
			}
		} else if _, err := io.WriteString(
			writer,
			"FUNCTION_PAYLOAD "+step.uid+" 30 \""+
				step.command+
				"\" 0xFFFF \"user=test\" application/json\n"+
				step.payload+"\nFUNCTION_PAYLOAD_END\n",
		); err != nil {
			t.Fatal(err)
		}
		output.waitContains(
			t,
			"FUNCTION_RESULT_BEGIN "+step.uid+" "+
				strconv.Itoa(step.status)+" application/json",
		)
	}
	if strings.Contains(
		output.String(),
		"backend-sensitive-detail",
	) {
		t.Fatalf(
			"secret validation detail reached protocol output: %q",
			output.String(),
		)
	}

	commands <- testProcessControl(processTerminate)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("process did not terminate")
	}
}

func TestProcessCoreSecretUpdateHoldsJobGraphThroughRestart(
	t *testing.T,
) {
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
			MethodHandler: func(
				collectorapi.RuntimeJob,
			) funcapi.MethodHandler {
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
	jobPayload, err := yaml.Marshal(jobConfig)
	if err != nil {
		t.Fatal(err)
	}
	jobs := testRunJobServices(t)
	jobs.Defaults = confgroup.Registry{
		"module": {UpdateEvery: 1},
	}
	jobs.Graph = []dyncfg.GraphConfig{{
		ID: jobConfig.FullName(), Module: jobConfig.Module(),
		Name: jobConfig.Name(), Status: dyncfg.StatusRunning.String(),
		Payload: jobPayload,
	}}
	jobs.StoreCreators, err = secretstore.NewCreatorCatalog(
		[]secretstore.Creator{{
			Kind:        secretstore.KindVault,
			DisplayName: "Vault",
			Schema:      `{}`,
			Create: func() secretstore.Store {
				return &processSecretStore{}
			},
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	initialStore := secretstore.Config{
		"name": "main", "kind": string(secretstore.KindVault),
		"value":           "initial",
		"__source__":      confgroup.TypeUser,
		"__source_type__": confgroup.TypeUser,
	}
	reader, writer := io.Pipe()
	defer writer.Close()
	output := newProcessSynchronizedBuffer()
	var graph *dyncfg.Graph
	process, err := newProcessCore(processCoreConfig{
		Input: reader, Output: output, FirstGeneration: 1,
		ShutdownTimeout: time.Second, Clock: lifecycle.RealClock{},
		Modules: modules, Jobs: jobs,
		Secrets: runSecretServices{
			Initial: []secretstore.Config{initialStore},
		},
		Discovery: testRunDiscoveryServices(t),
		Planner: func(
			capabilities runPlannerCapabilities,
		) (jobmgr.Planner, jobmgr.RunFinalizer, error) {
			graph = capabilities.Graph
			return runRejectingPlanner{},
				jobmgr.RunFinalizerFunc(
					func(context.Context, uint64) error { return nil },
				),
				nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	commands := make(chan processControl, 1)
	done := make(chan error, 1)
	go func() {
		done <- process.run(context.Background(), commands)
	}()
	output.waitContains(
		t,
		"CONFIG go.d:collector:module:job create running job",
	)
	if _, err := io.WriteString(
		writer,
		"FUNCTION_PAYLOAD secret-rotation 30 "+
			"\"config go.d:secretstore:vault:main update\" "+
			"0xFFFF \"user=test\" application/json\n"+
			"{\"value\":\"replacement\"}\n"+
			"FUNCTION_PAYLOAD_END\n",
	); err != nil {
		t.Fatal(err)
	}
	select {
	case <-restartEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("dependent restart did not reach the blocking start")
	}
	if _, err := io.WriteString(
		writer,
		"FUNCTION_PAYLOAD job-add 30 "+
			"\"config go.d:collector:module add other\" "+
			"0xFFFF \"user=test\" application/json\n"+
			"{\"option_str\":\"plain\",\"option_int\":1}\n"+
			"FUNCTION_PAYLOAD_END\n",
	); err != nil {
		t.Fatal(err)
	}
	waitActiveUIDs(t, process.uids, 2)
	if _, exists := graph.Lookup("module_other"); exists {
		t.Fatal("job graph changed while Store rotation held its claim")
	}
	if strings.Contains(
		output.String(),
		"FUNCTION_RESULT_BEGIN job-add",
	) {
		t.Fatal("job mutation completed before Store rotation released")
	}

	releaseOnce.Do(func() { close(releaseRestart) })
	output.waitContains(
		t,
		"FUNCTION_RESULT_BEGIN secret-rotation 200 application/json",
	)
	output.waitContains(
		t,
		"FUNCTION_RESULT_BEGIN job-add 202 application/json",
	)
	if record, exists := graph.Lookup("module_other"); !exists ||
		record.Status != dyncfg.StatusAccepted.String() {
		t.Fatalf(
			"released job graph record=%+v exists=%v",
			record,
			exists,
		)
	}

	commands <- testProcessControl(processTerminate)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("process did not terminate")
	}
}

func waitActiveUIDs(
	t *testing.T,
	uids *lifecycle.UIDLedger,
	want int,
) {
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
			t.Fatalf("active UIDs=%d want at least %d", active, want)
		}
	}
}

func waitSecretStart(
	t *testing.T,
	starts <-chan string,
	want string,
) {
	t.Helper()
	select {
	case got := <-starts:
		if got != want {
			t.Fatalf("collector secret=%q want=%q", got, want)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("collector did not start with secret %q", want)
	}
}

type processSecretStore struct {
	config struct {
		Value string `yaml:"value"`
	}
}

func (store *processSecretStore) Configuration() any {
	return &store.config
}

func (store *processSecretStore) Init(context.Context) error {
	if store.config.Value == "backend-sensitive-detail" {
		return errors.New(
			"backend rejected backend-sensitive-detail",
		)
	}
	return nil
}

func (store *processSecretStore) Publish() secretstore.PublishedStore {
	return processPublishedSecret(store.config.Value)
}

type processPublishedSecret string

func (secret processPublishedSecret) Resolve(
	context.Context,
	secretstore.ResolveRequest,
) (string, error) {
	return string(secret), nil
}
