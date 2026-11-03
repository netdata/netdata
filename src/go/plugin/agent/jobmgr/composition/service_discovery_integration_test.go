// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	agentdiscovery "github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/model"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/require"
)

// This drives the shipped shared Handler, SD actor, composition bridge and kernel.
// Only the discoverer's construction/execution is controlled by the test.
func TestServiceDiscoveryPreflightDoesNotBlockOtherCommands(t *testing.T) {
	for _, command := range []string{"update", "test"} {
		t.Run(command, func(t *testing.T) {
			entered, release, returned := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var starts atomic.Int32
			registry := sd.NewRegistry(sd.NewDescriptor("fixture", "{}",
				func(raw json.RawMessage) (sdIntegrationConfig, error) {
					var cfg sdIntegrationConfig
					err := json.Unmarshal(raw, &cfg)
					return cfg, err
				},
				func(cfg sdIntegrationConfig, _ string) ([]model.Discoverer, error) {
					if cfg.Mode == "blocked" {
						close(entered)
						<-release
						defer close(returned)
					}
					return []model.Discoverer{sdIntegrationDiscoverer{
						starts: &starts,
					}}, nil
				},
			))
			generation, output := newSDIntegrationGeneration(t, registry)
			released := false
			t.Cleanup(func() {
				if !released {
					close(release)
				}
			})
			submit := func(uid string, args []string, payload string) {
				t.Helper()
				timeout := time.Second
				if uid == "blocked" {
					timeout = 10 * time.Second
				}
				require.NoError(t, generation.kernel.Submit(t.Context(), jobmgr.Request{
					UID:    uid,
					Route:  "config",
					Source: lifecycle.SourceFunction,
					Args:   args,
					Payload: []byte(
						payload,
					),
					HasPayload:   payload != "",
					ContentType:  "application/json",
					CallerSource: "user=test",
					Deadline:     time.Now().Add(timeout),
				}))
			}
			result := func(uid string, code int) {
				t.Helper()
				output.waitContains(t, "FUNCTION_RESULT_BEGIN "+uid+" ")
				require.Contains(
					t,
					output.String(),
					fmt.Sprintf("FUNCTION_RESULT_BEGIN %s %d application/json", uid, code),
				)
			}
			for _, name := range []string{"a", "b"} {
				submit("add-"+name, []string{"go.d:sd:fixture", "add", name}, sdIntegrationPayload("initial"))
				result("add-"+name, 202)
				submit("enable-"+name, []string{"go.d:sd:fixture:" + name, "enable"}, "")
				result("enable-"+name, 202)
				output.waitContains(t, "CONFIG go.d:sd:fixture:"+name+" status running")
			}
			require.Eventually(t, func() bool { return starts.Load() == 2 }, time.Second, time.Millisecond)
			wire := output.String()
			reply := strings.Index(wire, "FUNCTION_RESULT_BEGIN enable-a 202")
			accepted := strings.Index(wire[reply:], "CONFIG go.d:sd:fixture:a status accepted") + reply
			running := strings.Index(wire, "CONFIG go.d:sd:fixture:a status running")
			require.True(t, reply >= 0 && accepted > reply && running > accepted, "wire=%q", wire)

			submit("blocked", []string{"go.d:sd:fixture:a", command}, sdIntegrationPayload("blocked"))
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("preflight not entered")
			}
			for _, read := range []string{"get", "schema"} {
				submit(read+"-b", []string{"go.d:sd:fixture:b", read}, "")
				result(read+"-b", 200)
			}
			submit("disable-b", []string{"go.d:sd:fixture:b", "disable"}, "")
			result("disable-b", 200)
			select {
			case <-returned:
				t.Fatal("blocked preflight unexpectedly returned")
			default:
			}
			// TEST shares GET's read identity. Capture its release before cancellation;
			// constructor return alone does not establish that the outer handler exited.
			var testReleased <-chan struct{}
			if command == "test" {
				var occupied bool
				testReleased, occupied = generation.discovery.BuildContext.Attempts.ProcessAttemptReleased(jobmgr.ProcessAttemptIdentity{
					Namespace: jobmgr.ProcessAttemptServiceDiscovery,
					Key:       jobmgr.ProcessAttemptIdentityKey("service-discovery-read", "go.d", "sd:fixture_a"),
					Resource:  "sd:fixture_a",
				})
				require.True(t, occupied, "blocked TEST must own the read identity before cancellation")
			}
			require.NoError(t, generation.kernel.Cancel(t.Context(), "blocked"))
			result("blocked", 499)
			// The abandoned physical UPDATE/TEST cannot occupy DISABLE's identity.
			submit("disable-a", []string{"go.d:sd:fixture:a", "disable"}, "")
			result("disable-a", 200)
			close(release)
			released = true
			select {
			case <-returned:
			case <-time.After(time.Second):
				t.Fatal("preflight did not return")
			}
			if testReleased != nil {
				select {
				case <-testReleased:
				case <-time.After(time.Second):
					t.Fatal("outer TEST attempt did not release")
				}
			}
			submit("get-a", []string{"go.d:sd:fixture:a", "get"}, "")
			result("get-a", 200)
			_, body, ok := strings.Cut(output.String(), "FUNCTION_RESULT_BEGIN get-a 200 application/json")
			require.True(t, ok)
			require.Contains(t, body, `"mode":"initial"`)
			require.NotContains(t, body, `"mode":"blocked"`)
			require.Never(t, func() bool { return starts.Load() != 2 }, 100*time.Millisecond, time.Millisecond)
			require.NoError(t, generation.run.DirtyCause())
		})
	}
}

