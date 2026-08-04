// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	awsbackend "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore/backends/aws"
	vaultbackend "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore/backends/vault"
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
	generation, err := newTestRunGeneration(t, runGenerationConfig{
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
	output.waitContains(t, "CONFIG go.d:secretstore:vault:valid create running job")
	output.waitContains(t, "CONFIG go.d:collector:module:unrelated create running job")
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
			testProcessCoreSecretMutationDependentRestart(t, "update", test.restartErr, false, false)
		})
	}
}

func TestProcessCoreSecretAddReplayDependentRestart(t *testing.T) {
	testProcessCoreSecretMutationDependentRestart(t, "add", nil, false, false)
}

func TestProcessCoreSecretUpdateDuringInitialCandidateRetriesLatestStoreGeneration(t *testing.T) {
	testProcessCoreSecretMutationDependentRestart(t, "update", nil, true, false)
}

func TestProcessCoreShutdownDuringPendingSecretCandidatePromotionQuiesces(t *testing.T) {
	testProcessCoreSecretMutationDependentRestart(t, "update", nil, true, true)
}

func testProcessCoreSecretMutationDependentRestart(
	t *testing.T,
	command string,
	restartErr error,
	gateInitial bool,
	terminateAfterReplacementInit bool,
) {
	t.Helper()
	starts := make(chan string, 4)
	initialEntered := make(chan struct{})
	releaseInitial := make(chan struct{})
	var enterInitialOnce sync.Once
	var releaseInitialOnce sync.Once
	t.Cleanup(func() {
		releaseInitialOnce.Do(func() { close(releaseInitial) })
	})
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
					if gateInitial && collector.Config.OptionStr == "initial" {
						enterInitialOnce.Do(func() { close(initialEntered) })
						select {
						case <-releaseInitial:
						case <-ctx.Done():
							return context.Cause(ctx)
						}
					}
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
	controls := newTestProcessControls(1)
	done := make(chan error, 1)
	go func() {
		done <- process.run(context.Background(), controls)
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
	if gateInitial {
		select {
		case <-initialEntered:
		case <-time.After(3 * time.Second):
			require.FailNow(t, "test failed", "initial collector did not enter its gated initialization")
		}
	} else {
		output.waitContains(t, "CONFIG go.d:collector:module:job create running job")
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

	if gateInitial {
		// The Store mutation commits while the brand-new job is still outside
		// the acknowledged graph. Releasing Init then exercises the generation
		// fence: the stale candidate is rejected and discovery retries it.
		output.waitContains(t, "FUNCTION_RESULT_BEGIN "+uid+" 200 application/json")
		releaseInitialOnce.Do(func() { close(releaseInitial) })
	}
	waitSecretStart(t, starts, "replacement")
	if gateInitial && !terminateAfterReplacementInit {
		output.waitContains(t, "CONFIG go.d:collector:module:job create running job")
	}
	if !gateInitial {
		output.waitContains(t, "FUNCTION_RESULT_BEGIN "+uid+" 200 application/json")
	}
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

	controls.sendTerminate(testProcessControl())
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
	replacementEntered := make(chan struct{})
	releaseReplacement := make(chan struct{})
	var replacementEnteredOnce sync.Once
	var releaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(releaseReplacement) })
	})
	var cleanups atomic.Int32
	modules := collectorapi.Registry{
		"module": {
			Create: func() collectorapi.CollectorV1 {
				collector := &collectorapi.MockCollectorV1{}
				collector.InitFunc = func(context.Context) error {
					starts <- collector.Config.OptionStr
					if collector.Config.OptionStr == "replacement" {
						replacementEnteredOnce.Do(func() { close(replacementEntered) })
						<-releaseReplacement
					}
					return nil
				}
				collector.CleanupFunc = func(context.Context) {
					cleanups.Add(1)
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
	controls := newTestProcessControls(1)
	done := make(chan error, 1)
	go func() {
		done <- process.run(context.Background(), controls)
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
	// Collector Init precedes graph publication and dependency acknowledgement.
	// The Running frame makes the initial dependent authoritative before update.
	output.waitContains(t, "CONFIG go.d:collector:module:job create running job")

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
	case <-replacementEntered:
	case <-time.After(3 * time.Second):
		require.FailNow(t, "test failed", "replacement collector did not enter initialization")
	}

	_, writeStringErr2 := io.WriteString(
		writer,
		"FUNCTION_CANCEL secret-cancel\n"+
			"FUNCTION secret-cancel-barrier invalid \"missing:route\" 0xFFFF \"user=test\"\n",
	)
	require.NoError(t, writeStringErr2)
	// Pipe writes acknowledge byte consumption, so a following processed
	// command is the barrier that makes the preceding cancellation authoritative.
	output.waitContains(t, "FUNCTION_RESULT_BEGIN secret-cancel-barrier 400 application/json")

	// Replacement Init belongs to the protected recovery child. Releasing it
	// completes the already-started replacement after parent cancellation.
	releaseOnce.Do(func() { close(releaseReplacement) })
	waitSecretStart(t, starts, "replacement")
	output.waitContains(t, "CONFIG go.d:collector:module:job status running")
	output.waitContains(t, "FUNCTION_RESULT_BEGIN secret-cancel 499 application/json")
	require.NotContains(t, output.String(), "FUNCTION_RESULT_BEGIN secret-cancel 200 application/json")

	require.EqualValues(t, 1, cleanups.Load())

	controls.sendTerminate(testProcessControl())
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
	controls := newTestProcessControls(1)
	done := make(chan error, 1)
	go func() {
		done <- process.run(context.Background(), controls)
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
		{
			uid:     "secret-update-public-invalid",
			command: "config go.d:secretstore:vault:main update",
			payload: `{"value":"public-failure"}`, status: 400,
		},
		{
			uid:     "secret-update-invalid",
			command: "config go.d:secretstore:vault:main update",
			payload: `{"value":"backend-sensitive-detail"}`, status: 400,
		},
		{uid: "secret-get-after-invalid", command: "config go.d:secretstore:vault:main get", status: 200},
		{uid: "secret-test", command: "config go.d:secretstore:vault:main test", status: 200},
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
	require.Contains(t, output.String(), `"Value":"initial"`)
	require.Contains(
		t,
		output.String(),
		"an operational result is unavailable for this secretstore configuration",
	)
	require.Contains(
		t,
		output.String(),
		"Secretstore configuration validation failed: the configured provider credential is unavailable",
	)
	require.NotContains(t, output.String(), "backend-sensitive-detail")
	require.NotContains(t, output.String(), "private provider response")

	controls.sendTerminate(testProcessControl())
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		require.FailNow(t, "test failed", "process did not terminate")
	}
}

func TestProcessCoreVaultOperationalTest(t *testing.T) {
	const (
		validToken      = "synthetic-valid-token"
		restrictedToken = "synthetic-restricted-token"
		invalidToken    = "synthetic-invalid-token"
	)
	var requests atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requests.Add(1)
		if req.Method != http.MethodGet || req.URL.Path != "/v1/auth/token/lookup-self" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch req.Header.Get("X-Vault-Token") {
		case validToken:
			_, _ = io.WriteString(w, `{"data":{}}`)
		case restrictedToken:
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"errors":["permission denied"]}`)
		default:
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(
				w,
				"{\"errors\":[\"2 errors occurred:\\n\\t* permission denied\\n\\t* invalid token\\n\\n\"]}",
			)
		}
	}))
	defer srv.Close()

	creators, err := secretstore.NewCreatorCatalog([]secretstore.Creator{vaultbackend.New()})
	require.NoError(t, err)
	jobs := testRunJobServices(t)
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
	controls := newTestProcessControls(1)
	done := make(chan error, 1)
	go func() {
		done <- process.run(context.Background(), controls)
	}()
	output.waitContains(t, "CONFIG go.d:secretstore:vault create accepted template")

	validPayload := `{"mode":"token","mode_token":{"token":"` + validToken +
		`"},"addr":"` + srv.URL + `"}`
	restrictedPayload := `{"mode":"token","mode_token":{"token":"` + restrictedToken +
		`"},"addr":"` + srv.URL + `"}`
	invalidPayload := `{"mode":"token","mode_token":{"token":"` + invalidToken +
		`"},"addr":"` + srv.URL + `"}`
	steps := []struct {
		uid     string
		command string
		payload string
		status  int
	}{
		{
			uid:     "vault-add",
			command: "config go.d:secretstore:vault add main",
			payload: validPayload,
			status:  200,
		},
		{
			uid:     "vault-test-stored",
			command: "config go.d:secretstore:vault:main test",
			status:  202,
		},
		{
			uid:     "vault-test-restricted",
			command: "config go.d:secretstore:vault:main test",
			payload: restrictedPayload,
			status:  200,
		},
		{
			uid:     "vault-test-invalid",
			command: "config go.d:secretstore:vault:main test",
			payload: invalidPayload,
			status:  422,
		},
		{
			uid:     "vault-test-stored-again",
			command: "config go.d:secretstore:vault:main test",
			status:  202,
		},
	}
	for _, step := range steps {
		if step.payload == "" {
			_, err = io.WriteString(
				writer,
				"FUNCTION "+step.uid+" 30 \""+step.command+"\" 0xFFFF \"user=test\"\n",
			)
		} else {
			_, err = io.WriteString(
				writer,
				"FUNCTION_PAYLOAD "+step.uid+" 30 \""+step.command+
					"\" 0xFFFF \"user=test\" application/json\n"+
					step.payload+"\nFUNCTION_PAYLOAD_END\n",
			)
		}
		require.NoError(t, err)
		output.waitContains(
			t,
			"FUNCTION_RESULT_BEGIN "+step.uid+" "+strconv.Itoa(step.status)+" application/json",
		)
	}

	wire := output.String()
	restrictedStart := strings.Index(wire, "FUNCTION_RESULT_BEGIN vault-test-restricted ")
	require.NotEqual(t, -1, restrictedStart)
	restrictedEnd := strings.Index(wire[restrictedStart:], "FUNCTION_RESULT_END")
	require.NotEqual(t, -1, restrictedEnd)
	restrictedResult := wire[restrictedStart : restrictedStart+restrictedEnd]

	require.EqualValues(t, 4, requests.Load())
	require.Contains(
		t,
		wire,
		"Secretstore operational test failed: the configured Vault authentication check failed",
	)
	require.Contains(
		t,
		restrictedResult,
		"an operational result is unavailable for this secretstore configuration",
	)
	require.Contains(t, restrictedResult, "No jobs currently use this secretstore.")
	require.Contains(t, wire, "Stored configuration is valid. No jobs are currently using this secretstore.")
	require.NotContains(t, wire, "invalid token")
	require.NotContains(t, wire, validToken)
	require.NotContains(t, wire, restrictedToken)
	require.NotContains(t, wire, invalidToken)
	require.NotContains(t, wire, srv.URL)

	controls.sendTerminate(testProcessControl())
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		require.FailNow(t, "test failed", "process did not terminate")
	}
}

func TestProcessCoreAWSOperationalTest(t *testing.T) {
	const (
		accessKey = "synthetic-access-key"
		secretKey = "synthetic-secret-key"
	)
	t.Setenv("AWS_ACCESS_KEY_ID", accessKey)
	t.Setenv("AWS_SECRET_ACCESS_KEY", secretKey)
	t.Setenv("AWS_SESSION_TOKEN", "")
	t.Setenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "")

	creators, err := secretstore.NewCreatorCatalog([]secretstore.Creator{awsbackend.New()})
	require.NoError(t, err)
	jobs := testRunJobServices(t)
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
	controls := newTestProcessControls(1)
	done := make(chan error, 1)
	go func() {
		done <- process.run(context.Background(), controls)
	}()
	output.waitContains(t, "CONFIG go.d:secretstore:aws-sm create accepted template")

	storedPayload := `{"auth_mode":"env","region":"us-east-1"}`
	temporaryECSPayload := `{"auth_mode":"ecs","region":"us-east-1"}`
	steps := []struct {
		uid     string
		command string
		payload string
		status  int
	}{
		{
			uid:     "aws-add",
			command: "config go.d:secretstore:aws-sm add main",
			payload: storedPayload,
			status:  200,
		},
		{
			uid:     "aws-test-stored",
			command: "config go.d:secretstore:aws-sm:main test",
			status:  202,
		},
		{
			uid:     "aws-test-temporary-ecs",
			command: "config go.d:secretstore:aws-sm:main test",
			payload: temporaryECSPayload,
			status:  422,
		},
		{
			uid:     "aws-test-stored-again",
			command: "config go.d:secretstore:aws-sm:main test",
			status:  202,
		},
	}
	for _, step := range steps {
		if step.payload == "" {
			_, err = io.WriteString(
				writer,
				"FUNCTION "+step.uid+" 30 \""+step.command+"\" 0xFFFF \"user=test\"\n",
			)
		} else {
			_, err = io.WriteString(
				writer,
				"FUNCTION_PAYLOAD "+step.uid+" 30 \""+
					step.command+"\" 0xFFFF \"user=test\" application/json\n"+
					step.payload+"\nFUNCTION_PAYLOAD_END\n",
			)
		}
		require.NoError(t, err)
		output.waitContains(
			t,
			"FUNCTION_RESULT_BEGIN "+step.uid+" "+strconv.Itoa(step.status)+" application/json",
		)
	}

	wire := output.String()
	require.Contains(t, wire, "Secretstore operational test failed: AWS credentials are unavailable")
	require.Contains(t, wire, "Stored configuration is valid. No jobs are currently using this secretstore.")
	require.NotContains(t, wire, "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI")
	require.NotContains(t, wire, accessKey)
	require.NotContains(t, wire, secretKey)

	controls.sendTerminate(testProcessControl())
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		require.FailNow(t, "test failed", "process did not terminate")
	}
}

func TestProcessCoreCancelledStoreMaterializationDoesNotHoldJobGraph(t *testing.T) {
	gate := newProcessBlockingStoreGate()
	t.Cleanup(gate.release)
	modules := collectorapi.Registry{
		"module": {
			Create: func() collectorapi.CollectorV1 {
				return &collectorapi.MockCollectorV1{
					InitFunc: func(context.Context) error { return nil },
					ChartsFunc: func() *collectorapi.Charts {
						return &collectorapi.Charts{&collectorapi.Chart{
							ID:    "chart",
							Title: "chart",
							Units: "value",
							Dims: collectorapi.Dims{&collectorapi.Dim{
								ID: "value",
							}},
						}}
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
	jobs := testRunJobServices(t)
	jobs.Defaults = confgroup.Registry{"module": {UpdateEvery: 1}}
	creators, err := secretstore.NewCreatorCatalog([]secretstore.Creator{{
		Kind:   secretstore.KindVault,
		Schema: `{}`,
		Create: func() secretstore.Store {
			return &processBlockingSecretStore{gate: gate}
		},
	}})
	require.NoError(t, err)
	jobs.StoreCreators = creators

	reader, writer := io.Pipe()
	defer func() { require.NoError(t, writer.Close()) }()
	output := newProcessSynchronizedBuffer()
	process, err := newProcessCore(processCoreConfig{
		Input:           reader,
		Output:          output,
		ShutdownTimeout: time.Second,
		Modules:         modules,
		Jobs:            jobs,
		Discovery:       testRunDiscoveryServices(t),
		Diagnostics:     testProcessDiagnostics(),
	})
	require.NoError(t, err)
	controls := newTestProcessControls(1)
	done := make(chan error, 1)
	go func() {
		done <- process.run(context.Background(), controls)
	}()
	output.waitContains(t, "CONFIG go.d:secretstore:vault create accepted template")

	_, err = io.WriteString(
		writer,
		"FUNCTION_PAYLOAD secret-blocked 30 "+
			"\"config go.d:secretstore:vault add main\" "+
			"0xFFFF \"user=test\" application/json\n"+
			"{\"value\":\"blocked\"}\n"+
			"FUNCTION_PAYLOAD_END\n",
	)
	require.NoError(t, err)
	select {
	case <-gate.entered:
	case <-time.After(3 * time.Second):
		require.FailNow(t, "test failed", "Store provider did not enter blocking Init")
	}

	_, err = io.WriteString(writer, "FUNCTION_CANCEL secret-blocked\n")
	require.NoError(t, err)
	_, err = io.WriteString(
		writer,
		"FUNCTION_PAYLOAD job-add-while-store-blocked 30 "+
			"\"config go.d:collector:module add unrelated\" "+
			"0xFFFF \"user=test\" application/json\n"+
			"{\"option_str\":\"plain\",\"option_int\":1}\n"+
			"FUNCTION_PAYLOAD_END\n",
	)
	require.NoError(t, err)

	progressBeforeRelease := false
	timeout := time.NewTimer(500 * time.Millisecond)
	for !progressBeforeRelease {
		if strings.Contains(
			output.String(),
			"FUNCTION_RESULT_BEGIN job-add-while-store-blocked 202 application/json",
		) {
			progressBeforeRelease = true
			break
		}
		select {
		case <-output.writes:
		case <-timeout.C:
			goto release
		}
	}

release:
	if !timeout.Stop() {
		select {
		case <-timeout.C:
		default:
		}
	}
	gate.release()
	output.waitContains(t, "FUNCTION_RESULT_BEGIN secret-blocked 499 application/json")
	output.waitContains(t, "FUNCTION_RESULT_BEGIN job-add-while-store-blocked 202 application/json")

	controls.sendTerminate(testProcessControl())
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		require.FailNow(t, "test failed", "process did not terminate")
	}
	require.True(t, progressBeforeRelease, "Store materialization held the job graph after cancellation")
}

func TestProcessCoreRotationFencesLateStoreMaterializationAndFreshEpochProceeds(t *testing.T) {
	gate := newProcessBlockingStoreGate()
	t.Cleanup(gate.release)
	jobs := testRunJobServices(t)
	creators, err := secretstore.NewCreatorCatalog([]secretstore.Creator{{
		Kind:   secretstore.KindVault,
		Schema: `{}`,
		Create: func() secretstore.Store {
			return &processBlockingSecretStore{gate: gate}
		},
	}})
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
	controls := newTestProcessControls(2)
	done := make(chan error, 1)
	go func() {
		done <- process.run(context.Background(), controls)
	}()
	output.waitContains(t, "CONFIG go.d:secretstore:vault create accepted template")

	_, err = io.WriteString(
		writer,
		"FUNCTION_PAYLOAD secret-old-epoch 30 "+
			"\"config go.d:secretstore:vault add main\" "+
			"0xFFFF \"user=test\" application/json\n"+
			"{\"value\":\"blocked\"}\n"+
			"FUNCTION_PAYLOAD_END\n",
	)
	require.NoError(t, err)
	select {
	case <-gate.entered:
	case <-time.After(3 * time.Second):
		require.FailNow(t, "test failed", "old-epoch Store provider did not enter blocking Init")
	}

	restart := testProcessControl()
	controls.sendRestart(restart)
	select {
	case err := <-restart.result:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		require.FailNow(t, "test failed", "rotation waited for old-epoch Store materialization")
	}

	_, err = io.WriteString(
		writer,
		"FUNCTION_PAYLOAD secret-fresh-epoch 30 "+
			"\"config go.d:secretstore:vault add main\" "+
			"0xFFFF \"user=test\" application/json\n"+
			"{\"value\":\"replacement\"}\n"+
			"FUNCTION_PAYLOAD_END\n",
	)
	require.NoError(t, err)
	output.waitContains(t, "FUNCTION_RESULT_BEGIN secret-fresh-epoch 200 application/json")

	_, err = io.WriteString(
		writer,
		"FUNCTION secret-fresh-get 30 "+
			"\"config go.d:secretstore:vault:main get\" "+
			"0xFFFF \"user=test\"\n",
	)
	require.NoError(t, err)
	output.waitContains(t, "FUNCTION_RESULT_BEGIN secret-fresh-get 200 application/json")
	require.Contains(t, output.String(), `"Value":"replacement"`)

	gate.release()
	terminate := testProcessControl()
	controls.sendTerminate(terminate)
	require.NoError(t, <-terminate.result)
	require.NoError(t, <-done)
}

func TestProcessCoreStoreRemovalCancelsPendingMaterializationAuthoritatively(t *testing.T) {
	t.Run("running update", func(t *testing.T) {
		testProcessCoreStoreRemovalCancelsPendingMaterialization(t, true)
	})
	t.Run("previously absent add", func(t *testing.T) {
		testProcessCoreStoreRemovalCancelsPendingMaterialization(t, false)
	})
}

func testProcessCoreStoreRemovalCancelsPendingMaterialization(t *testing.T, installInitial bool) {
	gate := newProcessBlockingStoreGate()
	t.Cleanup(gate.release)
	jobs := testRunJobServices(t)
	creators, err := secretstore.NewCreatorCatalog([]secretstore.Creator{{
		Kind:   secretstore.KindVault,
		Schema: `{}`,
		Create: func() secretstore.Store {
			return &processBlockingSecretStore{gate: gate}
		},
	}})
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
	controls := newTestProcessControls(1)
	done := make(chan error, 1)
	go func() {
		done <- process.run(context.Background(), controls)
	}()
	output.waitContains(t, "CONFIG go.d:secretstore:vault create accepted template")

	if installInitial {
		_, err = io.WriteString(
			writer,
			"FUNCTION_PAYLOAD secret-remove-initial 30 "+
				"\"config go.d:secretstore:vault add main\" "+
				"0xFFFF \"user=test\" application/json\n"+
				"{\"value\":\"initial\"}\n"+
				"FUNCTION_PAYLOAD_END\n",
		)
		require.NoError(t, err)
		output.waitContains(t, "FUNCTION_RESULT_BEGIN secret-remove-initial 200 application/json")
	}

	command := "\"config go.d:secretstore:vault add main\" "
	if installInitial {
		command = "\"config go.d:secretstore:vault:main update\" "
	}
	_, err = io.WriteString(
		writer,
		"FUNCTION_PAYLOAD secret-remove-blocked 30 "+
			command+
			"0xFFFF \"user=test\" application/json\n"+
			"{\"value\":\"blocked\"}\n"+
			"FUNCTION_PAYLOAD_END\n",
	)
	require.NoError(t, err)
	select {
	case <-gate.entered:
	case <-time.After(3 * time.Second):
		require.FailNow(t, "test failed", "Store update did not enter blocking Init")
	}

	_, err = io.WriteString(
		writer,
		"FUNCTION secret-remove 30 "+
			"\"config go.d:secretstore:vault:main remove\" "+
			"0xFFFF \"user=test\"\n",
	)
	require.NoError(t, err)
	output.waitContains(t, "FUNCTION_RESULT_BEGIN secret-remove-blocked 503 application/json")
	output.waitContains(t, "FUNCTION_RESULT_BEGIN secret-remove 200 application/json")

	gate.release()
	require.Eventually(t, func() bool {
		return process.attempts.Census().Active == 0
	}, time.Second, 10*time.Millisecond)

	_, err = io.WriteString(
		writer,
		"FUNCTION secret-remove-get 30 "+
			"\"config go.d:secretstore:vault:main get\" "+
			"0xFFFF \"user=test\"\n",
	)
	require.NoError(t, err)
	output.waitContains(t, "FUNCTION_RESULT_BEGIN secret-remove-get 404 application/json")

	controls.sendTerminate(testProcessControl())
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		require.FailNow(t, "test failed", "process did not terminate")
	}
}

func TestProcessCoreRetriesLatestPendingStoreAfterStuckIdentityReleases(t *testing.T) {
	gate := newProcessBlockingStoreGate()
	t.Cleanup(gate.release)
	jobs := testRunJobServices(t)
	creators, err := secretstore.NewCreatorCatalog([]secretstore.Creator{{
		Kind:   secretstore.KindVault,
		Schema: `{}`,
		Create: func() secretstore.Store {
			return &processBlockingSecretStore{gate: gate}
		},
	}})
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
	controls := newTestProcessControls(1)
	done := make(chan error, 1)
	go func() {
		done <- process.run(context.Background(), controls)
	}()
	output.waitContains(t, "CONFIG go.d:secretstore:vault create accepted template")

	_, err = io.WriteString(
		writer,
		"FUNCTION_PAYLOAD secret-pending-v1 30 "+
			"\"config go.d:secretstore:vault add main\" "+
			"0xFFFF \"user=test\" application/json\n"+
			"{\"value\":\"blocked\"}\n"+
			"FUNCTION_PAYLOAD_END\n",
	)
	require.NoError(t, err)
	select {
	case <-gate.entered:
	case <-time.After(3 * time.Second):
		require.FailNow(t, "test failed", "first Store provider did not enter blocking Init")
	}
	_, err = io.WriteString(
		writer,
		"FUNCTION_PAYLOAD secret-pending-v2 30 "+
			"\"config go.d:secretstore:vault add main\" "+
			"0xFFFF \"user=test\" application/json\n"+
			"{\"value\":\"replacement\"}\n"+
			"FUNCTION_PAYLOAD_END\n",
	)
	require.NoError(t, err)

	output.waitContains(t, "FUNCTION_RESULT_BEGIN secret-pending-v1 503 application/json")
	output.waitContains(t, "FUNCTION_RESULT_BEGIN secret-pending-v2 503 application/json")
	gate.release()
	output.waitContains(t, "CONFIG go.d:secretstore:vault:main create running job")

	_, err = io.WriteString(
		writer,
		"FUNCTION secret-pending-get 30 "+
			"\"config go.d:secretstore:vault:main get\" "+
			"0xFFFF \"user=test\"\n",
	)
	require.NoError(t, err)
	output.waitContains(t, "FUNCTION_RESULT_BEGIN secret-pending-get 200 application/json")
	require.Contains(t, output.String(), `"Value":"replacement"`)

	controls.sendTerminate(testProcessControl())
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
	controls := newTestProcessControls(1)
	done := make(chan error, 1)
	go func() {
		done <- process.run(context.Background(), controls)
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

	controls.sendTerminate(testProcessControl())
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

type processBlockingStoreGate struct {
	entered  chan struct{}
	releaseC chan struct{}
	once     sync.Once
}

func newProcessBlockingStoreGate() *processBlockingStoreGate {
	return &processBlockingStoreGate{
		entered:  make(chan struct{}),
		releaseC: make(chan struct{}),
	}
}

func (pbsg *processBlockingStoreGate) release() {
	pbsg.once.Do(func() {
		close(pbsg.releaseC)
	})
}

type processBlockingSecretStore struct {
	gate   *processBlockingStoreGate
	config struct {
		Value string `yaml:"value"`
	}
}

func (pbss *processBlockingSecretStore) Configuration() any {
	return &pbss.config
}

func (pbss *processBlockingSecretStore) Init(context.Context) error {
	if pbss.config.Value != "blocked" {
		return nil
	}
	select {
	case <-pbss.gate.entered:
	default:
		close(pbss.gate.entered)
	}
	<-pbss.gate.releaseC
	return nil
}

func (pbss *processBlockingSecretStore) Publish() secretstore.PublishedStore {
	return processPublishedSecret(pbss.config.Value)
}

func (pss *processSecretStore) Configuration() any {
	return &pss.config
}

func (pss *processSecretStore) Init(context.Context) error {
	if pss.config.Value == "public-failure" {
		return dyncfg.NewPublicError(
			"the configured provider credential is unavailable",
			errors.New("private provider response"),
		)
	}
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
