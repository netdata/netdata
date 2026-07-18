package wireeval

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/contract"
	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/observation"
	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/oracle"
	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/protocol"
	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/reducer"
	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/runner"
)

type ChildSpec struct {
	Executable string
	Arguments  []string
	CutControl bool
}

type RunSpec struct {
	Child             ChildSpec
	Side              oracle.Side
	WorkloadID        string
	Population        int
	PairIndex         int
	RanFirst          bool
	PairNonce         [16]byte
	EnvironmentSHA256 string
	Schedule          *contract.PerformanceSchedule
	ObservationSink   func(oracle.RunObservation) error
}

type waitOutcome struct {
	result runner.Result
	err    error
}

func Run(ctx context.Context, spec RunSpec) (oracle.RunSummary, error) {
	workload, err := findWorkload(spec.WorkloadID)
	if err != nil {
		return oracle.RunSummary{}, err
	}
	if spec.Population != 1 && spec.Population != 32 && spec.Population != 256 {
		return oracle.RunSummary{}, errors.New("wire evaluator: invalid population")
	}
	if !filepath.IsAbs(spec.Child.Executable) {
		return oracle.RunSummary{}, errors.New("wire evaluator: child executable must be absolute")
	}
	if err := spec.Schedule.Validate(spec.WorkloadID, spec.PairNonce); err != nil {
		return oracle.RunSummary{}, fmt.Errorf("wire evaluator: %w", err)
	}
	eventReader, eventWriter, err := os.Pipe()
	if err != nil {
		return oracle.RunSummary{}, err
	}
	defer eventReader.Close()
	var cutControlReader, cutControlWriter *os.File
	if spec.Child.CutControl {
		cutControlReader, cutControlWriter, err = os.Pipe()
		if err != nil {
			_ = eventWriter.Close()
			return oracle.RunSummary{}, err
		}
		defer cutControlWriter.Close()
	}

	decoder, err := observation.NewFunctionResultDecoder(8 * 1024)
	if err != nil {
		return oracle.RunSummary{}, err
	}
	results := make(chan observation.FunctionResult, 2*spec.Population)
	observerErrors := make(chan error, 1)
	ready := make(chan struct{})
	var setup setupTracker
	arguments := append([]string(nil), spec.Child.Arguments...)
	arguments = append(arguments, "--event-fd=3", "--process-generation=1")
	extraFiles := []*os.File{eventWriter}
	if cutControlReader != nil {
		arguments = append(arguments, "--cut-control-fd=4")
		extraFiles = append(extraFiles, cutControlReader)
	}
	lowerWall := time.Now().Unix()
	process, err := runner.Start(runner.Spec{
		Executable:  spec.Child.Executable,
		Arguments:   arguments,
		StderrLimit: 1024 * 1024,
		ExtraFiles:  extraFiles,
		ObserveOut: func(chunk []byte, readReturnMonoNS int64) error {
			parsed, skipped, err := decoder.Feed(chunk, readReturnMonoNS)
			if err != nil {
				select {
				case observerErrors <- err:
				default:
				}
				return err
			}
			becameReady, err := setup.Feed(skipped)
			if err != nil {
				select {
				case observerErrors <- err:
				default:
				}
				return err
			}
			if becameReady {
				close(ready)
			}
			for _, result := range parsed {
				select {
				case results <- result:
				default:
					return errors.New("wire evaluator: result stream exceeds bounded in-flight capacity")
				}
			}
			return nil
		},
	})
	if err != nil {
		_ = eventWriter.Close()
		if cutControlReader != nil {
			_ = cutControlReader.Close()
		}
		return oracle.RunSummary{}, err
	}
	defer process.Kill()
	if err := eventWriter.Close(); err != nil {
		return oracle.RunSummary{}, err
	}
	if cutControlReader != nil {
		if err := cutControlReader.Close(); err != nil {
			return oracle.RunSummary{}, err
		}
		cutControlReader = nil
		if err := cutControlWriter.Close(); err != nil {
			return oracle.RunSummary{}, err
		}
		cutControlWriter = nil
	}
	exit := make(chan waitOutcome, 1)
	go func() {
		result, err := process.Wait(ctx)
		exit <- waitOutcome{result: result, err: err}
	}()

	events := make(chan observation.PassiveEvent, 2*spec.Population)
	rawEvents := make(chan protocol.Event, 2*spec.Population)
	eventReadDone := make(chan error, 1)
	eventDone := make(chan error, 1)
	go func() {
		defer close(rawEvents)
		for {
			message, err := protocol.ReadEvent(eventReader)
			if err != nil {
				if errors.Is(err, io.EOF) {
					eventReadDone <- nil
				} else {
					eventReadDone <- err
				}
				return
			}
			if err := validatePerformanceEventKind(message.Kind); err != nil {
				eventReadDone <- err
				_ = process.Kill()
				return
			}
			select {
			case rawEvents <- message:
			default:
				eventReadDone <- errors.New("wire evaluator: raw event stream exceeds bounded in-flight capacity")
				_ = process.Kill()
				return
			}
		}
	}()
	go func() {
		var deliveryErr error
		for message := range rawEvents {
			if deliveryErr != nil {
				continue
			}
			observed, err := observation.NewPassiveEvent(message, process.Now())
			if err != nil {
				deliveryErr = err
				_ = process.Kill()
				continue
			}
			select {
			case events <- observed:
			default:
				deliveryErr = errors.New("wire evaluator: event stream exceeds bounded in-flight capacity")
				_ = process.Kill()
			}
		}
		eventDone <- errors.Join(deliveryErr, <-eventReadDone)
	}()

	select {
	case <-ready:
	case err := <-observerErrors:
		return oracle.RunSummary{}, err
	case err := <-eventDone:
		return oracle.RunSummary{}, errors.Join(errors.New("wire evaluator: event stream closed during setup"), err)
	case outcome := <-exit:
		return oracle.RunSummary{}, fmt.Errorf("wire evaluator: child exited during setup: %w; stderr=%s", outcome.err, outcome.result.Stderr)
	case <-ctx.Done():
		return oracle.RunSummary{}, fmt.Errorf(
			"wire evaluator: setup readiness %s: %w",
			setup.Status(),
			ctx.Err(),
		)
	}
	if err := drainSetup(ctx, process, spec.PairNonce, results, events, observerErrors, eventDone, exit); err != nil {
		return oracle.RunSummary{}, fmt.Errorf("wire evaluator: setup drain: %w", err)
	}

	reduced, err := reducer.NewPrepared(spec.Schedule)
	if err != nil {
		return oracle.RunSummary{}, err
	}
	nextSequence := 0
	completed := 0
	inflight := 0
	wantEvents := workload.ClassCounts[contract.ClassCancel] + workload.ClassCounts[contract.ClassDeadline]
	observedEvents := 0
	for completed < contract.PerformanceOperations || observedEvents < wantEvents {
		for nextSequence < contract.PerformanceOperations && inflight < spec.Population {
			operation := &spec.Schedule.Operations[nextSequence]
			t0, err := process.Write(operation.Request)
			if err != nil {
				return oracle.RunSummary{}, err
			}
			if err := reduced.Offer(observation.OfferedRequest{
				Sequence:         nextSequence,
				Class:            string([]byte{byte(operation.Class)}),
				Key:              operation.Key,
				UID:              operation.UID,
				RequestSHA256:    operation.RequestSHA256,
				FollowupSHA256:   operation.FollowupSHA256,
				UsefulWorkSHA256: operation.UsefulWorkSHA256,
				OfferedMonoNS:    t0,
			}); err != nil {
				return oracle.RunSummary{}, err
			}
			nextSequence++
			inflight++
		}
		select {
		case result := <-results:
			if err := reduced.ObserveResult(result); err != nil {
				return oracle.RunSummary{}, err
			}
			completed++
			inflight--
		case event := <-events:
			if err := reduced.ObserveEvent(event); err != nil {
				return oracle.RunSummary{}, err
			}
			if event.Message.Kind == observation.EventHandlerEntered || event.Message.Kind == observation.EventDeadlineObserved {
				observedEvents++
			}
			if event.Message.Kind == observation.EventHandlerEntered {
				sequence, exists := spec.Schedule.ByUID[event.Message.Token]
				if !exists {
					return oracle.RunSummary{}, fmt.Errorf(
						"wire evaluator: handler event for unknown schedule UID %s",
						event.Message.Token,
					)
				}
				operation := &spec.Schedule.Operations[sequence]
				if operation.Class != contract.ClassCancel || len(operation.Followup) == 0 {
					return oracle.RunSummary{}, fmt.Errorf(
						"wire evaluator: handler event for non-cancel sequence %d",
						sequence,
					)
				}
				followupAt, err := process.Write(operation.Followup)
				if err != nil {
					return oracle.RunSummary{}, err
				}
				if err := reduced.SetFollowup(event.Message.Token, followupAt); err != nil {
					return oracle.RunSummary{}, err
				}
			}
		case err := <-observerErrors:
			return oracle.RunSummary{}, err
		case err := <-eventDone:
			return oracle.RunSummary{}, errors.Join(errors.New("wire evaluator: event stream closed during workload"), err)
		case outcome := <-exit:
			return oracle.RunSummary{}, fmt.Errorf("wire evaluator: child exited during workload: %w; stderr=%s", outcome.err, outcome.result.Stderr)
		case <-ctx.Done():
			return oracle.RunSummary{}, fmt.Errorf(
				"wire evaluator: workload next=%d completed=%d inflight=%d events=%d/%d: %w",
				nextSequence,
				completed,
				inflight,
				observedEvents,
				wantEvents,
				ctx.Err(),
			)
		}
	}
	upperWall := time.Now().Unix()
	if _, err := process.WriteContext(ctx, []byte("QUIT\n")); err != nil {
		return oracle.RunSummary{}, err
	}
	if err := process.CloseInput(); err != nil {
		return oracle.RunSummary{}, err
	}
	outcome := <-exit
	if outcome.err != nil {
		return oracle.RunSummary{}, fmt.Errorf("wire evaluator: child exit: %w; stderr=%s", outcome.err, outcome.result.Stderr)
	}
	if outcome.result.StderrTruncated {
		return oracle.RunSummary{}, fmt.Errorf(
			"wire evaluator: %s child stderr truncated after %d bytes: workload=%s population=%d pair=%d",
			spec.Side, outcome.result.StderrBytes, spec.WorkloadID, spec.Population, spec.PairIndex,
		)
	}
	select {
	case err := <-eventDone:
		if err != nil {
			return oracle.RunSummary{}, err
		}
	case <-ctx.Done():
		return oracle.RunSummary{}, fmt.Errorf("wire evaluator: final event drain: %w", ctx.Err())
	}
	if _, err := decoder.Finish(); err != nil {
		return oracle.RunSummary{}, err
	}
	if err := rejectSurplusEvidence(results, events, observerErrors); err != nil {
		return oracle.RunSummary{}, err
	}
	observation := oracle.RunObservation{
		WorkloadID: spec.WorkloadID, Population: spec.Population, PairIndex: spec.PairIndex,
		Side: spec.Side, RanFirst: spec.RanFirst, PairNonce: spec.PairNonce,
		EnvironmentSHA256: spec.EnvironmentSHA256, WallLowerUnix: lowerWall, WallUpperUnix: upperWall,
		Operations: reduced.Completed(),
	}
	if spec.ObservationSink != nil {
		if err := spec.ObservationSink(observation); err != nil {
			return oracle.RunSummary{}, err
		}
	}
	return oracle.AnalyzeRun(observation)
}

