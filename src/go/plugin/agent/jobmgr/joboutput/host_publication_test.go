// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/hostoutput"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testHostDefinition(t testing.TB, hostname string) *hostoutput.Definition {
	t.Helper()
	d, err := hostoutput.NewDefinition(
		netdataapi.HostInfo{
			GUID:     "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
			Hostname: hostname,
			Labels:   map[string]string{"site": hostname},
		},
	)
	require.NoError(t, err)
	return d
}

func TestHostPublicationUsesAdmittedConfiguration(t *testing.T) {
	var out bytes.Buffer
	frames, err := lifecycle.NewFrameOwner(&out)
	require.NoError(t, err)
	gate, err := newGenerationOutputGate(frames)
	require.NoError(t, err)
	require.NoError(t, gate.Activate())
	p := hostoutput.New()
	fallback := testHostDefinition(t, "generated")
	owner := p.NewOwner(fallback.Info().GUID)
	defer owner.Release()
	request := hostoutput.Request{
		Owner:      owner,
		Definition: fallback,
		Payload:    []byte("HOST 'aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee'\n"),
	}
	prepared := hostoutput.Prepare(request, nil)
	// Preparation happens first; authority changes before the real gate admits the frame.
	configured := testHostDefinition(t, "configured")
	p.Bind(func(string) *hostoutput.Definition { return configured })
	require.NoError(t, gate.CommitBuiltJobOutput(prepared.Build, prepared))
	assert.Contains(t, out.String(), "HOST_LABEL 'site' 'configured'")
	assert.NotContains(t, out.String(), "HOST_LABEL 'site' 'generated'")
	out.Reset()
	p.Bind(nil)
	prepared = hostoutput.Prepare(request, nil)
	require.NoError(t, gate.CommitBuiltJobOutput(prepared.Build, prepared))
	assert.Contains(t, out.String(), "HOST_LABEL 'site' 'generated'")
}

func TestHostPublicationFencingAndFailures(t *testing.T) {
	for name, tc := range map[string]struct{ inactive, fenced, short, commitFailure bool }{
		"inactive": {inactive: true}, "fenced": {fenced: true}, "short write": {short: true}, "commit failure": {commitFailure: true},
	} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			var writer io.Writer = &out
			if tc.short {
				writer = hostTestWriter(func(payload []byte) (int, error) { return len(payload) - 1, nil })
			}
			frames, err := lifecycle.NewFrameOwner(writer)
			require.NoError(t, err)
			gate, err := newGenerationOutputGate(frames)
			require.NoError(t, err)
			if !tc.inactive {
				require.NoError(t, gate.Activate())
			}
			if tc.fenced {
				gate.Fence()
			}
			p := hostoutput.New()
			d := testHostDefinition(t, "node")
			owner := p.NewOwner(d.Info().GUID)
			defer owner.Release()
			state := &hostTestState{
				fail: tc.commitFailure,
			}
			tx := hostoutput.Prepare(hostoutput.Request{
				Owner:      owner,
				Definition: d,
				Payload:    []byte("samples"),
			}, state)
			built := false
			err = gate.CommitBuiltJobOutput(func() ([]byte, error) { built = true; return tx.Build() }, tx)
			require.Error(t, err)
			assert.Equal(t, !tc.inactive && !tc.fenced, built)
			assert.Equal(t, 1, state.aborts)
			assert.Zero(t, p.Len())
			assert.Equal(t, tc.short || tc.commitFailure, frames.Census().Poisoned)
		})
	}
}

func TestHostPublicationDrainsBeforeGenerationFence(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	frames, err := lifecycle.NewFrameOwner(
		hostTestWriter(func(payload []byte) (int, error) { close(entered); <-release; return len(payload), nil }),
	)
	require.NoError(t, err)
	gate, err := newGenerationOutputGate(frames)
	require.NoError(t, err)
	require.NoError(t, gate.Activate())
	p := hostoutput.New()
	d := testHostDefinition(t, "node")
	owner := p.NewOwner(d.Info().GUID)
	defer owner.Release()
	tx := hostoutput.Prepare(hostoutput.Request{
		Owner:      owner,
		Definition: d,
		Payload:    []byte("samples"),
	}, nil)
	result := make(chan error, 1)
	go func() { result <- gate.CommitBuiltJobOutput(tx.Build, tx) }()
	<-entered
	gate.RevokeAdmissions()
	go func() { gate.Fence(); close(finished) }()
	select {
	case <-finished:
		t.Fatal("fence returned during admitted write")
	default:
	}
	close(release)
	require.NoError(t, <-result)
	<-finished
	assert.Equal(t, 1, p.OwnerCount(d.Info().GUID))
}

