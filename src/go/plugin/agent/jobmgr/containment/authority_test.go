// SPDX-License-Identifier: GPL-3.0-or-later

package containment

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/stretchr/testify/require"
)

func TestAuthorityContainsOnlyTheOccupiedIdentity(t *testing.T) {
	authority := newTestAuthority(t, time.Second, 20*time.Millisecond, nil)
	identity := testIdentity(NamespaceJob, "module/job", "module/job")
	entered := make(chan struct{})
	release := make(chan struct{})
	attempt, err := authority.Start(Plan{
		Identity: identity,
		Target:   7,
		Work: func(context.Context) error {
			close(entered)
			<-release
			return nil
		},
	})
	require.NoError(t, err)
	<-entered

	_, err = authority.Start(Plan{
		Identity: identity,
		Target:   7,
		Work:     func(context.Context) error { return nil },
	})
	require.ErrorIs(t, err, ErrIdentityBusy)

	other, err := authority.Start(Plan{
		Identity: testIdentity(NamespaceJob, "module/other", "module/other"),
		Target:   7,
		Work:     func(context.Context) error { return nil },
	})
	require.NoError(t, err)
	require.NoError(t, other.Await(context.Background()))
	<-other.Released()

	require.ErrorIs(t, authority.Supersede(context.Background(), identity), ErrIdentityBusy)
	require.ErrorIs(t, attempt.Await(context.Background()), ErrSuperseded)
	require.Equal(t, Census{Active: 1, Contained: 1}, authority.Census())

	close(release)
	<-attempt.Released()
	require.Equal(t, Census{}, authority.Census())

	successor, err := authority.Start(Plan{
		Identity: identity,
		Target:   8,
		Work:     func(context.Context) error { return nil },
	})
	require.NoError(t, err)
	require.NoError(t, successor.Await(context.Background()))
	<-successor.Released()
}

func TestAuthorityFuseWinsLateCompletionAndRedactsDiagnostics(t *testing.T) {
	diagnostics := &recordingAttemptDiagnostics{}
	authority := newTestAuthority(t, 20*time.Millisecond, 10*time.Millisecond, diagnostics)
	identity := testIdentity(
		NamespaceStore,
		"raw-config-sensitive-fingerprint",
		"vault/main",
	)
	release := make(chan struct{})
	attempt, err := authority.Start(Plan{
		Identity: identity,
		Target:   11,
		Work: func(context.Context) error {
			<-release
			return errors.New("provider-sensitive-detail")
		},
	})
	require.NoError(t, err)

	require.ErrorIs(t, attempt.Await(context.Background()), ErrContainmentDeadline)
	require.Equal(t, Census{Active: 1, Contained: 1}, authority.Census())
	require.Equal(t, AttemptStateContained, attempt.State())

	close(release)
	<-attempt.Released()
	require.Equal(t, Census{}, authority.Census())
	require.Equal(t, AttemptStateReleased, attempt.State())

	wire := diagnostics.String()
	require.Contains(t, wire, "vault/main")
	require.NotContains(t, wire, identity.Key)
	require.NotContains(t, wire, "provider-sensitive-detail")
}

func TestAuthorityAdmissionStopsFuseButTargetCutStillContainsLifetime(t *testing.T) {
	authority := newTestAuthority(t, 20*time.Millisecond, 10*time.Millisecond, nil)
	entered := make(chan struct{})
	release := make(chan struct{})
	attempt, err := authority.Start(Plan{
		Identity: testIdentity(NamespaceJob, "module/job", "module/job"),
		Target:   23,
		Work: func(context.Context) error {
			close(entered)
			<-release
			return nil
		},
	})
	require.NoError(t, err)
	<-entered
	require.NoError(t, attempt.Admit())

	time.Sleep(40 * time.Millisecond)
	require.Equal(t, Census{Active: 1, Admitted: 1}, authority.Census())
	require.EqualValues(t, 1, authority.CutTarget(23))
	require.ErrorIs(t, attempt.Await(context.Background()), ErrTargetRetired)
	require.Equal(t, Census{Active: 1, Contained: 1}, authority.Census())

	close(release)
	<-attempt.Released()
	require.Equal(t, Census{}, authority.Census())
}

