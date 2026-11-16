// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	vaultbackend "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore/backends/vault"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/require"
)

func TestSecretWireRejectsChangedIdentity(t *testing.T) {
	for name, command := range map[string]string{
		"GET trailing NBSP":    "config go.d:secretstore:vault:db\u00a0 get",
		"ADD trailing NBSP":    "config go.d:secretstore:vault add db\u00a0",
		"ADD inner NBSP":       "config go.d:secretstore:vault add db\u00a0one",
		"ADD literal hex":      `config go.d:secretstore:vault add d\x62`,
		"ADD trailing slash":   `config go.d:secretstore:vault add db\`,
		"UPDATE trailing NBSP": "config go.d:secretstore:vault:db\u00a0 update",
		"UPDATE leading NBSP":  "config go.d:secretstore:vault:\u00a0db update",
		"UPDATE kind NBSP":     "config go.d:secretstore:vault\u00a0:db update",
		"UPDATE literal hex":   `config go.d:secretstore:vault:d\x62 update`,
	} {
		t.Run(name, func(t *testing.T) {
			p := newSecretAdoptionProcess(t, secretstore.Creator{
				Kind: secretstore.KindVault, Schema: `{}`,
				Create: func() secretstore.Store { return &processSecretStore{} },
			}, []secretstore.Config{{
				"name": "db", "kind": "vault", "value": "original",
				"__source__": "file=test", "__source_type__": confgroup.TypeUser,
			}})
			p.output.waitContains(t, "CONFIG go.d:secretstore:vault:db create running job")
			p.waitStoreAttemptReleased("vault:db")
			payload := `{"value":"replacement"}`
			if strings.HasSuffix(command, " get") {
				payload = ""
			}
			p.call("invalid", command, payload, 400)
			require.JSONEq(t, `{"value":"original"}`, p.call("get", "config go.d:secretstore:vault:db get", "", 200))
			require.NotContains(t, p.output.String(), "job /collectors/go.d/SecretStores dyncfg")
			// A rejected payload must not corrupt the next request; safe Unicode remains valid.
			p.call("valid", "config go.d:secretstore:vault add db-α", `{"value":"valid"}`, 202)
			require.JSONEq(t, `{"value":"valid"}`, p.call("get-valid", "config go.d:secretstore:vault:db-α get", "", 200))
		})
	}
}

func TestSecretInitialAcquisitionDoesNotBlockCommands(t *testing.T) {
	for _, value := range []string{"replacement", "public-failure"} {
		t.Run(value, func(t *testing.T) {
			testSecretInitialAcquisitionDoesNotBlockCommands(t, value)
		})
	}
}

func testSecretInitialAcquisitionDoesNotBlockCommands(t *testing.T, value string) {
	t.Helper()
	gate := newProcessBlockingStoreGate()
	defer gate.release()
	entered := make(chan context.Context, 1)
	creator := secretstore.Creator{
		Kind:   secretstore.KindVault,
		Schema: `{}`,
		Create: func() secretstore.Store {
			return &adoptionTestStore{
				gate:    gate,
				entered: entered,
			}
		},
	}
	p := newSecretAdoptionProcess(t, creator, []secretstore.Config{{
		"name": "main", "kind": "vault", "value": "blocked",
		"__source__": "file=test", "__source_type__": confgroup.TypeUser,
	}, {
		"name": "other", "kind": "vault", "value": "fast",
		"__source__": "file=test", "__source_type__": confgroup.TypeUser,
	}})
	var acquisition context.Context
	select {
	case acquisition = <-entered:
	case <-time.After(time.Second):
		t.Fatal("initial Store acquisition did not start")
	}
	p.output.waitContains(t, "CONFIG go.d:secretstore:vault:other create running job")
	p.call("get-initial", "config go.d:secretstore:vault:main get", "", 200)
	p.call("remove-file", "config go.d:secretstore:vault:main remove", "", 405)
	p.call("invalid-update", "config go.d:secretstore:vault:main update", `{"value":{}}`, 400)
	require.NoError(t, acquisition.Err(), "rejected commands canceled accepted acquisition")
	p.call("replay", "config go.d:secretstore:vault add main", fmt.Sprintf(`{"value":%q}`, value), 202)
	p.output.waitContains(t, "CONFIG go.d:secretstore:vault:main create accepted job")
	// The daemon can replay saved intent from both template ADD and the
	// file registration's UPDATE. Identical pending UPDATE joins the accepted ADD.
	p.call("replay-update", "config go.d:secretstore:vault:main update", fmt.Sprintf(`{"value":%q}`, value), 202)
	gate.release()
	status := "running"
	if value == "public-failure" {
		status = "failed"
	}
	p.output.waitContains(t, "CONFIG go.d:secretstore:vault:main create "+status+" job")
	body := p.call("get-replay", "config go.d:secretstore:vault:main get", "", 200)
	require.JSONEq(t, fmt.Sprintf(`{"value":%q}`, value), body)
	if status == "failed" {
		require.NotContains(t, p.output.String(), "CONFIG go.d:secretstore:vault:main create running job",
			"obsolete file acquisition became fallback after replacement failure")
	} else {
		require.Equal(t, 1, strings.Count(p.output.String(), "CONFIG go.d:secretstore:vault:main create running job"))
	}
}

func TestSecretFailedUpdatePreservesAcceptedConfig(t *testing.T) {
	p := newSecretAdoptionProcess(t, secretstore.Creator{
		Kind:   secretstore.KindVault,
		Schema: `{}`,
		Create: func() secretstore.Store { return &processSecretStore{} },
	}, nil)
	p.call("add-failed", "config go.d:secretstore:vault add main", `{"value":"public-failure"}`, 202)
	p.output.waitContains(t, "CONFIG go.d:secretstore:vault:main create failed job")
	p.waitStoreAttemptReleased("vault:main")
	p.call("retry-same-failed", "config go.d:secretstore:vault:main update", `{"value":"public-failure"}`, 400)
	p.waitStoreAttemptReleased("vault:main")
	p.call("update-failed", "config go.d:secretstore:vault:main update", `{"value":"backend-sensitive-detail"}`, 400)
	require.JSONEq(t, `{"value":"public-failure"}`,
		p.call("get-failed", "config go.d:secretstore:vault:main get", "", 200))
}

func TestSecretAdmissionPreservesRawReferences(t *testing.T) {
	t.Setenv("NETDATA_STEP03_MISSING_TOKEN", "")
	t.Setenv("NETDATA_STEP03_MISSING_BOOL", "")
	p := newSecretAdoptionProcess(t, vaultbackend.New(), nil)
	p.call("bad-type", "config go.d:secretstore:vault add invalid", `{"addr":{}}`, 400)
	p.call("get-invalid", "config go.d:secretstore:vault:invalid get", "", 404)
	const payload = `{"mode":"token","addr":"https://vault.invalid","mode_token":{"token":"${env:NETDATA_STEP03_MISSING_TOKEN}"},"tls_skip_verify":"${env:NETDATA_STEP03_MISSING_BOOL}"}`
	p.call("add-references", "config go.d:secretstore:vault add main", payload, 202)
	require.JSONEq(t, payload, p.call("get-references", "config go.d:secretstore:vault:main get", "", 200))
	require.NotContains(t, p.output.String(), "CONFIG go.d:secretstore:vault:invalid create")
}

func TestSecretAdmissionRejectsStoreReferencesWithoutAdoption(t *testing.T) {
	var acquisitions atomic.Int32
	p := newSecretAdoptionProcess(t, secretstore.Creator{
		Kind:   secretstore.KindVault,
		Schema: `{}`,
		Create: func() secretstore.Store {
			return &adoptionTestStore{
				observe: func() { acquisitions.Add(1) },
			}
		},
	}, nil)
	const payload = `{"value":"candidate","nested":{"token":"${store:vault:other:key}"}}`
	p.call("reject-store-reference", "config go.d:secretstore:vault add invalid", payload, 400)
	p.call("get-rejected-store", "config go.d:secretstore:vault:invalid get", "", 404)
	require.NotContains(t, p.output.String(), "CONFIG go.d:secretstore:vault:invalid create")
	require.Zero(t, acquisitions.Load())
	p.call("add-incumbent", "config go.d:secretstore:vault add main", `{"value":"incumbent"}`, 202)
	p.output.waitContains(t, "CONFIG go.d:secretstore:vault:main create running job")
	p.call("reject-store-update", "config go.d:secretstore:vault:main update", payload, 400)
	require.JSONEq(
		t,
		`{"value":"incumbent"}`,
		p.call("get-incumbent", "config go.d:secretstore:vault:main get", "", 200),
	)
	require.EqualValues(t, 1, acquisitions.Load(), "rejected payload started another acquisition")
}

func TestSecretMalformedInitialConfigRemainsReadable(t *testing.T) {
	p := newSecretAdoptionProcess(t, secretstore.Creator{
		Kind:   secretstore.KindVault,
		Schema: `{}`,
		Create: func() secretstore.Store { return &processSecretStore{} },
	}, []secretstore.Config{{
		"name": "main", "kind": "vault", "value": map[string]any{"bad": "type"},
		"__source__": "file=test", "__source_type__": confgroup.TypeUser,
	}})
	p.output.waitContains(t, "CONFIG go.d:secretstore:vault:main create failed job")
	require.JSONEq(t, `{"value":{"bad":"type"}}`,
		p.call("get-invalid-initial", "config go.d:secretstore:vault:main get", "", 200))
	p.call("repair-initial", "config go.d:secretstore:vault:main update", `{"value":"repaired"}`, 200)
	p.output.waitContains(t, "CONFIG go.d:secretstore:vault:main create running job")
}

func TestSecretSourceConversionUsesExactNamedResource(t *testing.T) {
	for _, name := range []string{"main", "vault"} {
		t.Run(name, func(t *testing.T) {
			p := newSecretAdoptionProcess(t, secretstore.Creator{
				Kind:   secretstore.KindVault,
				Schema: `{}`,
				Create: func() secretstore.Store { return &processSecretStore{} },
			}, []secretstore.Config{{
				"name": name, "kind": "vault", "value": "initial",
				"__source__": "file=test", "__source_type__": confgroup.TypeUser,
			}})
			id := "go.d:secretstore:vault:" + name
			p.output.waitContains(t, "CONFIG "+id+" create running job")
			p.waitStoreAttemptReleased("vault:" + name)
			p.call("convert", "config "+id+" update", `{"value":"initial"}`, 200)
			p.output.waitContains(t, "CONFIG "+id+" create running job /collectors/go.d/SecretStores dyncfg")
			p.call("remove-converted", "config "+id+" remove", "", 200)
			p.call("get-removed", "config "+id+" get", "", 404)
		})
	}
}

func TestSecretAcceptedAddPublishesBeforeAcquisition(t *testing.T) {
	gate := newProcessBlockingStoreGate()
	defer gate.release()
	entered := make(chan context.Context, 1)
	observed := make(chan string, 1)
	var p *secretAdoptionProcess
	p = newSecretAdoptionProcess(t, secretstore.Creator{
		Kind:   secretstore.KindVault,
		Schema: `{}`,
		Create: func() secretstore.Store {
			return &adoptionTestStore{
				gate:    gate,
				entered: entered,
				observe: func() { observed <- p.output.String() },
			}
		},
	}, nil)
	p.call("add-blocked", "config go.d:secretstore:vault add main", `{"value":"blocked","options":{"number":1}}`, 202)
	var atStart string
	select {
	case atStart = <-observed:
	case <-time.After(time.Second):
		t.Fatal("accepted Store acquisition did not start")
	}
	result := strings.Index(atStart, "FUNCTION_RESULT_BEGIN add-blocked 202")
	accepted := strings.Index(atStart, "CONFIG go.d:secretstore:vault:main create accepted job")
	require.NotEqual(t, -1, result)
	require.Greater(t, accepted, result)
	require.JSONEq(
		t,
		`{"value":"blocked","options":{"number":1}}`,
		p.call("get-blocked", "config go.d:secretstore:vault:main get", "", 200),
	)
	p.call(
		"duplicate-pending",
		"config go.d:secretstore:vault:main update",
		`{"value":"blocked","options":{"number":1}}`,
		202,
	)
	select {
	case <-observed:
		t.Fatal("identical pending update acquired another provider")
	default:
	}
	p.call("remove-blocked", "config go.d:secretstore:vault:main remove", "", 200)
	gate.release()
	p.call("get-removed", "config go.d:secretstore:vault:main get", "", 404)
	require.NotContains(t, p.output.String(), "CONFIG go.d:secretstore:vault:main create running job")
}

type adoptionTestStore struct {
	processSecretStore
	gate    *processBlockingStoreGate
	entered chan<- context.Context
	observe func()
}

func (s *adoptionTestStore) Init(ctx context.Context) error {
	if s.observe != nil {
		s.observe()
	}
	if s.config.Value == "blocked" {
		s.entered <- ctx
		<-s.gate.releaseC
	}
	return s.processSecretStore.Init(ctx)
}

type secretAdoptionProcess struct {
	t       *testing.T
	writer  *io.PipeWriter
	output  *processSynchronizedBuffer
	process *processCore
}

func newSecretAdoptionProcess(
	t *testing.T,
	creator secretstore.Creator,
	initial []secretstore.Config,
) *secretAdoptionProcess {
	t.Helper()
	catalog, err := secretstore.NewCreatorCatalog([]secretstore.Creator{creator})
	require.NoError(t, err)
	jobs := testRunJobServices(t)
	secretConfig := testRunSecrets(t)
	secretConfig.Providers.Resolver, err = secretresolver.NewDefaultAtomicResolver()
	require.NoError(t, err)
	secretConfig.Providers.Creators = catalog
	reader, writer := io.Pipe()
	output := newProcessSynchronizedBuffer()
	process, err := newProcessCore(processCoreConfig{
		Input:           reader,
		Output:          output,
		ShutdownTimeout: time.Second,
		Modules:         collectorapi.Registry{},
		Jobs:            jobs,
		Secrets: &SecretsConfig{
			Providers: secretConfig.Providers,
			Initial:   initial,
		},
		Discovery:   testRunDiscoveryServices(t),
		Diagnostics: testProcessDiagnostics(),
	})
	require.NoError(t, err)
	controls := newTestProcessControls(1)
	done := make(chan error, 1)
	go func() { done <- process.run(context.Background(), controls) }()
	t.Cleanup(func() {
		defer func() { require.NoError(t, writer.Close()) }()
		controls.sendTerminate(testProcessControl())
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(3 * time.Second):
			t.Error("secret adoption process did not terminate")
		}
	})
	output.waitContains(t, "CONFIG go.d:secretstore:vault create accepted template")
	return &secretAdoptionProcess{
		t:       t,
		writer:  writer,
		output:  output,
		process: process,
	}
}

// Wire results precede physical release. Wait before testing another preflight
// so contention cannot mask its validation result. This fixture never rotates.
func (p *secretAdoptionProcess) waitStoreAttemptReleased(key string) {
	p.t.Helper()
	identity := jobmgr.ProcessAttemptIdentity{
		Namespace: jobmgr.ProcessAttemptStore,
		Key:       jobmgr.ProcessAttemptIdentityKey("secret-store", "1", key),
		Resource:  jobmgr.ProcessAttemptDiagnosticResource(key, "secret Store"),
	}
	require.True(p.t, identity.Valid())
	if released, exists := p.process.attempts.ProcessAttemptReleased(identity); exists {
		select {
		case <-released:
		case <-time.After(time.Second):
			p.t.Fatal("completed Store attempt did not physically release")
		}
	}
	require.Zero(p.t, p.process.attempts.Census().Quarantined)
}

func (p *secretAdoptionProcess) call(uid, command, payload string, status int) string {
	p.t.Helper()
	return callProcessFunction(p.t, p.writer, p.output, uid, command, payload, status)
}
func callProcessFunction(t *testing.T, writer io.Writer, output *processSynchronizedBuffer, uid, command, payload string, status int) string {
	t.Helper()
	line := fmt.Sprintf("FUNCTION %s 10 \"%s\" 0xFFFF \"user=test\"\n", uid, command)
	if payload != "" {
		line = fmt.Sprintf(
			"FUNCTION_PAYLOAD %s 10 \"%s\" 0xFFFF \"user=test\" application/json\n%s\nFUNCTION_PAYLOAD_END\n",
			uid,
			command,
			payload,
		)
	}
	written := make(chan error, 1)
	go func() {
		_, err := io.WriteString(writer, line)
		written <- err
	}()
	select {
	case err := <-written:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("command ingress blocked")
	}
	begin := fmt.Sprintf("FUNCTION_RESULT_BEGIN %s %d application/json", uid, status)
	output.waitContains(t, begin)
	result := output.String()
	result = result[strings.Index(result, begin):]
	result = result[strings.IndexByte(result, '\n')+1:]
	result = strings.TrimSpace(result[:strings.Index(result, "FUNCTION_RESULT_END")])
	require.True(t, json.Valid([]byte(result)))
	return result
}