type sdIntegrationConfig struct {
	Mode string `json:"mode"`
}
type sdIntegrationDiscoverer struct{ starts *atomic.Int32 }

func (d sdIntegrationDiscoverer) Discover(ctx context.Context, _ chan<- []model.TargetGroup) {
	d.starts.Add(1)
	<-ctx.Done()
}
func sdIntegrationPayload(mode string) string {
	return `{"discoverer":{"fixture":{"mode":"` + mode + `"}},"services":[{"id":"fixture","match":"true"}]}`
}

func newSDIntegrationGeneration(t *testing.T, registry sd.Registry) (*runGeneration, *processSynchronizedBuffer) {
	t.Helper()
	services := sdIntegrationServices(t, registry)
	output := newProcessSynchronizedBuffer()
	frames, err := lifecycle.NewFrameOwner(output)
	require.NoError(t, err)
	uids := lifecycle.NewUIDLedger()
	generation, err := newTestRunGeneration(t, runGenerationConfig{
		Secrets:         testRunSecrets(t),
		Generation:      1,
		ShutdownTimeout: time.Second,
		UIDs:            uids,
		Frames:          frames,
		Modules:         collectorapi.Registry{},
		Jobs:            testRunJobServices(t),
		Discovery:       services,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		generation.Stop()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		require.NoError(t, generation.Wait(ctx))
		closeRunTestUIDs(t, uids)
	})
	require.NoError(t, generation.start(context.Background()))
	output.waitContains(t, "CONFIG go.d:sd:fixture create accepted template")
	return generation, output
}

func sdIntegrationServices(t *testing.T, registry sd.Registry) runDiscoveryServices {
	t.Helper()
	directory := t.TempDir()
	factory := agentdiscovery.NewProviderFactory(
		"sd",
		func(build agentdiscovery.BuildContext) (agentdiscovery.Discoverer, bool, error) {
			d, err := sd.NewServiceDiscovery(sd.Config{
				Epoch:          build.Epoch,
				Attempts:       build.Attempts,
				ConfigDefaults: build.Registry,
				PluginName:     build.Identity.Name,
				RunModePolicy:  build.RunMode,
				DyncfgOutput:   build.DyncfgOutput,
				ConfDir:        multipath.MultiPath{directory},
				FnReg:          build.FnReg,
				Discoverers:    registry,
			})
			return d, true, err
		},
	)
	providers, err := agentdiscovery.NewProviderCatalog([]agentdiscovery.ProviderFactory{factory})
	require.NoError(t, err)

	return runDiscoveryServices{
		BuildContext: agentdiscovery.BuildContext{
			Identity: agentdiscovery.PluginIdentity{
				Name: "go.d",
			},
			Registry: confgroup.Registry{
				"fixture": {},
			},
			Paths: agentdiscovery.PathsConfig{
				ServiceDiscoveryConfigDir: multipath.MultiPath{directory},
			},
		},
		Providers: providers,
	}
}

func TestServiceDiscoveryPublicationFailureDoesNotActivate(t *testing.T) {
	var starts, constructors atomic.Int32
	registry := sd.NewRegistry(sd.NewDescriptor("fixture", "{}",
		func(raw json.RawMessage) (sdIntegrationConfig, error) {
			var cfg sdIntegrationConfig
			err := json.Unmarshal(raw, &cfg)
			return cfg, err
		},
		func(sdIntegrationConfig, string) ([]model.Discoverer, error) {
			constructors.Add(1)
			return []model.Discoverer{sdIntegrationDiscoverer{
				starts: &starts,
			}}, nil
		},
	))
	reader, writer := io.Pipe()
	output := newProcessSynchronizedBuffer()
	process, err := newProcessCore(processCoreConfig{
		Secrets:         testRunSecrets(t),
		Input:           reader,
		Output:          sdAcceptedFailureWriter{output},
		ShutdownTimeout: time.Second,
		Modules:         collectorapi.Registry{},
		Jobs:            testRunJobServices(t),
		Discovery:       sdIntegrationServices(t, registry),
		Diagnostics:     testProcessDiagnostics(),
	})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- process.run(ctx, newTestProcessControls(1)) }()
	t.Cleanup(func() { cancel(); _ = writer.Close(); _ = reader.Close() })
	output.waitContains(t, "CONFIG go.d:sd:fixture create accepted template")
	payload := sdIntegrationPayload("initial")
	_, err = fmt.Fprintf(
		writer,
		"FUNCTION_PAYLOAD add 30 \"config go.d:sd:fixture add a\" 0xFFFF \"user=test\" application/json\n%s\nFUNCTION_PAYLOAD_END\n",
		payload,
	)
	require.NoError(t, err)
	output.waitContains(t, "FUNCTION_RESULT_BEGIN add 202 application/json")
	_, err = io.WriteString(writer, "FUNCTION enable 30 \"config go.d:sd:fixture:a enable\" 0xFFFF \"user=test\"\n")
	require.NoError(t, err)
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("publication failure did not close the process generation")
	}
	require.Contains(t, output.String(), "FUNCTION_RESULT_BEGIN enable 202 application/json")
	require.NotContains(t, output.String(), "CONFIG go.d:sd:fixture:a status running")
	require.Zero(t, constructors.Load())
	require.Zero(t, starts.Load())
}

type sdAcceptedFailureWriter struct{ output *processSynchronizedBuffer }

func (w sdAcceptedFailureWriter) Write(data []byte) (int, error) {
	if bytes.Contains(data, []byte("CONFIG go.d:sd:fixture:a status accepted")) {
		return 0, errors.New("test acceptance publication failure")
	}
	return w.output.Write(data)
}
