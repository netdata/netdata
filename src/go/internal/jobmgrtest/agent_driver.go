package jobmgrtest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/agent"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/dummy"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/agent/policy"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	frameworkfunctions "github.com/netdata/netdata/go/plugins/plugin/framework/functions"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

const productionFixtureModule = "jobmgrtest"

// AgentDriver exercises one exact immutable production predicate through the
// shipped Agent API. Predicates are never cached or credited by family name.
type AgentDriver struct{}

func (driver *AgentDriver) Run(
	ctx context.Context,
	predicate string,
) error {
	if driver == nil || ctx == nil {
		return errors.New("jobmgr test: invalid Agent driver")
	}
	run := agentRuntimePredicates[predicate]
	if run == nil {
		return fmt.Errorf(
			"jobmgr test: unknown Agent predicate %q",
			predicate,
		)
	}
	return run(ctx)
}

func prefixError(prefix string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", prefix, err)
}

type agentFixtureState struct {
	mu                  sync.Mutex
	events              []string
	checkGate           <-chan struct{}
	checkEntered        chan struct{}
	checkOnce           sync.Once
	cleanupGate         <-chan struct{}
	cleanupEntered      chan struct{}
	cleanupOnce         sync.Once
	handleGate          <-chan struct{}
	handleEntered       chan struct{}
	handleOnce          sync.Once
	handleCancelReturns bool
	emitCharts          bool
	checkErr            error
}

func (state *agentFixtureState) cleanup() {
	state.record("cleanup")
	if state.cleanupEntered != nil {
		state.cleanupOnce.Do(func() { close(state.cleanupEntered) })
	}
	if state.cleanupGate != nil {
		<-state.cleanupGate
	}
}

func (state *agentFixtureState) check() error {
	state.record("check")
	if state.checkEntered != nil {
		state.checkOnce.Do(func() { close(state.checkEntered) })
	}
	if state.checkGate != nil {
		<-state.checkGate
	}
	return state.checkErr
}

func (state *agentFixtureState) handle(
	ctx context.Context,
	event string,
) {
	state.record(event)
	state.record(event + ":entered")
	if state.handleEntered != nil {
		state.handleOnce.Do(func() { close(state.handleEntered) })
	}
	if state.handleGate != nil {
		if state.handleCancelReturns {
			select {
			case <-state.handleGate:
			case <-ctx.Done():
				state.record(event + ":cancelled")
			}
		} else {
			<-state.handleGate
		}
	}
	state.record(event + ":returned")
}

func (state *agentFixtureState) record(event string) {
	state.mu.Lock()
	state.events = append(state.events, event)
	state.mu.Unlock()
}

func (state *agentFixtureState) count(event string) int {
	state.mu.Lock()
	defer state.mu.Unlock()
	count := 0
	for _, observed := range state.events {
		if observed == event {
			count++
		}
	}
	return count
}

func (state *agentFixtureState) snapshot() []string {
	state.mu.Lock()
	defer state.mu.Unlock()
	return append([]string(nil), state.events...)
}

type synchronizedBuffer struct {
	mu   sync.Mutex
	data bytes.Buffer
}

func (buffer *synchronizedBuffer) Write(payload []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.data.Write(payload)
}

func (buffer *synchronizedBuffer) contains(fragment string) bool {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return bytes.Contains(buffer.data.Bytes(), []byte(fragment))
}

func (buffer *synchronizedBuffer) count(fragment string) int {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return bytes.Count(buffer.data.Bytes(), []byte(fragment))
}

func (buffer *synchronizedBuffer) snapshot() []byte {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return append([]byte(nil), buffer.data.Bytes()...)
}

func (buffer *synchronizedBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	if buffer.data.Len() > 8*1024 {
		data := buffer.data.Bytes()
		return fmt.Sprintf(
			"%q...%q (%d bytes)",
			data[:4*1024],
			data[len(data)-4*1024:],
			len(data),
		)
	}
	return buffer.data.String()
}

type observedFunctionResult struct {
	status       int
	contentType  string
	payload      []byte
	payloadBytes int
	payloadSHA   [sha256.Size]byte
}

