// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	vaultbackend "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore/backends/vault"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/require"
)

func TestSecretInitialAcquisitionDoesNotBlockCommands(t *testing.T) {
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
	p.call("replay", "config go.d:secretstore:vault add main", `{"value":"public-failure"}`, 202)
	p.output.waitContains(t, "CONFIG go.d:secretstore:vault:main create accepted job")
	// The daemon can replay saved intent from both template ADD and the
	// file registration's UPDATE. Busy UPDATE must not revoke the accepted ADD.
	p.call("replay-update", "config go.d:secretstore:vault:main update", `{"value":"public-failure"}`, 503)
	gate.release()
	p.output.waitContains(t, "CONFIG go.d:secretstore:vault:main create failed job")
	body := p.call("get-replay", "config go.d:secretstore:vault:main get", "", 200)
	require.JSONEq(t, `{"value":"public-failure"}`, body)
	require.NotContains(t, p.output.String(), "CONFIG go.d:secretstore:vault:main create running job",
		"obsolete file acquisition became fallback after replacement failure")
}

func TestSecretFailedUpdatePreservesAcceptedConfig(t *testing.T) {
	p := newSecretAdoptionProcess(t, secretstore.Creator{
		Kind:   secretstore.KindVault,
		Schema: `{}`,
		Create: func() secretstore.Store { return &processSecretStore{} },
	}, nil)
	p.call("add-failed", "config go.d:secretstore:vault add main", `{"value":"public-failure"}`, 202)
	p.output.waitContains(t, "CONFIG go.d:secretstore:vault:main create failed job")
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
	p.call("add-blocked", "config go.d:secretstore:vault add main", `{"value":"blocked"}`, 202)
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
	require.JSONEq(t, `{"value":"blocked"}`, p.call("get-blocked", "config go.d:secretstore:vault:main get", "", 200))
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
	t      *testing.T
	writer *io.PipeWriter
	output *processSynchronizedBuffer
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
	jobs.Resolver, err = secretresolver.NewDefaultAtomicResolver()
	require.NoError(t, err)
	jobs.StoreCreators = catalog
	reader, writer := io.Pipe()
	output := newProcessSynchronizedBuffer()
	process, err := newProcessCore(processCoreConfig{
		Input:           reader,
		Output:          output,
		ShutdownTimeout: time.Second,
		Modules:         collectorapi.Registry{},
		Jobs:            jobs,
		Secrets: runSecretServices{
			Initial: initial,
		},
		Discovery:   testRunDiscoveryServices(t),
		Diagnostics: testProcessDiagnostics(),
	})
	require.NoError(t, err)
	controls := newTestProcessControls(1)
	done := make(chan error, 1)
	go func() { done <- process.run(context.Background(), controls) }()
	t.Cleanup(func() {
		require.NoError(t, writer.Close())
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
		t:      t,
		writer: writer,
		output: output,
	}
}

func (p *secretAdoptionProcess) call(uid, command, payload string, status int) string {
	p.t.Helper()
	line := fmt.Sprintf("FUNCTION %s 10 %q 0xFFFF \"user=test\"\n", uid, command)
	if payload != "" {
		line = fmt.Sprintf(
			"FUNCTION_PAYLOAD %s 10 %q 0xFFFF \"user=test\" application/json\n%s\nFUNCTION_PAYLOAD_END\n",
			uid,
			command,
			payload,
		)
	}
	written := make(chan error, 1)
	go func() {
		_, err := io.WriteString(p.writer, line)
		written <- err
	}()
	select {
	case err := <-written:
		require.NoError(p.t, err)
	case <-time.After(time.Second):
		p.t.Fatal("Store acquisition blocked command ingress")
	}
	begin := fmt.Sprintf("FUNCTION_RESULT_BEGIN %s %d application/json", uid, status)
	p.output.waitContains(p.t, begin)
	result := p.output.String()
	result = result[strings.Index(result, begin):]
	result = result[strings.IndexByte(result, '\n')+1:]
	result = strings.TrimSpace(result[:strings.Index(result, "FUNCTION_RESULT_END")])
	require.True(p.t, json.Valid([]byte(result)))
	return result
}
