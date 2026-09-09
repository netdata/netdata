// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestV1RuntimeCycleAndStop(t *testing.T) {
	testRuntimeJoinBeforePostCleanupRelease(t, func(support []Support) Runtime {
		return NewV1Runtime(support)
	})
}

func TestV2RuntimeCycleAndRunnerStop(t *testing.T) {
	testRuntimeJoinBeforePostCleanupRelease(t, func(support []Support) Runtime {
		return NewV2Runtime(support)
	})
}

func testRuntimeJoinBeforePostCleanupRelease(t *testing.T, newRuntime func([]Support) Runtime) {
	t.Helper()
	events := []string{}
	runtime := newRuntime([]Support{
		&recordingLifecycleSupport{id: "first", events: &events},
		&recordingLifecycleSupport{id: "second", events: &events},
	})
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ReleaseAfterCleanup(context.Background()); !errors.Is(err, ErrLifecycleRuntimeNotStopped) {
		t.Fatalf("early release error=%v, want ErrLifecycleRuntimeNotStopped", err)
	}
	if err := runtime.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ReleaseAfterCleanup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ReleaseAfterCleanup(context.Background()); err != nil {
		t.Fatalf("idempotent release: %v", err)
	}
	if got, want := events, []string{
		"start:first", "start:second",
		"stop:second", "stop:first",
		"release:second", "release:first",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("events=%v want=%v", got, want)
	}
}

func TestV2RuntimeAbortsOnlyAcquiredSupportAfterPartialStart(t *testing.T) {
	events := []string{}
	wantFailure := errors.New("start failed")
	runtime := NewV2Runtime([]Support{
		&recordingLifecycleSupport{id: "runner", events: &events},
		&recordingLifecycleSupport{id: "registration", events: &events, startErr: wantFailure},
		&recordingLifecycleSupport{id: "scope", events: &events},
	})
	if err := runtime.Start(context.Background()); !errors.Is(err, wantFailure) {
		t.Fatalf("Start error=%v, want %v", err, wantFailure)
	}
	if err := runtime.Abort(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Abort(context.Background()); err != nil {
		t.Fatalf("idempotent Abort: %v", err)
	}
	if got, want := events, []string{
		"start:runner", "start:registration",
		"stop:runner", "release:runner",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("events=%v want=%v", got, want)
	}
}

type recordingLifecycleSupport struct {
	id       string
	events   *[]string
	startErr error
	stopErr  error
	relErr   error
}

func (support *recordingLifecycleSupport) Start(context.Context) error {
	*support.events = append(*support.events, "start:"+support.id)
	return support.startErr
}

func (support *recordingLifecycleSupport) Stop(context.Context) error {
	*support.events = append(*support.events, "stop:"+support.id)
	return support.stopErr
}

func (support *recordingLifecycleSupport) Release(context.Context) error {
	*support.events = append(*support.events, "release:"+support.id)
	return support.relErr
}