func (buffer *synchronizedBuffer) functionResult(
	uid string,
) (observedFunctionResult, bool, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	prefix := []byte("FUNCTION_RESULT_BEGIN " + uid + " ")
	data := buffer.data.Bytes()
	start := bytes.Index(data, prefix)
	if start < 0 {
		return observedFunctionResult{}, false, nil
	}
	if start > 0 && data[start-1] != '\n' {
		return observedFunctionResult{}, false, errors.New(
			"jobmgr test: Function result prefix is not line-aligned",
		)
	}
	headerEnd := bytes.IndexByte(data[start:], '\n')
	if headerEnd < 0 {
		return observedFunctionResult{}, false, nil
	}
	headerEnd += start
	fields := bytes.Fields(data[start:headerEnd])
	if len(fields) != 5 ||
		string(fields[0]) != "FUNCTION_RESULT_BEGIN" ||
		string(fields[1]) != uid {
		return observedFunctionResult{}, false, errors.New(
			"jobmgr test: malformed Function result header",
		)
	}
	status, err := strconv.Atoi(string(fields[2]))
	if err != nil {
		return observedFunctionResult{}, false, errors.New(
			"jobmgr test: malformed Function result status",
		)
	}
	const resultEnd = "FUNCTION_RESULT_END\n\n"
	end := bytes.Index(data[headerEnd+1:], []byte(resultEnd))
	if end < 0 {
		return observedFunctionResult{}, false, nil
	}
	end += headerEnd + 1
	deferred := data[headerEnd+1 : end]
	payload := deferred
	if len(payload) != 0 {
		if payload[len(payload)-1] != '\n' {
			return observedFunctionResult{}, false, errors.New(
				"jobmgr test: Function result payload lacks final LF",
			)
		}
		payload = payload[:len(payload)-1]
	}
	result := observedFunctionResult{
		status:       status,
		contentType:  string(fields[3]),
		payloadBytes: len(payload),
		payloadSHA:   sha256.Sum256(payload),
	}
	if len(payload) <= 1024*1024 {
		result.payload = append([]byte(nil), payload...)
	}
	return result, true, nil
}

type agentFixture struct {
	agent  *agent.Agent
	input  *io.PipeWriter
	output *synchronizedBuffer
	state  *agentFixtureState
	done   chan error
}

func startAgentFixture(
	ctx context.Context,
	v2 bool,
) (*agentFixture, error) {
	return startAgentFixtureWithState(
		ctx,
		v2,
		&agentFixtureState{},
	)
}

func startAgentFixtureWithState(
	ctx context.Context,
	v2 bool,
	state *agentFixtureState,
) (*agentFixture, error) {
	return startAgentFixtureConfiguredWithRegistry(
		ctx,
		state,
		fixtureRegistry(state, v2),
		nil,
		policy.Agent(true),
	)
}

func startAgentInstanceFixtureWithState(
	ctx context.Context,
	v2 bool,
	state *agentFixtureState,
) (*agentFixture, error) {
	return startAgentFixtureConfiguredWithRegistry(
		ctx,
		state,
		fixtureInstanceRegistry(state, v2),
		nil,
		policy.Agent(true),
	)
}

func startAgentCapacityFixtureWithState(
	ctx context.Context,
	state *agentFixtureState,
) (*agentFixture, error) {
	return startAgentFixtureConfiguredWithRegistry(
		ctx,
		state,
		fixtureCapacityRegistry(state),
		nil,
		policy.Agent(true),
	)
}

func startAgentFixtureConfigured(
	ctx context.Context,
	v2 bool,
	state *agentFixtureState,
	wrapOutput func(*synchronizedBuffer) io.Writer,
	runPolicy policy.RunModePolicy,
) (*agentFixture, error) {
	return startAgentFixtureConfiguredWithRegistry(
		ctx,
		state,
		fixtureRegistry(state, v2),
		wrapOutput,
		runPolicy,
	)
}

func startAgentFixtureConfiguredWithRegistry(
	ctx context.Context,
	state *agentFixtureState,
	registry collectorapi.Registry,
	wrapOutput func(*synchronizedBuffer) io.Writer,
	runPolicy policy.RunModePolicy,
) (*agentFixture, error) {
	logger.Level.SetByName("critical")
	reader, writer := io.Pipe()
	output := &synchronizedBuffer{}
	var agentOutput io.Writer = output
	if wrapOutput != nil {
		agentOutput = wrapOutput(output)
	}
	instance := agent.New(agent.Config{
		Name:            "jobmgrtest",
		ModuleRegistry:  registry,
		RunModule:       productionFixtureModule,
		MinUpdateEvery:  1,
		ShutdownTimeout: 250 * time.Millisecond,
		RunModePolicy:   runPolicy,
		DiscoveryProviders: []discovery.ProviderFactory{
			discovery.NewProviderFactory(
				"dummy",
				func(build discovery.BuildContext) (
					discovery.Discoverer,
					bool,
					error,
				) {
					provider, err := dummy.NewDiscovery(dummy.Config{
						Registry: build.Registry,
						Names:    build.DummyNames,
					})
					return provider, err == nil, err
				},
			),
		},
		DisableServiceDiscovery: true,
	})
	instance.In = reader
	instance.Out = agentOutput
	done := make(chan error, 1)
	go func() {
		done <- instance.RunContext(ctx)
	}()
	return &agentFixture{
		agent: instance, input: writer, output: output,
		state: state, done: done,
	}, nil
}

