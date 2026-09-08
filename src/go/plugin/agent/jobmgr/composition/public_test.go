// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentdiscovery "github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/stretchr/testify/require"
)

func TestProductionProcessRejectsInvalidInitialVnodes(t *testing.T) {
	tests := map[string]struct {
		vnodes map[string]*vnodes.VirtualNode
	}{
		"nil vnode": {vnodes: map[string]*vnodes.VirtualNode{"missing": nil}},
		"source type with separator": {
			vnodes: map[string]*vnodes.VirtualNode{
				"node": {
					Name:       "node",
					Hostname:   "node",
					GUID:       "11111111-1111-1111-1111-111111111111",
					SourceType: "user type",
					Source:     "file=/etc/netdata/vnodes.conf",
				},
			},
		},
		"hostname unsafe for host emission": {
			vnodes: map[string]*vnodes.VirtualNode{
				"node": {
					Name:       "node",
					Hostname:   "operator's-node",
					GUID:       "11111111-1111-1111-1111-111111111111",
					SourceType: confgroup.TypeUser,
					Source:     "file=/etc/netdata/vnodes.conf",
				},
			},
		},
		"unsupported GUID spelling": {
			vnodes: map[string]*vnodes.VirtualNode{
				"node": {
					Name:       "node",
					Hostname:   "node",
					GUID:       "urn:uuid:11111111-1111-1111-1111-111111111111",
					SourceType: confgroup.TypeUser,
					Source:     "file=/etc/netdata/vnodes.conf",
				},
			},
		},
		"semantically duplicate GUID": {
			vnodes: map[string]*vnodes.VirtualNode{
				"first": {
					Name:       "first",
					Hostname:   "first",
					GUID:       "11111111-1111-1111-1111-111111111111",
					SourceType: confgroup.TypeUser,
					Source:     "file=/etc/netdata/vnodes.conf",
				},
				"second": {
					Name:       "second",
					Hostname:   "second",
					GUID:       "11111111111111111111111111111111",
					SourceType: confgroup.TypeUser,
					Source:     "file=/etc/netdata/vnodes.conf",
				},
			},
		},
		"host label with trailing escape": {
			vnodes: map[string]*vnodes.VirtualNode{
				"node": {
					Name:       "node",
					Hostname:   "node",
					GUID:       "11111111-1111-1111-1111-111111111111",
					Labels:     map[string]string{"site": `value\`},
					SourceType: confgroup.TypeUser,
					Source:     "file=/etc/netdata/vnodes.conf",
				},
			},
		},
		"hostnames collide after host emission preparation": {
			vnodes: map[string]*vnodes.VirtualNode{
				"first": {
					Name:       "first",
					Hostname:   "host",
					GUID:       "11111111-1111-1111-1111-111111111111",
					SourceType: confgroup.TypeUser,
					Source:     "file=/etc/netdata/vnodes.conf",
				},
				"second": {
					Name:       "second",
					Hostname:   " host ",
					GUID:       "22222222-2222-2222-2222-222222222222",
					SourceType: confgroup.TypeUser,
					Source:     "file=/etc/netdata/vnodes.conf",
				},
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			config := testProductionProcessConfig(strings.NewReader(""), io.Discard)
			config.InitialVnodes = test.vnodes

			_, err := NewProcess(config)
			require.Error(t, err)
		})
	}
}

func TestProductionProcessAcceptsIndividuallyValidatedVNodeLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vnodes.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
- hostname: valid
  guid: 11111111-1111-1111-1111-111111111111
- hostname: bad=name
  guid: 22222222-2222-2222-2222-222222222222
`), 0o644))

	initial := vnodes.Load(dir)
	require.Len(t, initial, 1)
	config := testProductionProcessConfig(strings.NewReader(""), io.Discard)
	config.InitialVnodes = initial

	_, err := NewProcess(config)
	require.NoError(t, err)
}

func TestProductionProcessAcceptsCompactInitialVNodeGUID(t *testing.T) {
	config := testProductionProcessConfig(strings.NewReader(""), io.Discard)
	config.InitialVnodes = map[string]*vnodes.VirtualNode{
		"node": {
			Name:       "node",
			Hostname:   "node",
			GUID:       "11111111111111111111111111111111",
			SourceType: confgroup.TypeUser,
			Source:     "file=/etc/netdata/vnodes.conf",
		},
	}

	_, err := NewProcess(config)
	require.NoError(t, err)
}

func TestProductionProcessAcceptsWindowsInitialVNodeSource(t *testing.T) {
	config := testProductionProcessConfig(strings.NewReader(""), io.Discard)
	config.InitialVnodes = map[string]*vnodes.VirtualNode{
		"node": {
			Name:       "node",
			Hostname:   "node",
			GUID:       "11111111-1111-1111-1111-111111111111",
			SourceType: confgroup.TypeUser,
			Source:     `file=C:\Program Files\Netdata\vnodes.conf`,
		},
	}

	_, err := NewProcess(config)
	require.NoError(t, err)
}

func TestProductionProcessIsSingleUse(t *testing.T) {
	reader, writer := io.Pipe()
	var output bytes.Buffer
	process, err := NewProcess(testProductionProcessConfig(reader, &output))
	require.NoError(t, err)
	runDone := make(chan error, 1)
	go func() {
		runDone <- process.Run(context.Background())
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	if err := process.Terminate(ctx); err != nil {
		cancel()
		require.FailNow(t, "test failed", err)
	}
	cancel()

	require.NoError(t, writer.Close())

	require.NoError(t, <-runDone)

	require.Error(t, process.Run(context.Background()))
}

func TestProductionProcessQuitHasOneCleanTerminalDisposition(t *testing.T) {
	for range 32 {
		process, err := NewProcess(testProductionProcessConfig(strings.NewReader("QUIT\n"), io.Discard))
		require.NoError(t, err)

		require.NoError(t, process.Run(context.Background()))
	}
}

func TestProcessControlCancellationAfterHandoffWaitsForDisposition(t *testing.T) {
	tests := map[string]struct {
		call     func(*Process, context.Context) error
		controls func(processControls) <-chan processControl
	}{
		"restart": {
			call:     (*Process).Restart,
			controls: func(controls processControls) <-chan processControl { return controls.restart },
		},
		"terminate": {
			call:     (*Process).Terminate,
			controls: func(controls processControls) <-chan processControl { return controls.terminate },
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			started := make(chan struct{})
			close(started)
			process := &Process{
				controls: newProcessControls(),
				started:  started,
				done:     make(chan struct{}),
			}
			accepted := make(chan struct{})
			release := make(chan struct{})
			go func() {
				control := <-test.controls(process.controls)
				close(accepted)
				<-release
				control.result <- nil
			}()
			ctx, cancel := context.WithCancel(context.Background())
			result := make(chan error, 1)
			go func() {
				result <- test.call(process, ctx)
			}()
			select {
			case <-accepted:
			case <-time.After(time.Second):
				require.FailNow(t, "test failed", "process control was not accepted")
			}
			cancel()

			var early error
			returnedEarly := false
			select {
			case early = <-result:
				returnedEarly = true
			case <-time.After(100 * time.Millisecond):
			}
			close(release)
			if !returnedEarly {
				select {
				case early = <-result:
				case <-time.After(time.Second):
					require.FailNow(t, "test failed", "accepted process control did not complete")
				}
			}
			require.False(t, returnedEarly)
			require.Nil(t, early)
		})
	}
}

func testProductionProcessConfig(input io.Reader, output io.Writer) Config {
	factory := agentdiscovery.NewProviderFactory(
		"test",
		func(agentdiscovery.BuildContext) (agentdiscovery.Discoverer, bool, error) {
			return runTestDiscoverer{}, true, nil
		},
	)
	return Config{
		Input:      input,
		Output:     output,
		PluginName: "go.d",
		Modules: collectorapi.Registry{
			"test": {},
		},
		Defaults: confgroup.Registry{
			"test": {},
		},
		DiscoveryProviders: []agentdiscovery.ProviderFactory{factory},
		ShutdownTimeout:    time.Second,
	}
}

func TestProcessServiceSpansRestartsAndJoinsOnTermination(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	service := &testProcessService{started: make(chan struct{}), stopped: make(chan struct{}), release: make(chan struct{})}
	config := testProductionProcessConfig(reader, io.Discard)
	config.Services = []ProcessService{service}
	process, err := NewProcess(config)
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() { done <- process.Run(t.Context()) }()
	select {
	case <-service.started:
	case <-time.After(time.Second):
		t.Fatal("service did not start")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	require.NoError(t, process.Restart(ctx))
	select {
	case <-service.stopped:
		t.Fatal("service stopped on restart")
	default:
	}
	terminated := make(chan error, 1)
	go func() { terminated <- process.Terminate(ctx) }()
	select {
	case <-service.stopped:
	case <-time.After(time.Second):
		t.Fatal("service was not canceled")
	}
	select {
	case <-terminated:
		t.Fatal("termination returned before service joined")
	default:
	}
	close(service.release)
	require.NoError(t, <-terminated)
	require.NoError(t, <-done)
}

type testProcessService struct{ started, stopped, release chan struct{} }

func (s *testProcessService) Run(ctx context.Context) {
	close(s.started)
	<-ctx.Done()
	close(s.stopped)
	<-s.release
}

func TestProcessServicesJoinOnInputShutdownAndIsolatePanic(t *testing.T) {
	for _, input := range []string{"", "QUIT\n"} {
		t.Run(input, func(t *testing.T) {
			joined := make(chan struct{})
			panicked := make(chan struct{})
			config := testProductionProcessConfig(strings.NewReader(input), io.Discard)
			config.Services = []ProcessService{
				processServiceFunc(func(ctx context.Context) { <-ctx.Done(); close(joined) }),
				processServiceFunc(func(context.Context) { defer close(panicked); panic("injected service failure") }),
			}
			process, err := NewProcess(config)
			require.NoError(t, err)
			err = process.Run(t.Context())
			if input == "" {
				require.ErrorContains(t, err, "Function input stopped")
			} else {
				require.NoError(t, err)
			}
			select {
			case <-joined:
			default:
				t.Fatal("service outlived Process.Run")
			}
			select {
			case <-panicked:
			default:
				t.Fatal("panic service was not joined")
			}
		})
	}
}

type processServiceFunc func(context.Context)

func (f processServiceFunc) Run(ctx context.Context) { f(ctx) }

func TestProcessServiceShutdownTimeoutIsReportedWithoutBlockingTeardown(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	started, canceled, release, exited := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	config := testProductionProcessConfig(reader, io.Discard)
	config.ShutdownTimeout = 50 * time.Millisecond
	config.Services = []ProcessService{processServiceFunc(func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		close(canceled)
		<-release
		close(exited)
	})}
	process, err := NewProcess(config)
	require.NoError(t, err)
	runDone := make(chan error, 1)
	go func() { runDone <- process.Run(t.Context()) }()
	<-started
	terminated := make(chan error, 1)
	go func() { terminated <- process.Terminate(t.Context()) }()
	<-canceled
	var stopErr error
	timedOut := false
	select {
	case stopErr = <-terminated:
	case <-time.After(time.Second):
		timedOut = true
		t.Error("service join bypassed the shutdown budget")
	}
	var runErr error
	runTimedOut := false
	select {
	case runErr = <-runDone:
	case <-time.After(time.Second):
		runTimedOut = true
		t.Error("Process.Run re-entered service wait after termination")
	}
	close(release)
	<-exited
	if timedOut {
		stopErr = <-terminated
	}
	if runTimedOut {
		runErr = <-runDone
	}
	require.ErrorIs(t, stopErr, context.DeadlineExceeded)
	require.ErrorIs(t, runErr, context.DeadlineExceeded)
	select {
	case <-process.done:
	default:
		t.Fatal("process completion was not signaled")
	}
}
