package jobmgrtest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/policy"
)

func runAgentStartAcknowledgement(ctx context.Context) error {
	return errors.Join(
		runAgentStartAcknowledgementVariant(ctx, false),
		runAgentStartAcknowledgementVariant(ctx, true),
	)
}

func runAgentStartAcknowledgementVariant(ctx context.Context, v2 bool) error {
	release := make(chan struct{})
	entered := make(chan struct{})
	state := &agentFixtureState{
		checkGate:    release,
		checkEntered: entered,
	}
	fixture, err := startAgentInstanceFixtureWithState(ctx, v2, state)
	if err != nil {
		return err
	}
	defer fixture.close()
	released := false
	defer func() {
		if !released {
			close(release)
		}
		_ = fixture.input.Close()
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		return ctx.Err()
	}
	if fixture.output.contains(`FUNCTION GLOBAL "jobmgrtest:echo"`) || fixture.output.contains(" status running") {
		return fmt.Errorf("Start-dependent publication preceded Check acknowledgement: %s", fixture.output.String())
	}
	close(release)
	released = true
	if err := waitUntil(ctx, func() bool {
		return fixture.output.contains(`FUNCTION GLOBAL "jobmgrtest:echo"`) &&
			fixture.output.contains("CONFIG jobmgrtest:collector:jobmgrtest create running single ")
	}); err != nil {
		return fmt.Errorf(
			"Start acknowledgement did not publish running generation: %w; output=%s",
			err,
			fixture.output.String(),
		)
	}
	return fixture.terminate(ctx)
}

func runAgentStartReplacementOrdering(ctx context.Context) error {
	return errors.Join(
		runAgentStartReplacementOrderingVariant(ctx, false),
		runAgentStartReplacementOrderingVariant(ctx, true),
	)
}

func runAgentStartReplacementOrderingVariant(ctx context.Context, v2 bool) error {
	release := make(chan struct{})
	entered := make(chan struct{})
	state := &agentFixtureState{
		checkGate:    release,
		checkEntered: entered,
	}
	fixture, err := startAgentInstanceFixtureWithState(ctx, v2, state)
	if err != nil {
		return err
	}
	defer fixture.close()
	released := false
	defer func() {
		if !released {
			close(release)
		}
		_ = fixture.input.Close()
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		return ctx.Err()
	}
	const uid = "jobmgrtest-held-start-disable"
	if err := writeFunctionCall(fixture.input, uid, "config jobmgrtest:collector:jobmgrtest disable"); err != nil {
		return err
	}
	time.Sleep(50 * time.Millisecond)
	if _, completed, parseErr := fixture.output.functionResult(uid); parseErr != nil {
		return parseErr
	} else if completed {
		return errors.New("disable completed before held Start acknowledgement")
	}
	if state.count("init") != 1 || state.count("cleanup") != 0 {
		return fmt.Errorf("held Start ownership changed before acknowledgement: %v", state.snapshot())
	}
	close(release)
	released = true
	result, err := waitFunctionResult(ctx, fixture.output, uid)
	if err != nil {
		return fmt.Errorf(
			"disable after held Start failed: %w; events=%v; output=%s",
			err,
			state.snapshot(),
			fixture.output.String(),
		)
	}
	if result.status != 200 {
		return fmt.Errorf(
			"disable after held Start status=%d, want 200; payload=%s; events=%v; output=%s",
			result.status,
			result.payload,
			state.snapshot(),
			fixture.output.String(),
		)
	}
	events := state.snapshot()
	if indexOf(events, "cleanup", 0) < 0 || state.count("init") != 1 || state.count("check") != 1 {
		return fmt.Errorf("disable did not join exactly one held Start and Cleanup: %v", events)
	}
	return fixture.terminate(ctx)
}