func (fixture *agentFixture) terminate(ctx context.Context) error {
	if fixture == nil {
		return errors.New("jobmgr test: nil Agent fixture")
	}
	terminateErr := fixture.agent.Terminate(ctx)
	closeErr := fixture.input.Close()
	var runErr error
	select {
	case runErr = <-fixture.done:
	case <-ctx.Done():
		runErr = ctx.Err()
	}
	return errors.Join(terminateErr, closeErr, runErr)
}

func runAgentCollectorLifecycle(
	ctx context.Context,
	v2 bool,
	restart bool,
) error {
	fixture, err := startAgentFixture(ctx, v2)
	if err != nil {
		return err
	}
	if err := waitUntil(ctx, func() bool {
		return fixture.state.count("check") >= 1
	}); err != nil {
		_ = fixture.input.Close()
		return fmt.Errorf("collector did not become active: %w", err)
	}
	if restart {
		if err := fixture.agent.Restart(ctx); err != nil {
			_ = fixture.input.Close()
			return err
		}
		if err := waitUntil(ctx, func() bool {
			return fixture.state.count("check") >= 2
		}); err != nil {
			_ = fixture.input.Close()
			return fmt.Errorf("replacement collector did not start: %w", err)
		}
	}
	if err := fixture.terminate(ctx); err != nil {
		return err
	}
	generations := 1
	if restart {
		generations = 2
	}
	for _, event := range []string{"init", "check", "cleanup"} {
		if got := fixture.state.count(event); got < generations {
			return fmt.Errorf(
				"collector %s count=%d, want at least %d; events=%v",
				event,
				got,
				generations,
				fixture.state.snapshot(),
			)
		}
	}
	if got := fixture.state.count("cleanup"); got != generations {
		return fmt.Errorf(
			"collector cleanup count=%d, want %d",
			got,
			generations,
		)
	}
	if restart {
		events := fixture.state.snapshot()
		firstCleanup := indexOf(events, "cleanup", 0)
		secondInit := indexOf(events, "init", 1)
		if firstCleanup < 0 ||
			secondInit < 0 ||
			firstCleanup >= secondInit {
			return fmt.Errorf(
				"replacement initialized before old cleanup: %v",
				events,
			)
		}
	}
	return nil
}

func runAgentAcquiredAbort(
	ctx context.Context,
	v2 bool,
	requirePublication bool,
) error {
	state := &agentFixtureState{
		checkErr: errors.New("fixture autodetection failure"),
	}
	fixture, err := startAgentFixtureWithState(ctx, v2, state)
	if err != nil {
		return err
	}
	if err := waitUntil(ctx, func() bool {
		return state.count("check") >= 1 &&
			(!requirePublication || fixture.output.contains(
				`FUNCTION GLOBAL "jobmgrtest:echo"`,
			))
	}); err != nil {
		return errors.Join(
			fmt.Errorf(
				"acquired abort did not reach failure boundary: %w; events=%v",
				err,
				state.snapshot(),
			),
			fixture.terminate(ctx),
		)
	}
	terminationErr := fixture.terminate(ctx)
	if state.count("cleanup") != 1 ||
		state.count("handler-cleanup") != 1 {
		return errors.Join(
			fmt.Errorf(
				"acquired abort finalizers differ: events=%v",
				state.snapshot(),
			),
			terminationErr,
		)
	}
	if requirePublication &&
		(!fixture.output.contains(
			`FUNCTION GLOBAL "jobmgrtest:echo"`,
		) || !fixture.output.contains(
			`FUNCTION_DEL GLOBAL "jobmgrtest:echo"`,
		)) {
		return errors.Join(
			fmt.Errorf(
				"published exposure was not reversed: output=%s",
				fixture.output.String(),
			),
			terminationErr,
		)
	}
	return terminationErr
}

func runAgentFunctionFlow(
	ctx context.Context,
	boundaries bool,
) error {
	fixture, err := startAgentFixture(ctx, false)
	if err != nil {
		return err
	}
	if err := waitUntil(ctx, func() bool {
		return fixture.output.contains(
			`FUNCTION GLOBAL "jobmgrtest:echo"`,
		)
	}); err != nil {
		_ = fixture.input.Close()
		return fmt.Errorf("Function was not published: %w", err)
	}
	requests := []struct {
		uid      string
		line     string
		fragment string
	}{
		{
			uid: "jobmgrtest-success",
			line: "FUNCTION jobmgrtest-success 30 " +
				`"jobmgrtest:echo alpha beta" 0xFFFF "method=api,role=test"` +
				"\n",
			fragment: "FUNCTION_RESULT_BEGIN jobmgrtest-success 200 application/json",
		},
	}
	if boundaries {
		requests = append(requests,
			struct {
				uid      string
				line     string
				fragment string
			}{
				uid: "jobmgrtest-missing",
				line: "FUNCTION jobmgrtest-missing 30 " +
					`"missing:route" 0xFFFF "method=api,role=test"` +
					"\n",
				fragment: "FUNCTION_RESULT_BEGIN jobmgrtest-missing 404 application/json",
			},
			struct {
				uid      string
				line     string
				fragment string
			}{
				uid: "jobmgrtest-timeout",
				line: "FUNCTION jobmgrtest-timeout invalid " +
					`"jobmgrtest:echo" 0xFFFF "method=api,role=test"` +
					"\n",
				fragment: "FUNCTION_RESULT_BEGIN jobmgrtest-timeout 400 application/json",
			},
		)
	}
	for _, request := range requests {
		if _, err := io.WriteString(fixture.input, request.line); err != nil {
			_ = fixture.input.Close()
			return err
		}
		if err := waitUntil(ctx, func() bool {
			return fixture.output.contains(request.fragment)
		}); err != nil {
			_ = fixture.input.Close()
			return fmt.Errorf(
				"Function result %s was not observed: %w; output=%q",
				request.uid,
				err,
				fixture.output.String(),
			)
		}
	}
	return fixture.terminate(ctx)
}

