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
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/stretchr/testify/require"
)

func (at *attempt) stateSnapshot() attemptState {
	if at == nil || at.authority == nil {
		return 0
	}
	at.authority.mu.Lock()
	defer at.authority.mu.Unlock()
	return at.state
}

func TestAuthorityRejectsCanceledClaimBeforeReservingIdentity(t *testing.T) {
	authority := newTestAuthority(t, time.Second, 20*time.Millisecond, nil)
	ctx, cancel := context.WithCancelCause(context.Background())
	cause := errors.New("test: caller abandoned claim")
	cancel(cause)

	attempt, err := authority.start(ctx, jobmgr.ProcessAttemptPlan{
		Identity: testIdentity(jobmgr.ProcessAttemptJob, "module/job", "module/job"),
		Target:   7,
		Work:     func(context.Context, jobmgr.ProcessAttemptAdmission) error { return nil },
	})

	require.Nil(t, attempt)
	require.ErrorIs(t, err, cause)
	require.Equal(t, Census{}, authority.Census())
}

func TestAuthorityContainsOnlyTheOccupiedIdentity(t *testing.T) {
	authority := newTestAuthority(t, time.Second, 20*time.Millisecond, nil)
	identity := testIdentity(jobmgr.ProcessAttemptJob, "module/job", "module/job")
	entered := make(chan struct{})
	release := make(chan struct{})
	attempt, err := authority.start(context.Background(), jobmgr.ProcessAttemptPlan{
		Identity: identity,
		Target:   7,
		Work: func(context.Context, jobmgr.ProcessAttemptAdmission) error {
			close(entered)
			<-release
			return nil
		},
	})
	require.NoError(t, err)
	<-entered

	_, err = authority.start(context.Background(), jobmgr.ProcessAttemptPlan{
		Identity: identity,
		Target:   7,
		Work:     func(context.Context, jobmgr.ProcessAttemptAdmission) error { return nil },
	})
	require.ErrorIs(t, err, jobmgr.ErrProcessAttemptBusy)

	other, err := authority.start(context.Background(), jobmgr.ProcessAttemptPlan{
		Identity: testIdentity(jobmgr.ProcessAttemptJob, "module/other", "module/other"),
		Target:   7,
		Work:     func(context.Context, jobmgr.ProcessAttemptAdmission) error { return nil },
	})
	require.NoError(t, err)
	require.NoError(t, other.Await(context.Background()))
	<-other.Released()

	require.ErrorIs(
		t,
		authority.SupersedeProcessAttempt(context.Background(), identity),
		jobmgr.ErrProcessAttemptBusy,
	)
	require.ErrorIs(t, attempt.Await(context.Background()), jobmgr.ErrProcessAttemptSuperseded)
	require.Equal(t, Census{Active: 1, Contained: 1}, authority.Census())

	close(release)
	<-attempt.Released()
	require.Equal(t, Census{}, authority.Census())

	successor, err := authority.start(context.Background(), jobmgr.ProcessAttemptPlan{
		Identity: identity,
		Target:   8,
		Work:     func(context.Context, jobmgr.ProcessAttemptAdmission) error { return nil },
	})
	require.NoError(t, err)
	require.NoError(t, successor.Await(context.Background()))
	<-successor.Released()
}

func TestAuthorityFuseWinsLateCompletionAndRedactsDiagnostics(t *testing.T) {
	diagnostics := &recordingAttemptDiagnostics{}
	authority := newTestAuthority(t, 20*time.Millisecond, 10*time.Millisecond, diagnostics)
	identity := testIdentity(
		jobmgr.ProcessAttemptStore,
		"raw-config-sensitive-fingerprint",
		"vault/main",
	)
	release := make(chan struct{})
	fenced := false
	attempt, err := authority.start(context.Background(), jobmgr.ProcessAttemptPlan{
		Identity: identity,
		Target:   11,
		OnContainment: func(error) {
			fenced = true
		},
		Work: func(context.Context, jobmgr.ProcessAttemptAdmission) error {
			<-release
			return errors.New("provider-sensitive-detail")
		},
	})
	require.NoError(t, err)

	require.ErrorIs(t, attempt.Await(context.Background()), jobmgr.ErrProcessAttemptDeadline)
	require.True(t, fenced)
	require.Equal(t, Census{Active: 1, Contained: 1}, authority.Census())
	require.Equal(t, attemptStateContained, attempt.stateSnapshot())

	close(release)
	<-attempt.Released()
	require.Equal(t, Census{}, authority.Census())
	require.Equal(t, attemptStateReleased, attempt.stateSnapshot())

	wire := diagnostics.String()
	require.Contains(t, wire, "vault/main")
	require.NotContains(t, wire, identity.Key)
	require.NotContains(t, wire, "provider-sensitive-detail")
}