func validatePerformanceEventKind(kind protocol.EventKind) error {
	switch kind {
	case protocol.EventHandlerEntered, protocol.EventDeadlineObserved:
		return nil
	default:
		return fmt.Errorf("wire evaluator: unexpected performance event kind %q", kind)
	}
}

func rejectSurplusEvidence(results <-chan observation.FunctionResult, events <-chan observation.PassiveEvent, observerErrors <-chan error) error {
	if count := len(results); count != 0 {
		return fmt.Errorf("wire evaluator: %d trailing Function results after expected workload", count)
	}
	if count := len(events); count != 0 {
		return fmt.Errorf("wire evaluator: %d trailing passive events after expected workload", count)
	}
	select {
	case err := <-observerErrors:
		return err
	default:
		return nil
	}
}

func findWorkload(id string) (contract.PerformanceWorkload, error) {
	for _, workload := range contract.PerformanceWorkloads() {
		if workload.ID == id {
			return workload, nil
		}
	}
	return contract.PerformanceWorkload{}, fmt.Errorf("wire evaluator: unknown workload %q", id)
}

func drainSetup(
	ctx context.Context,
	process *runner.Process,
	pairNonce [16]byte,
	results <-chan observation.FunctionResult,
	events <-chan observation.PassiveEvent,
	observerErrors <-chan error,
	eventDone <-chan error,
	exit <-chan waitOutcome,
) error {
	want, err := contract.ExpectedResult(contract.ClassJobManager)
	if err != nil {
		return err
	}
	for ordinal := range contract.PerformanceFixtureCount {
		fixture, err := contract.Fixture(ordinal)
		if err != nil {
			return err
		}
		uid := contract.SetupUID(pairNonce, uint16(ordinal))
		request, err := contract.PerformanceRequest(contract.ClassJobManager, uid, fixture.KeyToken)
		if err != nil {
			return err
		}
		_, err = process.Write(request)
		if err != nil {
			return err
		}
		for {
			select {
			case result := <-results:
				if result.UID != uid || result.ReadReturnMonoNS < 0 || result.Status != want.Status ||
					result.ContentType != want.ContentType || !bytes.Equal(result.Payload, want.Payload) ||
					!bytes.Equal(result.Deferred, want.Deferred) {
					return fmt.Errorf("wire evaluator: setup drain result differs for %s", fixture.PublicConfigID)
				}
				goto nextFixture
			case event := <-events:
				return fmt.Errorf("wire evaluator: unexpected setup passive event %q", event.Message.Kind)
			case err := <-observerErrors:
				return err
			case err := <-eventDone:
				return errors.Join(errors.New("wire evaluator: event stream closed during setup drain"), err)
			case outcome := <-exit:
				return fmt.Errorf("wire evaluator: child exited during setup drain: %w; stderr=%s", outcome.err, outcome.result.Stderr)
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	nextFixture:
	}
	if count := len(results); count != 0 {
		return fmt.Errorf("wire evaluator: %d surplus results after setup drain", count)
	}
	if count := len(events); count != 0 {
		return fmt.Errorf("wire evaluator: %d surplus events after setup drain", count)
	}
	select {
	case err := <-observerErrors:
		return err
	default:
		return nil
	}
}

func digest(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