func runAgentFunctionHeaderBoundaries(ctx context.Context) error {
	return runAgentFunctionScenario(
		ctx,
		func(fixture *agentFixture) error {
			tests := map[string]struct {
				bytes  int
				status int
			}{
				"exact command": {
					bytes:  frameworkfunctions.MaximumCommandLineBytes,
					status: 404,
				},
				"one byte over command": {
					bytes:  frameworkfunctions.MaximumCommandLineBytes + 1,
					status: 400,
				},
				"one byte over reader scratch": {
					bytes:  64*1024 + 1,
					status: 400,
				},
			}
			for name, test := range tests {
				uid := "jobmgrtest-" + strings.ReplaceAll(
					name,
					" ",
					"-",
				)
				line, err := functionCommandOfLength(
					uid,
					"30",
					"missing:route",
					test.bytes,
				)
				if err != nil {
					return err
				}
				if _, err := io.WriteString(
					fixture.input,
					line+"\n",
				); err != nil {
					return err
				}
				result, err := waitFunctionResult(
					ctx,
					fixture.output,
					uid,
				)
				if err != nil {
					return err
				}
				if result.status != test.status {
					return fmt.Errorf(
						"%s status=%d, want %d",
						name,
						result.status,
						test.status,
					)
				}
			}
			return sendFunctionAndRequireStatus(
				ctx,
				fixture,
				"jobmgrtest-header-successor",
				"30",
				"jobmgrtest:echo successor",
				200,
			)
		},
	)
}

func runAgentFunctionBodyBoundaries(ctx context.Context) error {
	return runAgentFunctionScenario(
		ctx,
		func(fixture *agentFixture) error {
			tests := map[string]struct {
				size   int
				status int
			}{
				"one byte below": {
					size:   lifecycle.MaximumInputBodyBytes - 1,
					status: 200,
				},
				"exact": {
					size:   lifecycle.MaximumInputBodyBytes,
					status: 200,
				},
				"one byte over": {
					size:   lifecycle.MaximumInputBodyBytes + 1,
					status: 413,
				},
			}
			for name, test := range tests {
				uid := "jobmgrtest-body-" + strings.ReplaceAll(
					name,
					" ",
					"-",
				)
				before := fixture.state.count("raw:echo")
				digest, err := writeAgentFunctionPayload(
					fixture.input,
					uid,
					"jobmgrtest:echo",
					"application/octet-stream",
					test.size,
					'x',
				)
				if err != nil {
					return err
				}
				result, err := waitFunctionResult(
					ctx,
					fixture.output,
					uid,
				)
				if err != nil {
					return err
				}
				if result.status != test.status {
					return fmt.Errorf(
						"%s status=%d, want %d",
						name,
						result.status,
						test.status,
					)
				}
				if test.status == 200 {
					if err := validatePayloadDigestResult(
						result,
						test.size,
						digest,
					); err != nil {
						return fmt.Errorf("%s: %w", name, err)
					}
					continue
				}
				if fixture.state.count("raw:echo") != before {
					return errors.New(
						"oversized input reached the Function handler",
					)
				}
				if string(result.payload) !=
					`{"errorMessage":"Payload too large.","status":413}` {
					return fmt.Errorf(
						"oversized input payload=%q",
						result.payload,
					)
				}
			}
			return sendFunctionAndRequireStatus(
				ctx,
				fixture,
				"jobmgrtest-body-successor",
				"30",
				"jobmgrtest:echo successor",
				200,
			)
		},
	)
}

func runAgentFunctionRawPayload(ctx context.Context) error {
	return runAgentFunctionScenario(
		ctx,
		func(fixture *agentFixture) error {
			payload := []byte("  leading\tvalue  \r\n\r\ntrailing\t")
			uid := "jobmgrtest-raw-payload"
			digest, err := writeAgentRawFunctionPayload(
				fixture.input,
				uid,
				"jobmgrtest:echo",
				"application/octet-stream",
				payload,
			)
			if err != nil {
				return err
			}
			result, err := waitFunctionResult(
				ctx,
				fixture.output,
				uid,
			)
			if err != nil {
				return err
			}
			return validatePayloadDigestResult(
				result,
				len(payload),
				digest,
			)
		},
	)
}