func TestAuthorityAdmissionStopsFuseButTargetCutStillContainsLifetime(t *testing.T) {
	diagnostics := &recordingAttemptDiagnostics{}
	authority := newTestAuthority(t, 20*time.Millisecond, 10*time.Millisecond, diagnostics)
	entered := make(chan struct{})
	release := make(chan struct{})
	attempt, err := authority.start(context.Background(), jobmgr.ProcessAttemptPlan{
		Identity: testIdentity(jobmgr.ProcessAttemptJob, "module/job", "module/job"),
		Target:   23,
		Work: func(context.Context, jobmgr.ProcessAttemptAdmission) error {
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
	require.ErrorIs(t, attempt.Await(context.Background()), jobmgr.ErrProcessAttemptRetired)
	require.Equal(t, Census{Active: 1, Contained: 1}, authority.Census())
	events := diagnostics.Snapshot()

	close(release)
	<-attempt.Released()
	require.Equal(t, Census{}, authority.Census())
	require.Len(t, events, 1)
	require.Equal(t, jobmgr.DiagnosticInfo, events[0].Level)
	require.ErrorIs(t, events[0].Err, jobmgr.ErrProcessAttemptRetired)
}

func TestAuthorityTargetCutPermanentlyRejectsRetiredTargets(t *testing.T) {
	authority := newTestAuthority(t, time.Second, 10*time.Millisecond, nil)

	require.Zero(t, authority.CutTarget(23))
	for _, target := range []uint64{22, 23} {
		_, err := authority.start(context.Background(), jobmgr.ProcessAttemptPlan{
			Identity: testIdentity(
				jobmgr.ProcessAttemptJob,
				fmt.Sprintf("module/job-%d", target),
				fmt.Sprintf("module/job-%d", target),
			),
			Target: target,
			Work:   func(context.Context, jobmgr.ProcessAttemptAdmission) error { return nil },
		})
		require.ErrorIs(t, err, jobmgr.ErrProcessAttemptRetired)
	}

	successor, err := authority.start(context.Background(), jobmgr.ProcessAttemptPlan{
		Identity: testIdentity(jobmgr.ProcessAttemptJob, "module/successor", "module/successor"),
		Target:   24,
		Work:     func(context.Context, jobmgr.ProcessAttemptAdmission) error { return nil },
	})
	require.NoError(t, err)
	require.NoError(t, successor.Await(context.Background()))
	<-successor.Released()
}

func TestAuthorityCallerCancellationPrecedesUnobservedCompletion(t *testing.T) {
	authority := newTestAuthority(t, time.Second, 10*time.Millisecond, nil)

	for index := range 200 {
		attempt, err := authority.start(context.Background(), jobmgr.ProcessAttemptPlan{
			Identity: testIdentity(
				jobmgr.ProcessAttemptFunctionInvocation,
				fmt.Sprintf("function-%d", index),
				fmt.Sprintf("function-%d", index),
			),
			Target: 1,
			Work:   func(context.Context, jobmgr.ProcessAttemptAdmission) error { return nil },
		})
		require.NoError(t, err)
		<-attempt.Released()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		require.ErrorIs(t, attempt.Await(ctx), context.Canceled)
	}
}

func TestAuthorityCompletionAndCutHaveOneTerminalOwner(t *testing.T) {
	for range 200 {
		authority := newTestAuthority(t, time.Second, 10*time.Millisecond, nil)
		entered := make(chan struct{})
		release := make(chan struct{})
		attempt, err := authority.start(context.Background(), jobmgr.ProcessAttemptPlan{
			Identity: testIdentity(jobmgr.ProcessAttemptJobTest, "module/job/config", "module/job"),
			Work: func(context.Context, jobmgr.ProcessAttemptAdmission) error {
				close(entered)
				<-release
				return nil
			},
		})
		require.NoError(t, err)
		<-entered

		cut := make(chan bool, 1)
		go func() {
			cut <- attempt.Cut(jobmgr.ErrProcessAttemptSuperseded)
		}()
		close(release)

		err = attempt.Await(context.Background())
		cutWon := <-cut
		switch {
		case err == nil:
			require.False(t, cutWon)
		case errors.Is(err, jobmgr.ErrProcessAttemptSuperseded):
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
	identity := testIdentity(jobmgr.ProcessAttemptFunctionPoll, "module/job", "module/job")
	attempt, err := authority.start(context.Background(), jobmgr.ProcessAttemptPlan{
		Identity: identity,
		Work: func(context.Context, jobmgr.ProcessAttemptAdmission) error {
			panic("provider-sensitive-panic")
		},
	})
	require.NoError(t, err)
	require.ErrorIs(t, attempt.Await(context.Background()), jobmgr.ErrProcessAttemptQuarantined)
	require.ErrorIs(t, attempt.Await(context.Background()), jobmgr.ErrProcessAttemptWorkerPanic)
	<-attempt.Released()
	require.Equal(t, Census{Quarantined: 1}, authority.Census())
	_, err = authority.start(context.Background(), jobmgr.ProcessAttemptPlan{
		Identity: identity,
		Work:     func(context.Context, jobmgr.ProcessAttemptAdmission) error { return nil },
	})
	require.ErrorIs(t, err, jobmgr.ErrProcessAttemptQuarantined)
	require.NotContains(t, diagnostics.String(), "provider-sensitive-panic")
}

func TestAuthorityContainsContainmentFencePanics(t *testing.T) {
	diagnostics := &recordingAttemptDiagnostics{}
	authority, err := newAuthority(diagnostics, policy{
		fuse:              time.Second,
		supersessionGrace: 10 * time.Millisecond,
	})
	require.NoError(t, err)

	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseWork := func() {
		releaseOnce.Do(func() { close(release) })
	}
	defer releaseWork()
	attempt, err := authority.start(context.Background(), jobmgr.ProcessAttemptPlan{
		Identity: testIdentity(jobmgr.ProcessAttemptFunctionPoll, "module/job", "module/job"),
		OnContainment: func(error) {
			panic("provider-sensitive-fence-panic")
		},
		Work: func(context.Context, jobmgr.ProcessAttemptAdmission) error {
			<-release
			return nil
		},
	})
	require.NoError(t, err)

	var cut bool
	require.NotPanics(t, func() {
		cut = attempt.Cut(jobmgr.ErrProcessAttemptRetired)
	})
	require.True(t, cut)
	require.Equal(t, Census{Active: 1, Contained: 1}, authority.Census())
	require.ErrorIs(t, attempt.Await(context.Background()), jobmgr.ErrProcessAttemptRetired)
	require.ErrorIs(t, attempt.Await(context.Background()), jobmgr.ErrProcessAttemptFencePanic)
	events := diagnostics.Snapshot()
	require.Len(t, events, 1)
	require.Equal(t, jobmgr.DiagnosticError, events[0].Level)

	releaseWork()
	<-attempt.Released()
	require.Equal(t, Census{Quarantined: 1}, authority.Census())
	require.Contains(t, diagnostics.String(), jobmgr.ErrProcessAttemptFencePanic.Error())
	require.NotContains(t, diagnostics.String(), "provider-sensitive-fence-panic")
}

func TestAuthorityContainmentFenceReceivesRawCause(t *testing.T) {
	authority := newTestAuthority(t, time.Second, 10*time.Millisecond, nil)
	release := make(chan struct{})
	fenced := make(chan error, 1)
	attempt, err := authority.start(context.Background(), jobmgr.ProcessAttemptPlan{
		Identity: testIdentity(jobmgr.ProcessAttemptJobRuntime, "module/job", "module/job"),
		Target:   23,
		OnContainment: func(cause error) {
			fenced <- cause
		},
		Work: func(context.Context, jobmgr.ProcessAttemptAdmission) error {
			<-release
			return nil
		},
	})
	require.NoError(t, err)

	independent := errors.New("independent failure")
	cause := errors.Join(jobmgr.ErrProcessAttemptRetired, independent)
	require.True(t, attempt.Cut(cause))
	require.Equal(t, cause, <-fenced)
	require.ErrorIs(t, attempt.Await(context.Background()), jobmgr.ErrProcessAttemptRetired)
	require.ErrorIs(t, attempt.Await(context.Background()), independent)

	close(release)
	<-attempt.Released()
}

func TestAuthorityOrdinaryPreAdmissionErrorDoesNotQuarantine(t *testing.T) {
	authority := newTestAuthority(t, time.Second, 10*time.Millisecond, nil)
	identity := testIdentity(jobmgr.ProcessAttemptJob, "module/job", "module/job")
	rejected := errors.New("candidate rejected")

	attempt, err := authority.start(context.Background(), jobmgr.ProcessAttemptPlan{
		Identity: identity,
		Work: func(context.Context, jobmgr.ProcessAttemptAdmission) error {
			return rejected
		},
	})
	require.NoError(t, err)
	require.ErrorIs(t, attempt.Await(context.Background()), rejected)
	<-attempt.Released()
	require.Equal(t, Census{}, authority.Census())

	successor, err := authority.start(context.Background(), jobmgr.ProcessAttemptPlan{
		Identity: identity,
		Work:     func(context.Context, jobmgr.ProcessAttemptAdmission) error { return nil },
	})
	require.NoError(t, err)
	require.NoError(t, successor.Await(context.Background()))
	<-successor.Released()
}

func TestAuthorityRetainedPreAdmissionErrorQuarantinesOnlyIdentity(t *testing.T) {
	authority := newTestAuthority(t, time.Second, 10*time.Millisecond, nil)
	identity := testIdentity(jobmgr.ProcessAttemptJob, "module/job", "module/job")
	retained := lifecycle.RetainOwnership(errors.New("candidate cleanup failed"))

	attempt, err := authority.start(context.Background(), jobmgr.ProcessAttemptPlan{
		Identity: identity,
		Work: func(context.Context, jobmgr.ProcessAttemptAdmission) error {
			return retained
		},
	})
	require.NoError(t, err)
	require.ErrorIs(t, attempt.Await(context.Background()), jobmgr.ErrProcessAttemptQuarantined)
	require.False(t, lifecycle.OwnershipRetained(attempt.Await(context.Background())))
	<-attempt.Released()
	require.Equal(t, Census{Quarantined: 1}, authority.Census())

	_, err = authority.start(context.Background(), jobmgr.ProcessAttemptPlan{
		Identity: identity,
		Work:     func(context.Context, jobmgr.ProcessAttemptAdmission) error { return nil },
	})
	require.ErrorIs(t, err, jobmgr.ErrProcessAttemptQuarantined)

	other, err := authority.start(context.Background(), jobmgr.ProcessAttemptPlan{
		Identity: testIdentity(jobmgr.ProcessAttemptJob, "module/other", "module/other"),
		Work:     func(context.Context, jobmgr.ProcessAttemptAdmission) error { return nil },
	})
	require.NoError(t, err)
	require.NoError(t, other.Await(context.Background()))
	<-other.Released()
	require.Equal(t, Census{Quarantined: 1}, authority.Census())

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, authority.Shutdown(ctx))
}

func TestAuthorityAdmittedTerminalErrorQuarantinesBeforeRelease(t *testing.T) {
	diagnostics := &recordingAttemptDiagnostics{}
	authority := newTestAuthority(t, time.Second, 10*time.Millisecond, diagnostics)
	identity := testIdentity(jobmgr.ProcessAttemptJobRuntime, "module/job", "module/job")
	admitted := make(chan error, 1)
	release := make(chan struct{})

	attempt, err := authority.start(context.Background(), jobmgr.ProcessAttemptPlan{
		Identity: identity,
		Work: func(_ context.Context, admission jobmgr.ProcessAttemptAdmission) error {
			admitted <- admission.Admit()
			<-release
			return errors.New("provider-sensitive-cleanup")
		},
	})
	require.NoError(t, err)
	require.NoError(t, <-admitted)
	require.Equal(t, Census{Active: 1, Admitted: 1}, authority.Census())

	close(release)
	<-attempt.Released()
	require.ErrorIs(t, attempt.Await(context.Background()), jobmgr.ErrProcessAttemptQuarantined)
	require.Equal(t, Census{Quarantined: 1}, authority.Census())

	_, err = authority.start(context.Background(), jobmgr.ProcessAttemptPlan{
		Identity: identity,
		Work:     func(context.Context, jobmgr.ProcessAttemptAdmission) error { return nil },
	})
	require.ErrorIs(t, err, jobmgr.ErrProcessAttemptQuarantined)
	require.NotContains(t, diagnostics.String(), "provider-sensitive-cleanup")
}

func TestAuthorityAdmissionAfterCutReturnsTerminalDisposition(t *testing.T) {
	authority := newTestAuthority(t, time.Second, 10*time.Millisecond, nil)
	entered := make(chan struct{})
	proceed := make(chan struct{})
	admitted := make(chan error, 1)

	attempt, err := authority.start(context.Background(), jobmgr.ProcessAttemptPlan{
		Identity: testIdentity(jobmgr.ProcessAttemptJobRuntime, "module/job", "module/job"),
		Work: func(_ context.Context, admission jobmgr.ProcessAttemptAdmission) error {
			close(entered)
			<-proceed
			err := admission.Admit()
			admitted <- err
			return err
		},
	})
	require.NoError(t, err)
	<-entered
	require.True(t, attempt.Cut(jobmgr.ErrProcessAttemptRetired))
	close(proceed)

	require.ErrorIs(t, <-admitted, jobmgr.ErrProcessAttemptRetired)
	<-attempt.Released()
	require.ErrorIs(t, attempt.Await(context.Background()), jobmgr.ErrProcessAttemptRetired)
}

func TestAuthorityContainedAdmittedTerminalErrorQuarantinesOnRelease(t *testing.T) {
	authority := newTestAuthority(t, time.Second, 10*time.Millisecond, nil)
	identity := testIdentity(jobmgr.ProcessAttemptFunctionBundle, "module/agent", "module")
	admitted := make(chan error, 1)
	release := make(chan struct{})

	attempt, err := authority.start(context.Background(), jobmgr.ProcessAttemptPlan{
		Identity: identity,
		Work: func(_ context.Context, admission jobmgr.ProcessAttemptAdmission) error {
			admitted <- admission.Admit()
			<-release
			return errors.New("cleanup failed")
		},
	})
	require.NoError(t, err)
	require.NoError(t, <-admitted)
	require.True(t, attempt.Cut(jobmgr.ErrProcessAttemptRetired))
	require.ErrorIs(t, attempt.Await(context.Background()), jobmgr.ErrProcessAttemptRetired)
	close(release)
	<-attempt.Released()
	require.Equal(t, Census{Quarantined: 1}, authority.Census())

	_, err = authority.start(context.Background(), jobmgr.ProcessAttemptPlan{
		Identity: identity,
		Work:     func(context.Context, jobmgr.ProcessAttemptAdmission) error { return nil },
	})
	require.ErrorIs(t, err, jobmgr.ErrProcessAttemptQuarantined)
}

func TestAuthorityShutdownReportsBoundedRetainedSample(t *testing.T) {
	diagnostics := &recordingAttemptDiagnostics{}
	authority := newTestAuthority(t, time.Second, 10*time.Millisecond, diagnostics)
	releases := make([]chan struct{}, 0, 12)
	attempts := make([]*attempt, 0, 12)
	for index := range 12 {
		release := make(chan struct{})
		releases = append(releases, release)
		attempt, err := authority.start(context.Background(), jobmgr.ProcessAttemptPlan{
			Identity: testIdentity(
				jobmgr.ProcessAttemptFunctionInvocation,
				fmt.Sprintf("private-%d", index),
				fmt.Sprintf("module/job/%d", index),
			),
			Work: func(context.Context, jobmgr.ProcessAttemptAdmission) error {
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

func testIdentity(
	namespace jobmgr.ProcessAttemptNamespace,
	key, resource string,
) jobmgr.ProcessAttemptIdentity {
	return jobmgr.ProcessAttemptIdentity{
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