func TestAuthorityCompletionAndCutHaveOneTerminalOwner(t *testing.T) {
	for range 200 {
		authority := newTestAuthority(t, time.Second, 10*time.Millisecond, nil)
		entered := make(chan struct{})
		release := make(chan struct{})
		attempt, err := authority.Start(Plan{
			Identity: testIdentity(NamespaceJobTest, "module/job/config", "module/job"),
			Work: func(context.Context) error {
				close(entered)
				<-release
				return nil
			},
		})
		require.NoError(t, err)
		<-entered

		cut := make(chan bool, 1)
		go func() {
			cut <- attempt.Cut(ErrSuperseded)
		}()
		close(release)

		err = attempt.Await(context.Background())
		cutWon := <-cut
		switch {
		case err == nil:
			require.False(t, cutWon)
		case errors.Is(err, ErrSuperseded):
			require.True(t, cutWon)
		default:
			require.NoError(t, err)
		}
		<-attempt.Released()
		require.Equal(t, Census{}, authority.Census())
	}
}

func TestAuthorityContainsPanicsWithoutPublishingPanicValues(t *testing.T) {
	diagnostics := &recordingAttemptDiagnostics{}
	authority := newTestAuthority(t, time.Second, 10*time.Millisecond, diagnostics)
	attempt, err := authority.Start(Plan{
		Identity: testIdentity(NamespaceFunctionPoll, "module/job", "module/job"),
		Work: func(context.Context) error {
			panic("provider-sensitive-panic")
		},
	})
	require.NoError(t, err)
	require.ErrorIs(t, attempt.Await(context.Background()), ErrWorkerPanic)
	<-attempt.Released()
	require.NotContains(t, diagnostics.String(), "provider-sensitive-panic")
}

func TestAuthorityShutdownReportsBoundedRetainedSample(t *testing.T) {
	diagnostics := &recordingAttemptDiagnostics{}
	authority := newTestAuthority(t, time.Second, 10*time.Millisecond, diagnostics)
	releases := make([]chan struct{}, 0, 12)
	attempts := make([]*Attempt, 0, 12)
	for index := range 12 {
		release := make(chan struct{})
		releases = append(releases, release)
		attempt, err := authority.Start(Plan{
			Identity: testIdentity(
				NamespaceFunctionInvocation,
				fmt.Sprintf("private-%d", index),
				fmt.Sprintf("module/job/%d", index),
			),
			Work: func(context.Context) error {
				<-release
				return nil
			},
		})
		require.NoError(t, err)
		attempts = append(attempts, attempt)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, authority.Shutdown(ctx), context.DeadlineExceeded)
	require.Equal(t, Census{Active: 12, Contained: 12}, authority.Census())

	events := diagnostics.Snapshot()
	var aggregate *jobmgr.DiagnosticEvent
	samples := 0
	for index := range events {
		switch events[index].Name {
		case "job manager process retained attempts":
			event := events[index]
			aggregate = &event
		case "job manager process retained attempt":
			samples++
		}
	}
	require.NotNil(t, aggregate)
	require.EqualValues(t, 12, aggregate.Count)
	require.LessOrEqual(t, samples, MaximumDiagnosticIdentitySample)

	for _, release := range releases {
		close(release)
	}
	for _, attempt := range attempts {
		<-attempt.Released()
	}
	require.Equal(t, Census{}, authority.Census())
}

func TestAuthorityProductionPolicyIsFixed(t *testing.T) {
	require.Equal(t, 2*time.Minute, DefaultFuse)
	require.Equal(t, 2*time.Second, DefaultSupersessionGrace)
}

func newTestAuthority(
	t *testing.T,
	fuse time.Duration,
	grace time.Duration,
	diagnostics jobmgr.DiagnosticObserver,
) *Authority {
	t.Helper()
	authority, err := newAuthority(diagnostics, policy{
		fuse:              fuse,
		supersessionGrace: grace,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, authority.Shutdown(ctx))
	})
	return authority
}

func testIdentity(namespace Namespace, key, resource string) Identity {
	return Identity{
		Namespace: namespace,
		Key:       key,
		Resource:  resource,
	}
}

type recordingAttemptDiagnostics struct {
	mu     sync.Mutex
	events []jobmgr.DiagnosticEvent
}

func (rad *recordingAttemptDiagnostics) ObserveDiagnostic(event jobmgr.DiagnosticEvent) {
	rad.mu.Lock()
	rad.events = append(rad.events, event)
	rad.mu.Unlock()
}

func (rad *recordingAttemptDiagnostics) Snapshot() []jobmgr.DiagnosticEvent {
	rad.mu.Lock()
	defer rad.mu.Unlock()
	return append([]jobmgr.DiagnosticEvent(nil), rad.events...)
}

func (rad *recordingAttemptDiagnostics) String() string {
	events := rad.Snapshot()
	var builder strings.Builder
	for _, event := range events {
		fmt.Fprintf(
			&builder,
			"%s %s %s %v %d\n",
			event.Name,
			event.Resource,
			event.State,
			event.Err,
			event.Count,
		)
	}
	return builder.String()
}