func runAgentFunctionTimeoutBoundaries(ctx context.Context) error {
	return runAgentFunctionScenario(
		ctx,
		func(fixture *agentFixture) error {
			tests := map[string]struct {
				timeout string
				status  int
			}{
				"negative": {
					timeout: "-1",
					status:  400,
				},
				"one over maximum": {
					timeout: "901",
					status:  400,
				},
				"integer overflow": {
					timeout: "9223372036854775808",
					status:  400,
				},
				"zero": {
					timeout: "0",
					status:  504,
				},
				"one second": {
					timeout: "1",
					status:  200,
				},
				"maximum": {
					timeout: "900",
					status:  200,
				},
			}
			for name, test := range tests {
				uid := "jobmgrtest-timeout-" + strings.ReplaceAll(
					name,
					" ",
					"-",
				)
				if err := sendFunctionAndRequireStatus(
					ctx,
					fixture,
					uid,
					test.timeout,
					"jobmgrtest:echo "+uid,
					test.status,
				); err != nil {
					return fmt.Errorf("%s: %w", name, err)
				}
			}
			return nil
		},
	)
}

func runAgentFunctionInvalidJSON(ctx context.Context) error {
	return runAgentFunctionScenario(
		ctx,
		func(fixture *agentFixture) error {
			uid := "jobmgrtest-invalid-json"
			before := fixture.state.count("handle:json")
			if _, err := writeAgentRawFunctionPayload(
				fixture.input,
				uid,
				"jobmgrtest:json",
				"application/json",
				[]byte("{not-json"),
			); err != nil {
				return err
			}
			result, err := waitFunctionResult(
				ctx,
				fixture.output,
				uid,
			)
			if err != nil {
				return err
			}
			if result.status != 400 ||
				string(result.payload) !=
					`{"errorMessage":"Bad request.","status":400}` {
				return fmt.Errorf(
					"invalid JSON result differs: status=%d payload=%q",
					result.status,
					result.payload,
				)
			}
			if fixture.state.count("handle:json") != before {
				return errors.New(
					"invalid JSON reached the non-raw Function handler",
				)
			}
			return nil
		},
	)
}

func runAgentFunctionResultBoundaries(ctx context.Context) error {
	return runAgentFunctionScenario(
		ctx,
		func(fixture *agentFixture) error {
			overUID := "jobmgrtest-result-over"
			if err := sendFunctionAndRequireStatus(
				ctx,
				fixture,
				overUID,
				"30",
				fmt.Sprintf(
					"jobmgrtest:echo result-deferred:%d",
					lifecycle.FunctionPayloadBytes+1,
				),
				413,
			); err != nil {
				return err
			}
			over, err := waitFunctionResult(
				ctx,
				fixture.output,
				overUID,
			)
			if err != nil {
				return err
			}
			if string(over.payload) !=
				`{"errorMessage":"Payload too large.","status":413}` {
				return fmt.Errorf(
					"oversized result payload=%q",
					over.payload,
				)
			}

			const largeDeferredBytes = 32 * 1024 * 1024
			largeUID := "jobmgrtest-result-large"
			if _, err := io.WriteString(
				fixture.input,
				fmt.Sprintf(
					"FUNCTION %s 30 %q 0xFFFF %q\n",
					largeUID,
					fmt.Sprintf(
						"jobmgrtest:echo result-deferred:%d",
						largeDeferredBytes,
					),
					"method=api,role=test",
				),
			); err != nil {
				return err
			}
			large, err := waitFunctionResult(
				ctx,
				fixture.output,
				largeUID,
			)
			if err != nil {
				return err
			}
			wantPayloadBytes := largeDeferredBytes - 1
			if large.status != 200 ||
				large.contentType != "application/json" ||
				large.payloadBytes != wantPayloadBytes ||
				large.payloadSHA != repeatedJSONSHA256(
					wantPayloadBytes,
				) {
				return fmt.Errorf(
					"large result differs: status=%d content=%q bytes=%d sha256=%x",
					large.status,
					large.contentType,
					large.payloadBytes,
					large.payloadSHA,
				)
			}
			return nil
		},
	)
}

func runAgentFunctionScenario(
	ctx context.Context,
	exercise func(*agentFixture) error,
) error {
	fixture, err := startAgentFixture(ctx, false)
	if err != nil {
		return err
	}
	if err := waitUntil(ctx, func() bool {
		return fixture.output.contains(
			`FUNCTION GLOBAL "jobmgrtest:echo"`,
		) && fixture.output.contains(
			`FUNCTION GLOBAL "jobmgrtest:json"`,
		)
	}); err != nil {
		return errors.Join(
			fmt.Errorf("Functions were not published: %w", err),
			fixture.terminate(ctx),
		)
	}
	exerciseErr := exercise(fixture)
	return errors.Join(exerciseErr, fixture.terminate(ctx))
}

