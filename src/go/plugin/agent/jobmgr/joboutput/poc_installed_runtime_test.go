// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errPOCInstalledIdentityBusy = errors.New("POC installed identity is busy")
	errPOCOutputFenced          = errors.New("POC installed output is fenced")
)

func TestPOCInstalledRuntimeRetirementFencesOutputAndDetachesRun(t *testing.T) {
	tests := map[string]func([]jobruntime.Support) jobruntime.Runtime{
		"V1": func(support []jobruntime.Support) jobruntime.Runtime {
			return jobruntime.NewV1Runtime(support)
		},
		"V2": func(support []jobruntime.Support) jobruntime.Runtime {
			return jobruntime.NewV2Runtime(support)
		},
	}
	for name, newRuntime := range tests {
		t.Run(name, func(t *testing.T) {
			var output bytes.Buffer
			frame, err := lifecycle.NewFrameOwner(&output)
			require.NoError(t, err)
			writer := &pocFencedFrameWriter{
				writer: FrameWriter{Owner: frame},
			}
			physical := newPOCPhysicalRuntimeSupport(writer, false)
			t.Cleanup(physical.release)
			projection := &pocRunProjectionSupport{}
			var cleanups int
			authority := newPOCInstalledAuthority()
			entry, err := authority.install(
				"module_job",
				writer,
				newRuntime([]jobruntime.Support{physical}),
				newRuntime([]jobruntime.Support{projection}),
				func() { cleanups++ },
			)
			require.NoError(t, err)
			physical.waitStarted(t)
			require.Equal(t, "active\n", output.String())

			retireCtx, cancelRetire := context.WithCancel(context.Background())
			retired := make(chan error, 1)
			go func() {
				retired <- authority.retire(retireCtx, "module_job")
			}()
			physical.waitStopEntered(t)
			physical.waitLateWrite(t)
			cancelRetire()
			require.ErrorIs(t, <-retired, context.Canceled)

			assert.True(t, projection.detached())
			assert.ErrorIs(t, physical.lateWriteError(), errPOCOutputFenced)
			assert.Equal(t, "active\n", output.String())
			assert.True(t, authority.occupied("module_job"))

			var transactionEvents []string
			err = writer.CommitJobOutput(
				[]byte("late transaction\n"),
				&recordingFrameState{events: &transactionEvents},
			)
			require.ErrorIs(t, err, errPOCOutputFenced)
			assert.Equal(t, []string{"abort"}, transactionEvents)
			assert.Equal(t, "active\n", output.String())

			_, err = authority.install(
				"module_job",
				writer,
				newRuntime([]jobruntime.Support{newPOCPhysicalRuntimeSupport(writer, true)}),
				newRuntime([]jobruntime.Support{&pocRunProjectionSupport{}}),
				func() {},
			)
			require.ErrorIs(t, err, errPOCInstalledIdentityBusy)

			otherWriter := newPOCTestWriter(t)
			otherPhysical := newPOCPhysicalRuntimeSupport(otherWriter, true)
			other, err := authority.install(
				"module_other",
				otherWriter,
				newRuntime([]jobruntime.Support{otherPhysical}),
				newRuntime([]jobruntime.Support{&pocRunProjectionSupport{}}),
				func() {},
			)
			require.NoError(t, err)
			require.NoError(t, authority.retire(context.Background(), "module_other"))
			<-other.done

			physical.release()
			<-entry.done
			assert.Equal(t, 1, cleanups)
			assert.False(t, authority.occupied("module_job"))
		})
	}
}

type pocFencedFrameWriter struct {
	mu     sync.RWMutex
	writer FrameWriter
	fenced bool
}

func newPOCTestWriter(t *testing.T) *pocFencedFrameWriter {
	t.Helper()
	frame, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
	require.NoError(t, err)
	return &pocFencedFrameWriter{
		writer: FrameWriter{Owner: frame},
	}
}

func (writer *pocFencedFrameWriter) Fence() {
	writer.mu.Lock()
	writer.fenced = true
	writer.mu.Unlock()
}

func (writer *pocFencedFrameWriter) Write(payload []byte) (int, error) {
	writer.mu.RLock()
	defer writer.mu.RUnlock()
	if writer.fenced {
		return 0, errPOCOutputFenced
	}
	return writer.writer.Write(payload)
}

func (writer *pocFencedFrameWriter) CommitJobOutput(
	payload []byte,
	transaction jobruntime.OutputStateTransaction,
) error {
	writer.mu.RLock()
	defer writer.mu.RUnlock()
	if writer.fenced {
		return errors.Join(errPOCOutputFenced, transaction.Abort())
	}
	return writer.writer.CommitJobOutput(payload, transaction)
}

type pocPhysicalRuntimeSupport struct {
	writer *pocFencedFrameWriter

	started       chan struct{}
	stopEntered   chan struct{}
	lateAttempted chan struct{}
	releaseStop   chan struct{}
	releaseOnce   sync.Once

	mu      sync.Mutex
	lateErr error
}