func TestCleanupCapabilitySurvivesGenerationFence(t *testing.T) {
	var out bytes.Buffer
	frames, err := lifecycle.NewFrameOwner(&out)
	require.NoError(t, err)
	gate, err := newGenerationOutputGate(frames)
	require.NoError(t, err)
	require.NoError(t, gate.Activate())
	cleanup, err := NewCleanupOutputGate(frames)
	require.NoError(t, err)
	p := hostoutput.New()
	d := testHostDefinition(t, "node")
	owner := p.NewOwner(d.Info().GUID)
	defer owner.Release()
	tx := hostoutput.Prepare(hostoutput.Request{
		Owner:      owner,
		Definition: d,
		Payload:    []byte("samples"),
	}, nil)
	require.NoError(t, gate.CommitBuiltJobOutput(tx.Build, tx))
	gate.Fence()
	out.Reset()
	tx = hostoutput.Prepare(
		hostoutput.Request{
			Owner:      owner,
			Definition: d,
			Payload:    []byte("obsolete"),
			Cleanup:    true,
		},
		nil,
	)
	require.NoError(t, cleanup.CommitBuiltJobOutput(tx.Build, tx))
	assert.Equal(t, "obsolete", out.String())
	cleanup.Fence()
	out.Reset()
	tx = hostoutput.Prepare(
		hostoutput.Request{
			Owner:      owner,
			Definition: d,
			Payload:    []byte("obsolete"),
			Cleanup:    true,
		},
		nil,
	)
	require.Error(t, cleanup.CommitBuiltJobOutput(tx.Build, tx))
	assert.Empty(t, out.String())
}

func BenchmarkConfiguredHostPublication(b *testing.B) {
	for _, count := range []int{1, 1000, 10000} {
		b.Run(fmt.Sprintf("configured_%d", count), func(b *testing.B) {
			frames, err := lifecycle.NewFrameOwner(io.Discard)
			require.NoError(b, err)
			gate, err := newGenerationOutputGate(frames)
			require.NoError(b, err)
			require.NoError(b, gate.Activate())
			p := hostoutput.New()
			d := testHostDefinition(b, "node")
			guid := d.Info().GUID
			definitions := make(map[string]*vnodes.VirtualNode, count)
			for i := 0; i < count; i++ {
				key := fmt.Sprint(i)
				definitions[key] = &vnodes.VirtualNode{
					Name:     key,
					GUID:     key,
					Hostname: key,
				}
			}
			definitions[guid] = &vnodes.VirtualNode{
				Name:     guid,
				GUID:     guid,
				Hostname: d.Info().Hostname,
				Labels:   d.Info().Labels,
			}
			configuration, err := discovery.NewVNodeConfigurationWithInitial(definitions)
			require.NoError(b, err)
			p.Bind(configuration.Definition)
			owner := p.NewOwner(guid)
			defer owner.Release()
			request := hostoutput.Request{
				Owner:      owner,
				Definition: d,
				Payload:    []byte("HOST '" + guid + "'\nBEGIN 'job.chart'\nSET 'value' = 1\nEND\n"),
			}
			warm := hostoutput.Prepare(request, nil)
			require.NoError(b, gate.CommitBuiltJobOutput(warm.Build, warm))
			b.ReportAllocs()
			b.ResetTimer()
			// O(1) lookup and publication checks per host batch, independent of configured inventory size.
			for b.Loop() {
				tx := hostoutput.Prepare(request, nil)
				if err := gate.CommitBuiltJobOutput(tx.Build, tx); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

type hostTestWriter func([]byte) (int, error)

func (f hostTestWriter) Write(payload []byte) (int, error) { return f(payload) }

type hostTestState struct {
	mu     sync.Mutex
	aborts int
	fail   bool
}

func (s *hostTestState) Commit() error {
	if s.fail {
		return errors.New("state failure")
	}
	return nil
}
func (s *hostTestState) Abort() error { s.mu.Lock(); defer s.mu.Unlock(); s.aborts++; return nil }

func BenchmarkHostPublicationChangesAndRetirement(b *testing.B) {
	for name, tc := range map[string]struct{ retire bool }{"metadata change": {}, "owner retirement": {retire: true}} {
		b.Run(name, func(b *testing.B) {
			frames, err := lifecycle.NewFrameOwner(io.Discard)
			require.NoError(b, err)
			gate, err := newGenerationOutputGate(frames)
			require.NoError(b, err)
			require.NoError(b, gate.Activate())
			p := hostoutput.New()
			definitions := []*hostoutput.Definition{testHostDefinition(b, "one"), testHostDefinition(b, "two")}
			guid := definitions[0].Info().GUID
			owner := p.NewOwner(guid)
			payload := []byte("HOST '" + guid + "'\nBEGIN 'job.chart'\nSET 'value' = 1\nEND\n")
			iteration := 0
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				definition := definitions[iteration%2]
				iteration++
				if tc.retire {
					owner.Release()
					owner = p.NewOwner(guid)
				}
				tx := hostoutput.Prepare(
					hostoutput.Request{
						Owner:      owner,
						Definition: definition,
						Payload:    payload,
					},
					nil,
				)
				if err := gate.CommitBuiltJobOutput(tx.Build, tx); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			owner.Release()
			require.Zero(b, p.Len())
		})
	}
}