func sendFunctionAndRequireStatus(
	ctx context.Context,
	fixture *agentFixture,
	uid string,
	timeout string,
	call string,
	want int,
) error {
	if _, err := io.WriteString(
		fixture.input,
		fmt.Sprintf(
			"FUNCTION %s %s %q 0xFFFF %q\n",
			uid,
			timeout,
			call,
			"method=api,role=test",
		),
	); err != nil {
		return err
	}
	result, err := waitFunctionResult(ctx, fixture.output, uid)
	if err != nil {
		return err
	}
	if result.status != want {
		return fmt.Errorf(
			"Function result %s status=%d, want %d",
			uid,
			result.status,
			want,
		)
	}
	return nil
}

func waitFunctionResult(
	ctx context.Context,
	output *synchronizedBuffer,
	uid string,
) (observedFunctionResult, error) {
	timer := time.NewTicker(10 * time.Millisecond)
	defer timer.Stop()
	for {
		result, ready, err := output.functionResult(uid)
		if err != nil {
			return observedFunctionResult{}, err
		}
		if ready {
			return result, nil
		}
		select {
		case <-ctx.Done():
			return observedFunctionResult{}, fmt.Errorf(
				"wait for Function result %s: %w; output=%s",
				uid,
				ctx.Err(),
				output.String(),
			)
		case <-timer.C:
		}
	}
}

func functionCommandOfLength(
	uid string,
	timeout string,
	route string,
	length int,
) (string, error) {
	prefix := fmt.Sprintf(
		"FUNCTION %s %s \"%s ",
		uid,
		timeout,
		route,
	)
	suffix := "\" 0xFFFF \"method=api,role=test\""
	padding := length - len(prefix) - len(suffix)
	if padding < 0 {
		return "", fmt.Errorf(
			"jobmgr test: command length %d is too small",
			length,
		)
	}
	command := prefix + strings.Repeat("x", padding) + suffix
	if len(command) != length {
		return "", errors.New(
			"jobmgr test: constructed command length differs",
		)
	}
	return command, nil
}

func writeAgentFunctionPayload(
	writer io.Writer,
	uid string,
	route string,
	contentType string,
	size int,
	value byte,
) ([sha256.Size]byte, error) {
	header := fmt.Sprintf(
		"FUNCTION_PAYLOAD %s 30 %q 0xFFFF %q %s\n",
		uid,
		route,
		"method=api,role=test",
		contentType,
	)
	if _, err := io.WriteString(writer, header); err != nil {
		return [sha256.Size]byte{}, err
	}
	digest := sha256.New()
	block := bytes.Repeat([]byte{value}, 64*1024)
	remaining := size
	for remaining > 0 {
		count := min(remaining, len(block))
		if _, err := writer.Write(block[:count]); err != nil {
			return [sha256.Size]byte{}, err
		}
		_, _ = digest.Write(block[:count])
		remaining -= count
	}
	if _, err := io.WriteString(
		writer,
		"\nFUNCTION_PAYLOAD_END\n",
	); err != nil {
		return [sha256.Size]byte{}, err
	}
	var result [sha256.Size]byte
	copy(result[:], digest.Sum(nil))
	return result, nil
}

func writeAgentRawFunctionPayload(
	writer io.Writer,
	uid string,
	route string,
	contentType string,
	payload []byte,
) ([sha256.Size]byte, error) {
	header := fmt.Sprintf(
		"FUNCTION_PAYLOAD %s 30 %q 0xFFFF %q %s\n",
		uid,
		route,
		"method=api,role=test",
		contentType,
	)
	if _, err := io.WriteString(writer, header); err != nil {
		return [sha256.Size]byte{}, err
	}
	if _, err := writer.Write(payload); err != nil {
		return [sha256.Size]byte{}, err
	}
	if _, err := io.WriteString(
		writer,
		"\nFUNCTION_PAYLOAD_END\n",
	); err != nil {
		return [sha256.Size]byte{}, err
	}
	return sha256.Sum256(payload), nil
}

func validatePayloadDigestResult(
	result observedFunctionResult,
	wantBytes int,
	wantSHA [sha256.Size]byte,
) error {
	if result.status != 200 ||
		result.contentType != "application/json" {
		return fmt.Errorf(
			"payload digest status=%d content=%q",
			result.status,
			result.contentType,
		)
	}
	var payload struct {
		ContentType   string `json:"content_type"`
		PayloadBytes  int    `json:"payload_bytes"`
		PayloadSHA256 string `json:"payload_sha256"`
		Status        int    `json:"status"`
	}
	if err := json.Unmarshal(result.payload, &payload); err != nil {
		return fmt.Errorf("decode payload digest result: %w", err)
	}
	if payload.ContentType != "application/octet-stream" ||
		payload.PayloadBytes != wantBytes ||
		payload.PayloadSHA256 != hex.EncodeToString(wantSHA[:]) ||
		payload.Status != 200 {
		return fmt.Errorf(
			"payload digest result differs: %+v",
			payload,
		)
	}
	return nil
}

