package jobmgrtest

import (
	"context"
	"errors"
	"fmt"
	"io"
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

const (
	ProcessRestart                ProcessScenario = "restart"
	ProcessNoncooperativeShutdown ProcessScenario = "noncooperative shutdown"
	ProcessInputFence             ProcessScenario = "input fence"
	ProcessRepeatedStop           ProcessScenario = "repeated stop"
)

func (driver *ProcessDriver) Run(
	ctx context.Context,
	scenario ProcessScenario,
) error {
	if driver == nil || ctx == nil {
		return errors.New("jobmgr test: invalid Process driver")
	}
	var result error
	switch scenario {
	case ProcessRestart:
		result = runProcessRestart(ctx)
	case ProcessNoncooperativeShutdown:
		result = runProcessNoncooperativeShutdown(ctx)
	case ProcessInputFence:
		result = runProcessInputFence(ctx)
	case ProcessRepeatedStop:
		result = runCollectorRepeatedStop(ctx)
	default:
		result = fmt.Errorf(
			"jobmgr test: unknown Process scenario %q",
			scenario,
		)
	}
	return result
}

type processFixture struct {
	process *composition.Process
	input   *io.PipeWriter
	state   *agentFixtureState
	done    chan error
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
	done := make(chan error, 1)
	go func() {
		done <- process.Run(runCtx)
	}()
	return &processFixture{
		process: process, input: writer, state: state, done: done,
	}, nil
}

func runProcessRestart(ctx context.Context) error {
	release := make(chan struct{})
	entered := make(chan struct{})
	state := &agentFixtureState{
		cleanupGate: release, cleanupEntered: entered,
	}
	fixture, err := startProcessFixture(
		context.Background(),
		state,
		time.Second,
	)
	if err != nil {
		return err
	}
	if err := waitUntil(ctx, func() bool {
		return state.count("check") == 1
	}); err != nil {
		_ = fixture.input.Close()
		close(release)
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
		close(release)
		return ctx.Err()
	}
	select {
	case err := <-restarted:
		close(release)
		_ = fixture.input.Close()
		return fmt.Errorf(
			"restart returned before old Cleanup disposition: %v",
			err,
		)
	case <-time.After(50 * time.Millisecond):
	}
	if state.count("init") != 1 {
		close(release)
		_ = fixture.input.Close()
		return errors.New(
			"replacement initialized before old Cleanup disposition",
		)
	}
	close(release)
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
	select {
	case err := <-fixture.done:
		if err != nil {
			return err
		}
	case <-ctx.Done():
		return ctx.Err()
	}
	if got := state.count("cleanup"); got != 2 {
		return fmt.Errorf("process cleanup count=%d, want 2", got)
	}
	return nil
}

func runProcessInputFence(ctx context.Context) error {
	state := &agentFixtureState{}
	fixture, err := startProcessFixture(
		context.Background(),
		state,
		time.Second,
	)
	if err != nil {
		return err
	}
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
	select {
	case err := <-fixture.done:
		if err != nil {
			return err
		}
	case <-ctx.Done():
		return ctx.Err()
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
		context.Background(),
		state,
		100*time.Millisecond,
	)
	if err != nil {
		return err
	}
	if err := waitUntil(ctx, func() bool {
		return state.count("check") == 1
	}); err != nil {
		_ = fixture.input.Close()
		close(release)
		return err
	}
	terminated := make(chan error, 1)
	go func() {
		terminated <- fixture.process.Terminate(ctx)
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		close(release)
		_ = fixture.input.Close()
		return ctx.Err()
	}
	var terminalErr error
	select {
	case terminalErr = <-terminated:
	case <-ctx.Done():
		close(release)
		_ = fixture.input.Close()
		return ctx.Err()
	}
	close(release)
	closeErr := fixture.input.Close()
	var runErr error
	select {
	case runErr = <-fixture.done:
	case <-ctx.Done():
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
		context.Background(),
		state,
		time.Second,
	)
	if err != nil {
		return err
	}
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
		if secondErr == nil {
			return errors.New(
				"repeated stop returned success while Cleanup was held",
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
	select {
	case err := <-fixture.done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
