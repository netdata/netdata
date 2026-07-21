package jobmgrtest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	agentdiscovery "github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/dummy"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/composition"
	"github.com/netdata/netdata/go/plugins/plugin/agent/policy"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
)

// ProcessDriver exercises process-fixed behavior through composition.NewProcess.
type ProcessDriver struct{}

type ProcessScenario string

var processRuntimeScenarios = map[ProcessScenario]func(context.Context) error{
	"restart waits for old Cleanup":              runProcessRestart,
	"noncooperative Cleanup remains owned":       runProcessNoncooperativeShutdown,
	"input fence cleans up exactly once":         runProcessInputFence,
	"repeated stop invokes Cleanup exactly once": runCollectorRepeatedStop,
}

func ProcessScenarios() map[ProcessScenario]struct{} {
	scenarios := make(
		map[ProcessScenario]struct{},
		len(processRuntimeScenarios),
	)
	for scenario := range processRuntimeScenarios {
		scenarios[scenario] = struct{}{}
	}
	return scenarios
}

func (d *ProcessDriver) Run(
	ctx context.Context,
	scenario ProcessScenario,
) error {
	if d == nil || ctx == nil {
		return errors.New("jobmgr test: invalid Process driver")
	}
	run := processRuntimeScenarios[scenario]
	if run == nil {
		return fmt.Errorf(
			"jobmgr test: unknown Process scenario %q",
			scenario,
		)
	}
	return run(ctx)
}

type processFixture struct {
	process *composition.Process
	input   *io.PipeWriter
	state   *agentFixtureState
	cancel  context.CancelFunc
	done    chan struct{}
	runErr  error

	forceOnce sync.Once
}

func startProcessFixture(
	runCtx context.Context,
	state *agentFixtureState,
	shutdownTimeout time.Duration,
) (*processFixture, error) {
	logger.Level.SetByName("critical")
	reader, writer := io.Pipe()
	output := &synchronizedBuffer{}
	defaults := confgroup.Registry{
		productionFixtureModule: {
			UpdateEvery: 1,
		},
	}
	build := agentdiscovery.BuildContext{
		RunMode: policy.Agent(true),
		Identity: agentdiscovery.PluginIdentity{
			Name: "jobmgrtest",
		},
		Registry:   defaults,
		DummyNames: []string{productionFixtureModule},
	}
	provider := agentdiscovery.NewProviderFactory(
		"dummy",
		func(build agentdiscovery.BuildContext) (
			agentdiscovery.Discoverer,
			bool,
			error,
		) {
			discoverer, err := dummy.NewDiscovery(dummy.Config{
				Registry: build.Registry,
				Names:    build.DummyNames,
			})
			return discoverer, err == nil, err
		},
	)
	process, err := composition.NewProcess(composition.Config{
		Input: reader, Output: output,
		PluginName:            "jobmgrtest",
		Modules:               fixtureRegistry(state, false),
		Defaults:              defaults,
		DiscoveryBuildContext: build,
		DiscoveryProviders:    []agentdiscovery.ProviderFactory{provider},
		AutoEnable:            true,
		ShutdownTimeout:       shutdownTimeout,
	})
	if err != nil {
		_ = reader.Close()
		_ = writer.Close()
		return nil, err
	}
	runCtx, cancel := context.WithCancel(runCtx)
	state.ownerDone = runCtx.Done()
	fixture := &processFixture{
		process: process,
		input:   writer,
		state:   state,
		cancel:  cancel,
		done:    make(chan struct{}),
	}
	go func() {
		fixture.runErr = process.Run(runCtx)
		close(fixture.done)
	}()
	return fixture, nil
}