func repeatedJSONSHA256(payloadBytes int) [sha256.Size]byte {
	const prefix = `{"pad":"`
	const suffix = `"}`
	repeated := payloadBytes - len(prefix) - len(suffix)
	digest := sha256.New()
	_, _ = io.WriteString(digest, prefix)
	block := bytes.Repeat([]byte{'A'}, 64*1024)
	for repeated > 0 {
		count := min(repeated, len(block))
		_, _ = digest.Write(block[:count])
		repeated -= count
	}
	_, _ = io.WriteString(digest, suffix)
	var result [sha256.Size]byte
	copy(result[:], digest.Sum(nil))
	return result
}

func runAgentFunctionBurst(ctx context.Context) error {
	fixture, err := startAgentFixture(ctx, false)
	if err != nil {
		return err
	}
	if err := waitUntil(ctx, func() bool {
		return fixture.output.contains(
			`FUNCTION GLOBAL "jobmgrtest:echo"`,
		)
	}); err != nil {
		_ = fixture.input.Close()
		return err
	}
	const requests = 32
	for index := 0; index < requests; index++ {
		uid := fmt.Sprintf("jobmgrtest-burst-%02d", index)
		line := fmt.Sprintf(
			"FUNCTION %s 30 %q 0xFFFF %q\n",
			uid,
			"jobmgrtest:echo "+uid,
			"method=api,role=test",
		)
		if _, err := io.WriteString(fixture.input, line); err != nil {
			_ = fixture.input.Close()
			return err
		}
	}
	for index := 0; index < requests; index++ {
		uid := fmt.Sprintf("jobmgrtest-burst-%02d", index)
		if err := waitUntil(ctx, func() bool {
			return fixture.output.contains(
				"FUNCTION_RESULT_BEGIN " + uid + " 200 application/json",
			)
		}); err != nil {
			_ = fixture.input.Close()
			return fmt.Errorf("Function burst stalled at %s: %w", uid, err)
		}
	}
	return fixture.terminate(ctx)
}

func fixtureRegistry(
	state *agentFixtureState,
	v2 bool,
) collectorapi.Registry {
	return fixtureRegistryWithFunctions(state, v2, false, 2)
}

func fixtureInstanceRegistry(
	state *agentFixtureState,
	v2 bool,
) collectorapi.Registry {
	return fixtureRegistryWithFunctions(state, v2, true, 2)
}

func fixtureCapacityRegistry(
	state *agentFixtureState,
) collectorapi.Registry {
	return fixtureRegistryWithFunctions(state, false, false, 2)
}

func fixtureOutputRegistry(
	state *agentFixtureState,
	v2 bool,
) collectorapi.Registry {
	registry := fixtureRegistry(state, v2)
	creator := registry[productionFixtureModule]
	creator.FunctionOnly = false
	registry[productionFixtureModule] = creator
	return registry
}

func fixtureRegistryWithFunctions(
	state *agentFixtureState,
	v2 bool,
	instanceFunctions bool,
	functionCount int,
) collectorapi.Registry {
	creator := collectorapi.Creator{
		Defaults:       collectorapi.Defaults{UpdateEvery: 1},
		FunctionOnly:   true,
		InstancePolicy: collectorapi.InstancePolicySingle,
		MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
			return fixtureFunctionHandler{state: state}
		},
	}
	functions := func() []funcapi.FunctionConfig {
		return fixtureFunctionConfigs(functionCount)
	}
	if instanceFunctions {
		creator.InstanceFunctions = func(collectorapi.RuntimeJob) []funcapi.FunctionConfig {
			return functions()
		}
	} else {
		creator.AgentFunctions = functions
	}
	if v2 {
		creator.CreateV2 = func() collectorapi.CollectorV2 {
			return &fixtureCollectorV2{
				state: state,
				store: metrix.NewCollectorStore(),
			}
		}
	} else {
		creator.Create = func() collectorapi.CollectorV1 {
			return &collectorapi.MockCollectorV1{
				InitFunc: func(context.Context) error {
					state.record("init")
					return nil
				},
				CheckFunc: func(context.Context) error {
					return state.check()
				},
				CollectFunc: func(context.Context) map[string]int64 {
					state.record("collect")
					if state.emitCharts {
						return map[string]int64{"value": 1}
					}
					return nil
				},
				ChartsFunc: func() *collectorapi.Charts {
					if !state.emitCharts {
						return nil
					}
					return &collectorapi.Charts{
						&collectorapi.Chart{
							ID: "value", Title: "Value", Units: "value",
							Dims: collectorapi.Dims{
								&collectorapi.Dim{ID: "value"},
							},
						},
					}
				},
				CleanupFunc: func(context.Context) {
					state.cleanup()
				},
			}
		}
	}
	return collectorapi.Registry{productionFixtureModule: creator}
}