func runAgentBlockingStop(ctx context.Context) error {
	release := make(chan struct{})
	entered := make(chan struct{})
	state := &agentFixtureState{
		cleanupGate:    release,
		cleanupEntered: entered,
	}
	fixture, err := startAgentFixtureWithState(ctx, false, state)
	if err != nil {
		return err
	}
	defer fixture.close()
	released := false
	defer func() {
		if !released {
			close(release)
		}
		_ = fixture.input.Close()
	}()
	if err := waitUntil(ctx, func() bool {
		return state.count("check") == 1
	}); err != nil {
		return err
	}
	stopped := make(chan error, 1)
	go func() { stopped <- fixture.terminate(ctx) }()
	select {
	case <-entered:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-stopped:
		return fmt.Errorf("Agent reported terminal success with live Stop: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	released = true
	select {
	case err := <-stopped:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func runAgentHeldHandlerTerminal(ctx context.Context) error {
	release := make(chan struct{})
	entered := make(chan struct{})
	state := &agentFixtureState{
		handleGate:    release,
		handleEntered: entered,
	}
	fixture, err := startAgentFixtureWithState(ctx, false, state)
	if err != nil {
		return err
	}
	defer fixture.close()
	released := false
	defer func() {
		if !released {
			close(release)
		}
		_ = fixture.input.Close()
	}()
	if err := waitUntil(ctx, func() bool {
		return fixture.output.contains(`FUNCTION GLOBAL "jobmgrtest:echo"`)
	}); err != nil {
		return err
	}
	const uid = "jobmgrtest-held-handler"
	if _, err := io.WriteString(
		fixture.input,
		"FUNCTION "+uid+
			" 30 \"jobmgrtest:echo held\" 0xFFFF \"method=api,role=test\"\n",
	); err != nil {
		return err
	}
	select {
	case <-entered:
	case <-ctx.Done():
		return ctx.Err()
	}
	stopped := make(chan error, 1)
	go func() { stopped <- fixture.terminate(ctx) }()
	select {
	case err := <-stopped:
		return fmt.Errorf("Agent returned with live handler: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	released = true
	select {
	case err := <-stopped:
		if err != nil {
			return err
		}
	case <-ctx.Done():
		return ctx.Err()
	}
	if fixture.output.contains("FUNCTION_RESULT_BEGIN " + uid + " 200 ") {
		return errors.New("old-generation handler committed a frame after shutdown quarantine")
	}
	return nil
}

func runAgentFunctionAdmissionClosesBeforeLeaseDrain(ctx context.Context) error {
	release := make(chan struct{})
	entered := make(chan struct{})
	state := &agentFixtureState{
		handleGate:    release,
		handleEntered: entered,
	}
	fixture, err := startAgentInstanceFixtureWithState(ctx, false, state)
	if err != nil {
		return err
	}
	defer fixture.close()
	released := false
	defer func() {
		if !released {
			close(release)
		}
		_ = fixture.input.Close()
	}()
	if err := waitUntil(ctx, func() bool {
		return fixture.output.contains(`FUNCTION GLOBAL "jobmgrtest:echo"`)
	}); err != nil {
		return err
	}
	const firstUID = "jobmgrtest-lease-active"
	if err := writeFunctionCall(fixture.input, firstUID, "jobmgrtest:echo active"); err != nil {
		return err
	}
	select {
	case <-entered:
	case <-ctx.Done():
		return ctx.Err()
	}
	const disableUID = "jobmgrtest-lease-disable"
	if err := writeFunctionCall(
		fixture.input,
		disableUID,
		"config jobmgrtest:collector:jobmgrtest disable",
	); err != nil {
		return err
	}
	if err := waitUntil(ctx, func() bool {
		return fixture.output.contains(`FUNCTION_DEL GLOBAL "jobmgrtest:echo"`)
	}); err != nil {
		return fmt.Errorf(
			"job stop did not close Function admission: %w; events=%v; output=%s",
			err,
			state.snapshot(),
			fixture.output.String(),
		)
	}
	const laterUID = "jobmgrtest-lease-later"
	if err := writeFunctionCall(fixture.input, laterUID, "jobmgrtest:echo later"); err != nil {
		return err
	}
	result, err := waitFunctionResult(ctx, fixture.output, laterUID)
	if err != nil {
		return err
	}
	if result.status != 503 {
		return fmt.Errorf("post-close invocation status=%d, want 503", result.status)
	}
	if state.count("raw:echo") != 1 {
		return fmt.Errorf("post-close invocation entered the handler: %v", state.snapshot())
	}
	if _, completed, parseErr := fixture.output.functionResult(disableUID); parseErr != nil {
		return parseErr
	} else if completed {
		return errors.New("disable completed before the active Function lease drained")
	}
	close(release)
	released = true
	first, err := waitFunctionResult(ctx, fixture.output, firstUID)
	if err != nil {
		return err
	}
	if first.status != 200 {
		return fmt.Errorf("active invocation status=%d, want 200", first.status)
	}
	disabled, err := waitFunctionResult(ctx, fixture.output, disableUID)
	if err != nil {
		return err
	}
	if disabled.status != 200 || state.count("cleanup") != 1 || state.count("handler-cleanup") != 1 {
		return fmt.Errorf(
			"disable did not drain and clean the exact generation: status=%d events=%v",
			disabled.status,
			state.snapshot(),
		)
	}
	return fixture.terminate(ctx)
}

func runAgentFunctionReplacementOrdering(ctx context.Context) error {
	release := make(chan struct{})
	entered := make(chan struct{})
	state := &agentFixtureState{
		handleGate:    release,
		handleEntered: entered,
	}
	fixture, err := startAgentInstanceFixtureWithState(ctx, false, state)
	if err != nil {
		return err
	}
	defer fixture.close()
	released := false
	defer func() {
		if !released {
			close(release)
		}
		_ = fixture.input.Close()
	}()
	const functionPublication = `FUNCTION GLOBAL "jobmgrtest:echo"`
	const initialPublication = "CONFIG jobmgrtest:collector:jobmgrtest create running single "
	const runningPublication = "CONFIG jobmgrtest:collector:jobmgrtest status running"
	if err := waitUntil(ctx, func() bool {
		return fixture.output.contains(functionPublication) && fixture.output.contains(initialPublication)
	}); err != nil {
		return fmt.Errorf(
			"initial generation was not published: %w; events=%v; output=%s",
			err,
			state.snapshot(),
			fixture.output.String(),
		)
	}
	initialFunctions := fixture.output.count(functionPublication)
	initialRunning := fixture.output.count(initialPublication)

	const oldUID = "jobmgrtest-replace-old"
	if err := writeFunctionCall(fixture.input, oldUID, "jobmgrtest:echo old"); err != nil {
		return err
	}
	select {
	case <-entered:
	case <-ctx.Done():
		return ctx.Err()
	}

	const updateUID = "jobmgrtest-replace-update"
	if _, err := writeAgentRawFunctionPayload(
		fixture.input,
		updateUID,
		"config jobmgrtest:collector:jobmgrtest update",
		"application/json",
		[]byte(`{}`),
	); err != nil {
		return err
	}
	if err := waitUntil(ctx, func() bool {
		return fixture.output.contains(`FUNCTION_DEL GLOBAL "jobmgrtest:echo"`)
	}); err != nil {
		return fmt.Errorf(
			"update did not withdraw the old Function generation: %w; events=%v; output=%s",
			err,
			state.snapshot(),
			fixture.output.String(),
		)
	}
	if fixture.output.count(functionPublication) != initialFunctions ||
		fixture.output.count(initialPublication) != initialRunning ||
		fixture.output.contains(runningPublication) {
		return fmt.Errorf(
			"replacement published before the old handler drained: events=%v output=%s",
			state.snapshot(),
			fixture.output.String(),
		)
	}

	close(release)
	released = true
	old, err := waitFunctionResult(ctx, fixture.output, oldUID)
	if err != nil {
		return fmt.Errorf(
			"old invocation did not complete after release: %w; events=%v; output=%s",
			err,
			state.snapshot(),
			fixture.output.String(),
		)
	}
	if old.status != 200 {
		return fmt.Errorf("old invocation status=%d, want 200", old.status)
	}
	updated, err := waitFunctionResult(ctx, fixture.output, updateUID)
	if err != nil {
		return fmt.Errorf(
			"update did not complete after old generation drain: %w; events=%v; output=%s",
			err,
			state.snapshot(),
			fixture.output.String(),
		)
	}
	if updated.status != 200 {
		return fmt.Errorf("update status=%d, want 200; payload=%s", updated.status, updated.payload)
	}
	if err := waitUntil(ctx, func() bool {
		return fixture.output.count(functionPublication) ==
			initialFunctions+1 &&
			fixture.output.count(initialPublication) ==
				initialRunning+1
	}); err != nil {
		return fmt.Errorf(
			"successor generation was not published: %w; events=%v; output=%s",
			err,
			state.snapshot(),
			fixture.output.String(),
		)
	}
	events := state.snapshot()
	returned := indexOf(events, "raw:echo:returned", 0)
	handlerCleanup := indexOf(events, "handler-cleanup", 0)
	runtimeCleanup := -1
	for index := returned + 1; index < len(events); index++ {
		if events[index] == "cleanup" {
			runtimeCleanup = index
			break
		}
	}
	if returned < 0 || handlerCleanup <= returned || runtimeCleanup <= returned {
		return fmt.Errorf("new running publication preceded old generation drain: %v", events)
	}

	const successorUID = "jobmgrtest-replace-successor"
	if err := writeFunctionCall(fixture.input, successorUID, "jobmgrtest:echo successor"); err != nil {
		return err
	}
	successor, err := waitFunctionResult(ctx, fixture.output, successorUID)
	if err != nil {
		return fmt.Errorf(
			"successor invocation did not complete: %w; events=%v; output=%s",
			err,
			state.snapshot(),
			fixture.output.String(),
		)
	}
	if successor.status != 200 || state.count("raw:echo") != 2 {
		return fmt.Errorf(
			"successor generation did not own the next invocation: status=%d events=%v",
			successor.status,
			state.snapshot(),
		)
	}
	return fixture.terminate(ctx)
}

const (
	formerOperationPopulation = 256
	formerSameRoutePopulation = 32
)

func runAgentUIDGrowthBeyondFormerLimit(ctx context.Context) error {
	return runAgentHeldFunctionPopulation(
		ctx,
		formerOperationPopulation+1,
		func(int) string { return "jobmgrtest:echo held" },
	)
}

func runAgentConcurrentSameRouteFunctionPopulation(ctx context.Context) error {
	return runAgentHeldFunctionPopulation(
		ctx,
		formerSameRoutePopulation+1,
		func(int) string { return "jobmgrtest:echo held" },
	)
}

func runAgentHeldFunctionPopulation(ctx context.Context, admitted int, route func(int) string) error {
	release := make(chan struct{})
	entered := make(chan struct{})
	state := &agentFixtureState{
		handleGate:    release,
		handleEntered: entered,
	}
	fixture, err := startAgentCapacityFixtureWithState(ctx, state)
	if err != nil {
		return err
	}
	defer fixture.close()
	released := false
	defer func() {
		if !released {
			close(release)
		}
		_ = fixture.input.Close()
	}()
	if err := waitUntil(ctx, func() bool {
		return fixture.output.contains(`FUNCTION GLOBAL "jobmgrtest:echo"`)
	}); err != nil {
		return err
	}
	for index := range admitted {
		if err := writeFunctionCall(
			fixture.input,
			fmt.Sprintf("jobmgrtest-held-%03d", index),
			route(index),
		); err != nil {
			return err
		}
	}
	if err := waitUntil(ctx, func() bool {
		return state.count("raw:echo:entered") >= admitted
	}); err != nil {
		return fmt.Errorf(
			"same-route handlers entered=%d, want at least %d before release: %w",
			state.count("raw:echo:entered"),
			admitted,
			err,
		)
	}
	close(release)
	released = true
	if err := waitUntil(ctx, func() bool {
		return fixture.output.count("FUNCTION_RESULT_BEGIN jobmgrtest-held-") >= admitted
	}); err != nil {
		return err
	}
	statuses, err := heldFunctionStatuses(fixture.output.snapshot())
	if err != nil {
		return err
	}
	for index := range admitted {
		uid := fmt.Sprintf("jobmgrtest-held-%03d", index)
		if statuses[uid] != 200 {
			return fmt.Errorf("admitted Function %s status=%d, want 200", uid, statuses[uid])
		}
	}
	return fixture.terminate(ctx)
}

func heldFunctionStatuses(output []byte) (map[string]int, error) {
	statuses := make(map[string]int)
	for line := range bytes.SplitSeq(output, []byte{'\n'}) {
		fields := bytes.Fields(line)
		if len(fields) < 4 ||
			string(fields[0]) != "FUNCTION_RESULT_BEGIN" ||
			!bytes.HasPrefix(fields[1], []byte("jobmgrtest-held-")) {
			continue
		}
		status, err := strconv.Atoi(string(fields[2]))
		if err != nil {
			return nil, fmt.Errorf("invalid held Function status %q: %w", fields[2], err)
		}
		statuses[string(fields[1])] = status
	}
	return statuses, nil
}

func runAgentControlWithLargeOrdinaryPopulation(ctx context.Context) error {
	release := make(chan struct{})
	state := &agentFixtureState{
		handleGate:          release,
		handleCancelReturns: true,
	}
	fixture, err := startAgentCapacityFixtureWithState(ctx, state)
	if err != nil {
		return err
	}
	defer fixture.close()
	defer fixture.input.Close()
	if err := waitUntil(ctx, func() bool {
		return fixture.output.contains(`FUNCTION GLOBAL "jobmgrtest:echo"`)
	}); err != nil {
		return err
	}
	for index := 0; index <= formerOperationPopulation; index++ {
		if err := writeFunctionCall(
			fixture.input,
			fmt.Sprintf("jobmgrtest-control-%03d", index),
			"jobmgrtest:echo held",
		); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(fixture.input, "FUNCTION_CANCEL jobmgrtest-control-000\nQUIT\n"); err != nil {
		return err
	}
	if runErr := fixture.wait(ctx); runErr != nil {
		if errors.Is(runErr, context.DeadlineExceeded) || errors.Is(runErr, context.Canceled) {
			return errors.New("CANCEL and QUIT did not progress with a large ordinary population")
		}
		return runErr
	}
	if state.count("raw:echo:cancelled") == 0 {
		return errors.New("CANCEL did not reach a live ordinary Function before QUIT")
	}
	return nil
}

func runAgentFunctionStatusVariants(ctx context.Context) error {
	return runAgentFunctionFlow(ctx, true)
}

type outputFaultCut uint8

const (
	outputFaultFunction outputFaultCut = iota + 1
	outputFaultConfig
	outputFaultV1
	outputFaultV2
	outputFaultRuntime
	outputFaultCleanup
)

type injectedWriteMode uint8

const (
	injectedShortNil injectedWriteMode = iota + 1
	injectedShortError
	injectedPreByteError
	injectedEPIPE
)

type outputFaultWaitResult uint8

const (
	outputFaultInjectionObserved outputFaultWaitResult = iota + 1
	outputFaultFixtureTerminated
	outputFaultContextDone
)

type predicateFaultWriter struct {
	target   *synchronizedBuffer
	match    func([]byte) bool
	mode     injectedWriteMode
	injected chan struct{}
	once     sync.Once
}

func (writer *predicateFaultWriter) Write(payload []byte) (int, error) {
	if !writer.match(payload) {
		return writer.target.Write(payload)
	}
	var count int
	var err error
	writer.once.Do(func() {
		switch writer.mode {
		case injectedShortNil:
			count = len(payload) / 2
			if count == 0 {
				count = 1
			}
			_, _ = writer.target.Write(payload[:count])
		case injectedShortError:
			count = len(payload) / 2
			if count == 0 {
				count = 1
			}
			_, _ = writer.target.Write(payload[:count])
			err = errors.New("injected short write")
		case injectedPreByteError:
			err = errors.New("injected pre-byte write failure")
		case injectedEPIPE:
			err = syscall.EPIPE
		}
		close(writer.injected)
	})
	if count == 0 && err == nil {
		return writer.target.Write(payload)
	}
	return count, err
}

func runAgentOutputFaultCut(ctx context.Context, cut outputFaultCut) error {
	modes := map[string]injectedWriteMode{
		"short nil":      injectedShortNil,
		"short error":    injectedShortError,
		"pre-byte error": injectedPreByteError,
		"EPIPE":          injectedEPIPE,
	}
	for name, mode := range modes {
		if err := runAgentOutputFaultMode(ctx, cut, mode); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

func runAgentOutputFaultMode(ctx context.Context, cut outputFaultCut, mode injectedWriteMode) error {
	state := &agentFixtureState{
		emitCharts: cut == outputFaultV1 ||
			cut == outputFaultV2 ||
			cut == outputFaultRuntime,
	}
	injected := make(chan struct{})
	match := outputFaultMatcher(cut)
	v2 := cut == outputFaultV2 || cut == outputFaultRuntime
	runPolicy := policy.Agent(true)
	if cut == outputFaultRuntime {
		runPolicy.EnableRuntimeCharts = true
	}
	registry := fixtureRegistry(state, v2)
	if state.emitCharts {
		registry = fixtureOutputRegistry(state, v2)
	}
	fixture, err := startAgentFixtureConfiguredWithRegistry(
		ctx,
		state,
		registry,
		func(output *synchronizedBuffer) io.Writer {
			return &predicateFaultWriter{
				target:   output,
				match:    match,
				mode:     mode,
				injected: injected,
			}
		},
		runPolicy,
	)
	if err != nil {
		return err
	}
	defer fixture.close()
	defer fixture.input.Close()
	switch cut {
	case outputFaultFunction:
		if err := waitUntil(ctx, func() bool {
			return fixture.output.contains(`FUNCTION GLOBAL "jobmgrtest:echo"`)
		}); err != nil {
			return err
		}
		if err := writeFunctionCall(fixture.input, "jobmgrtest-fault", "jobmgrtest:echo fault"); err != nil {
			return err
		}
	case outputFaultCleanup:
		if err := waitUntil(ctx, func() bool {
			return fixture.output.contains(`FUNCTION GLOBAL "jobmgrtest:echo"`)
		}); err != nil {
			return err
		}
		go func() {
			terminationCtx, cancel := context.WithTimeout(context.Background(), fixtureJoinPeriod)
			defer cancel()
			_ = fixture.terminate(terminationCtx)
		}()
	}
	switch waitForOutputFault(injected, fixture.done, ctx.Done()) {
	case outputFaultInjectionObserved:
	case outputFaultFixtureTerminated:
		runErr := fixture.wait(context.Background())
		return fmt.Errorf(
			"output fault cut %d terminated before injection: %w; events=%v; output=%s",
			cut,
			runErr,
			state.snapshot(),
			fixture.output.String(),
		)
	case outputFaultContextDone:
		return fmt.Errorf(
			"output fault cut %d was not reached: %w; events=%v; output=%s",
			cut,
			ctx.Err(),
			state.snapshot(),
			fixture.output.String(),
		)
	default:
		return errors.New("invalid output fault wait result")
	}
	runErr := fixture.wait(ctx)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if runErr == nil {
		return errors.New("output fault produced a clean Agent disposition")
	}
	return nil
}

func waitForOutputFault(
	injected <-chan struct{},
	fixtureDone <-chan struct{},
	contextDone <-chan struct{},
) outputFaultWaitResult {
	// The writer records injection before its error can terminate the fixture.
	// Rechecking makes that causal order win when both channels are ready.
	if outputFaultInjectionReady(injected) {
		return outputFaultInjectionObserved
	}
	select {
	case <-injected:
		return outputFaultInjectionObserved
	case <-fixtureDone:
		if outputFaultInjectionReady(injected) {
			return outputFaultInjectionObserved
		}
		return outputFaultFixtureTerminated
	case <-contextDone:
		if outputFaultInjectionReady(injected) {
			return outputFaultInjectionObserved
		}
		return outputFaultContextDone
	}
}

func outputFaultInjectionReady(injected <-chan struct{}) bool {
	select {
	case <-injected:
		return true
	default:
		return false
	}
}

func outputFaultMatcher(cut outputFaultCut) func([]byte) bool {
	return func(payload []byte) bool {
		frame := string(payload)
		switch cut {
		case outputFaultFunction:
			return strings.Contains(frame, "FUNCTION_RESULT_BEGIN jobmgrtest-fault ")
		case outputFaultConfig:
			return strings.HasPrefix(frame, "CONFIG ")
		case outputFaultV1:
			return strings.Contains(frame, "CHART 'jobmgrtest.")
		case outputFaultV2:
			return strings.Contains(frame, "CHART 'jobmgrtest.")
		case outputFaultRuntime:
			return strings.Contains(frame, ".internal.")
		case outputFaultCleanup:
			return strings.Contains(frame, `FUNCTION_DEL GLOBAL "jobmgrtest:echo"`)
		default:
			return false
		}
	}
}

func writeFunctionCall(writer io.Writer, uid string, call string) error {
	_, err := io.WriteString(writer, fmt.Sprintf("FUNCTION %s 30 %q 0xFFFF %q\n", uid, call, "method=api,role=test"))
	return err
}