func newPOCPhysicalRuntimeSupport(
	writer *pocFencedFrameWriter,
	cooperative bool,
) *pocPhysicalRuntimeSupport {
	support := &pocPhysicalRuntimeSupport{
		writer:        writer,
		started:       make(chan struct{}),
		stopEntered:   make(chan struct{}),
		lateAttempted: make(chan struct{}),
		releaseStop:   make(chan struct{}),
	}
	if cooperative {
		support.release()
	}
	return support
}

func (support *pocPhysicalRuntimeSupport) Start(context.Context) error {
	if _, err := support.writer.Write([]byte("active\n")); err != nil {
		return err
	}
	close(support.started)
	return nil
}

func (support *pocPhysicalRuntimeSupport) Stop(context.Context) error {
	close(support.stopEntered)
	_, err := support.writer.Write([]byte("late\n"))
	support.mu.Lock()
	support.lateErr = err
	support.mu.Unlock()
	close(support.lateAttempted)
	<-support.releaseStop
	return nil
}

func (*pocPhysicalRuntimeSupport) Release(context.Context) error {
	return nil
}

func (support *pocPhysicalRuntimeSupport) release() {
	support.releaseOnce.Do(func() {
		close(support.releaseStop)
	})
}

func (support *pocPhysicalRuntimeSupport) waitStarted(t *testing.T) {
	t.Helper()
	<-support.started
}

func (support *pocPhysicalRuntimeSupport) waitStopEntered(t *testing.T) {
	t.Helper()
	<-support.stopEntered
}

func (support *pocPhysicalRuntimeSupport) waitLateWrite(t *testing.T) {
	t.Helper()
	<-support.lateAttempted
}

func (support *pocPhysicalRuntimeSupport) lateWriteError() error {
	support.mu.Lock()
	defer support.mu.Unlock()
	return support.lateErr
}

type pocRunProjectionSupport struct {
	mu       sync.Mutex
	started  bool
	stopped  bool
	released bool
}

func (support *pocRunProjectionSupport) Start(context.Context) error {
	support.mu.Lock()
	defer support.mu.Unlock()
	support.started = true
	return nil
}

func (support *pocRunProjectionSupport) Stop(context.Context) error {
	support.mu.Lock()
	defer support.mu.Unlock()
	support.stopped = true
	return nil
}

func (support *pocRunProjectionSupport) Release(context.Context) error {
	support.mu.Lock()
	defer support.mu.Unlock()
	support.released = true
	return nil
}

func (support *pocRunProjectionSupport) detached() bool {
	support.mu.Lock()
	defer support.mu.Unlock()
	return support.started && support.stopped && support.released
}

type pocInstalledAuthority struct {
	mu      sync.Mutex
	entries map[string]*pocInstalledEntry
}

type pocInstalledEntry struct {
	id         string
	writer     *pocFencedFrameWriter
	physical   jobruntime.Runtime
	projection jobruntime.Runtime
	cleanup    func()
	done       chan struct{}
	detachOnce sync.Once
	detachErr  error
	finalErr   error
}

func newPOCInstalledAuthority() *pocInstalledAuthority {
	return &pocInstalledAuthority{
		entries: make(map[string]*pocInstalledEntry),
	}
}

func (authority *pocInstalledAuthority) install(
	id string,
	writer *pocFencedFrameWriter,
	physical jobruntime.Runtime,
	projection jobruntime.Runtime,
	cleanup func(),
) (*pocInstalledEntry, error) {
	authority.mu.Lock()
	if authority.entries[id] != nil {
		authority.mu.Unlock()
		return nil, errPOCInstalledIdentityBusy
	}
	entry := &pocInstalledEntry{
		id:         id,
		writer:     writer,
		physical:   physical,
		projection: projection,
		cleanup:    cleanup,
		done:       make(chan struct{}),
	}
	authority.entries[id] = entry
	authority.mu.Unlock()

	if err := physical.Start(context.Background()); err != nil {
		return nil, err
	}
	if err := projection.Start(context.Background()); err != nil {
		return nil, err
	}
	return entry, nil
}

func (authority *pocInstalledAuthority) retire(ctx context.Context, id string) error {
	authority.mu.Lock()
	entry := authority.entries[id]
	authority.mu.Unlock()
	if entry == nil {
		return errors.New("POC installed identity is not active")
	}
	entry.detachOnce.Do(func() {
		entry.writer.Fence()
		entry.detachErr = errors.Join(
			entry.projection.Stop(ctx),
			entry.projection.ReleaseAfterCleanup(ctx),
		)
		go authority.finalize(entry)
	})
	if entry.detachErr != nil {
		return entry.detachErr
	}
	select {
	case <-entry.done:
		return entry.finalErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (authority *pocInstalledAuthority) finalize(entry *pocInstalledEntry) {
	entry.finalErr = errors.Join(
		entry.physical.Stop(context.Background()),
		entry.physical.ReleaseAfterCleanup(context.Background()),
	)
	entry.cleanup()
	authority.mu.Lock()
	if authority.entries[entry.id] == entry {
		delete(authority.entries, entry.id)
	}
	authority.mu.Unlock()
	close(entry.done)
}

func (authority *pocInstalledAuthority) occupied(id string) bool {
	authority.mu.Lock()
	defer authority.mu.Unlock()
	return authority.entries[id] != nil
}