func fixtureFunctionConfigs(count int) []funcapi.FunctionConfig {
	functions := []funcapi.FunctionConfig{
		{
			ID: "echo", FunctionName: "jobmgrtest:echo",
			Name: "jobmgrtest:echo", RawRequest: true,
		},
		{
			ID: "json", FunctionName: "jobmgrtest:json",
			Name: "jobmgrtest:json",
		},
	}
	for ordinal := 0; len(functions) < count; ordinal++ {
		id := fmt.Sprintf("work-%03d", ordinal)
		functions = append(functions, funcapi.FunctionConfig{
			ID: id, FunctionName: "jobmgrtest:" + id,
			Name: "jobmgrtest:" + id, RawRequest: true,
		})
	}
	return functions[:count]
}

type fixtureCollectorV2 struct {
	collectorapi.Base
	state *agentFixtureState
	store metrix.CollectorStore
}

func (collector *fixtureCollectorV2) Init(context.Context) error {
	collector.state.record("init")
	return nil
}

func (collector *fixtureCollectorV2) Check(context.Context) error {
	return collector.state.check()
}

func (collector *fixtureCollectorV2) Collect(context.Context) error {
	collector.state.record("collect")
	if collector.state.emitCharts {
		collector.store.Write().
			SnapshotMeter("jobmgrtest").
			Gauge("value").
			Observe(1)
	}
	return nil
}

func (collector *fixtureCollectorV2) Cleanup(context.Context) {
	collector.state.cleanup()
}

func (*fixtureCollectorV2) Configuration() any {
	return struct{}{}
}

func (*fixtureCollectorV2) VirtualNode() *vnodes.VirtualNode {
	return nil
}

func (collector *fixtureCollectorV2) MetricStore() metrix.CollectorStore {
	return collector.store
}

func (*fixtureCollectorV2) ChartTemplateYAML() string {
	return `
version: "v1"
groups:
  - family: "jobmgrtest"
    metrics: ["jobmgrtest.value"]
    charts:
      - context: "value"
        title: "Value"
        units: "value"
        dimensions:
          - selector: "jobmgrtest.value"
            name: "value"
`
}

type fixtureFunctionHandler struct {
	state *agentFixtureState
}

func (fixtureFunctionHandler) MethodParams(
	context.Context,
	string,
) ([]funcapi.ParamConfig, error) {
	return nil, nil
}

func (handler fixtureFunctionHandler) Handle(
	ctx context.Context,
	method string,
	_ funcapi.ResolvedParams,
) *funcapi.FunctionResponse {
	handler.state.handle(ctx, "handle:"+method)
	return funcapi.RawResponse(map[string]any{
		"method": method,
		"status": 200,
	})
}

func (handler fixtureFunctionHandler) HandleRaw(
	ctx context.Context,
	request funcapi.RawMethodRequest,
) *funcapi.FunctionResponse {
	handler.state.handle(ctx, "raw:"+request.Method)
	if deferred, ok := requestedDeferredBytes(request.Args); ok {
		const fixedBytes = len(`{"pad":""}`)
		if deferred <= fixedBytes {
			return funcapi.ErrorResponse(
				400,
				"invalid result boundary",
			)
		}
		return funcapi.RawResponse(map[string]any{
			"pad": strings.Repeat(
				"A",
				deferred-1-fixedBytes,
			),
		})
	}
	if len(request.Payload) != 0 {
		digest := sha256.Sum256(request.Payload)
		return funcapi.RawResponse(map[string]any{
			"content_type":   request.ContentType,
			"payload_bytes":  len(request.Payload),
			"payload_sha256": hex.EncodeToString(digest[:]),
			"status":         200,
		})
	}
	return funcapi.RawResponse(map[string]any{
		"args":   strings.Join(request.Args, " "),
		"method": request.Method,
		"status": 200,
	})
}

func (handler fixtureFunctionHandler) Cleanup(context.Context) {
	handler.state.record("handler-cleanup")
}

func requestedDeferredBytes(arguments []string) (int, bool) {
	for _, argument := range arguments {
		value, ok := strings.CutPrefix(
			argument,
			"result-deferred:",
		)
		if !ok {
			continue
		}
		deferred, err := strconv.Atoi(value)
		return deferred, err == nil
	}
	return 0, false
}

func waitUntil(ctx context.Context, predicate func() bool) error {
	timer := time.NewTicker(10 * time.Millisecond)
	defer timer.Stop()
	for {
		if predicate() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func indexOf(values []string, value string, occurrence int) int {
	for index, observed := range values {
		if observed != value {
			continue
		}
		if occurrence == 0 {
			return index
		}
		occurrence--
	}
	return -1
}