func (f *processFixture) wait(ctx context.Context) error {
	select {
	case <-f.done:
		return f.runErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (f *processFixture) close() {
	f.forceOnce.Do(func() {
		f.cancel()
		_ = f.input.Close()
	})
	joinCtx, cancelJoin := context.WithTimeout(
		context.Background(),
		fixtureJoinPeriod,
	)
	defer cancelJoin()
	_ = f.wait(joinCtx)
}

func runProcessRestart(ctx context.Context) error {
	release := make(chan struct{})
	entered := make(chan struct{})
	state := &agentFixtureState{
		cleanupGate: release, cleanupEntered: entered,
	}
	fixture, err := startProcessFixture(
		ctx,
		state,
		time.Second,
	)
	if err != nil {
		return err
	}
	defer fixture.close()
	var releaseOnce sync.Once
	releaseCleanup := func() {
		releaseOnce.Do(func() { close(release) })
	}
	defer releaseCleanup()
	if err := waitUntil(ctx, func() bool {
		return state.count("check") == 1
	}); err != nil {
		_ = fixture.input.Close()
		return err
	}
	restarted := make(chan error, 1)
	go func() {
		restarted <- fixture.process.Restart(ctx)
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		_ = fixture.input.Close()
		return ctx.Err()
	}
	select {
	case err := <-restarted:
		_ = fixture.input.Close()
		return fmt.Errorf(
			"restart returned before old Cleanup disposition: %v",
			err,
		)
	case <-time.After(50 * time.Millisecond):
	}
	if state.count("init") != 1 {
		_ = fixture.input.Close()
		return errors.New(
			"replacement initialized before old Cleanup disposition",
		)
	}
	releaseCleanup()
	select {
	case err := <-restarted:
		if err != nil {
			_ = fixture.input.Close()
			return err
		}
	case <-ctx.Done():
		_ = fixture.input.Close()
		return ctx.Err()
	}
	if err := waitUntil(ctx, func() bool {
		return state.count("check") == 2
	}); err != nil {
		_ = fixture.input.Close()
		return err
	}
	if err := fixture.process.Terminate(ctx); err != nil {
		_ = fixture.input.Close()
		return err
	}
	if err := fixture.input.Close(); err != nil {
		return err
	}
	if err := fixture.wait(ctx); err != nil {
		return err
	}
	if got := state.count("cleanup"); got != 2 {
		return fmt.Errorf("process cleanup count=%d, want 2", got)
	}
	return nil
}

func runProcessInputFence(ctx context.Context) error {
	state := &agentFixtureState{}
	fixture, err := startProcessFixture(
		ctx,
		state,
		time.Second,
	)
	if err != nil {
		return err
	}
	defer fixture.close()
	if err := waitUntil(ctx, func() bool {
		return state.count("check") == 1
	}); err != nil {
		_ = fixture.input.Close()
		return err
	}
	if err := fixture.process.Terminate(ctx); err != nil {
		_ = fixture.input.Close()
		return err
	}
	if err := fixture.input.Close(); err != nil {
		return err
	}
	if err := fixture.wait(ctx); err != nil {
		return err
	}
	if got := state.count("cleanup"); got != 1 {
		return fmt.Errorf("process cleanup count=%d, want 1", got)
	}
	return nil
}

func runProcessNoncooperativeShutdown(ctx context.Context) error {
	release := make(chan struct{})
	entered := make(chan struct{})
	state := &agentFixtureState{
		cleanupGate: release, cleanupEntered: entered,
	}
	fixture, err := startProcessFixture(
		ctx,
		state,
		100*time.Millisecond,
	)
	if err != nil {
		return err
	}
	defer fixture.close()
	var releaseOnce sync.Once
	releaseCleanup := func() {
		releaseOnce.Do(func() { close(release) })
	}
	defer releaseCleanup()
	if err := waitUntil(ctx, func() bool {
		return state.count("check") == 1
	}); err != nil {
		_ = fixture.input.Close()
		return err
	}
	terminated := make(chan error, 1)
	go func() {
		terminated <- fixture.process.Terminate(ctx)
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		_ = fixture.input.Close()
		return ctx.Err()
	}
	var terminalErr error
	select {
	case terminalErr = <-terminated:
	case <-ctx.Done():
		_ = fixture.input.Close()
		return ctx.Err()
	}
	releaseCleanup()
	closeErr := fixture.input.Close()
	runErr := fixture.wait(ctx)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if terminalErr == nil || runErr == nil {
		return fmt.Errorf(
			"noncooperative Cleanup reported success: terminate=%v run=%v",
			terminalErr,
			runErr,
		)
	}
	if state.count("init") != 1 || state.count("cleanup") != 1 {
		return fmt.Errorf(
			"noncooperative Cleanup ownership changed: events=%v",
			state.snapshot(),
		)
	}
	return closeErr
}

func runCollectorRepeatedStop(ctx context.Context) error {
	release := make(chan struct{})
	entered := make(chan struct{})
	state := &agentFixtureState{
		cleanupGate:    release,
		cleanupEntered: entered,
	}
	fixture, err := startProcessFixture(
		ctx,
		state,
		time.Second,
	)
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
	results := []chan error{
		make(chan error, 1),
		make(chan error, 1),
	}
	go func() {
		results[0] <- fixture.process.Terminate(ctx)
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		return ctx.Err()
	}
	go func() {
		results[1] <- fixture.process.Terminate(ctx)
	}()
	secondConsumed := false
	var secondErr error
	select {
	case secondErr = <-results[1]:
		secondConsumed = true
		if !errors.Is(secondErr, composition.ErrProcessStopped) {
			return fmt.Errorf(
				"repeated stop error=%v, want %w",
				secondErr,
				composition.ErrProcessStopped,
			)
		}
	case <-time.After(50 * time.Millisecond):
	}
	if got := state.count("cleanup"); got != 1 {
		return fmt.Errorf(
			"held repeated stop Cleanup count=%d, want 1",
			got,
		)
	}
	close(release)
	released = true
	var terminalErr error
	for index, result := range results {
		if index == 1 && secondConsumed {
			continue
		}
		select {
		case err := <-result:
			if index == 1 &&
				errors.Is(err, composition.ErrProcessStopped) {
				continue
			}
			terminalErr = errors.Join(terminalErr, err)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if terminalErr != nil {
		return terminalErr
	}
	if got := state.count("cleanup"); got != 1 {
		return fmt.Errorf(
			"repeated stop invoked Cleanup %d times, want 1",
			got,
		)
	}
	if err := fixture.input.Close(); err != nil {
		return err
	}
	return fixture.wait(ctx)
}
