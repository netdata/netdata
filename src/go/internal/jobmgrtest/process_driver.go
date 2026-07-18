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

// ProcessDriver exercises process-fixed input ownership and acknowledged
// restart/termination through composition.NewProcess.
type ProcessDriver struct {
	mu      sync.Mutex
	results map[string]error
}

func (driver *ProcessDriver) Run(
	ctx context.Context,
	scenario string,
) error {
	if driver == nil || ctx == nil {
		return errors.New("jobmgr test: invalid Process driver")
	}
	driver.mu.Lock()
	if result, ok := driver.results[scenario]; ok {
		driver.mu.Unlock()
		return result
	}
	if driver.results == nil {
		driver.results = make(map[string]error)
	}
	driver.mu.Unlock()

	var result error
	switch scenario {
	case "restart":
		result = runProcessRestart(ctx)
	case "input-fence":
		result = runProcessInputFence(ctx)
	case "noncooperative-shutdown":
		result = runProcessNoncooperativeShutdown(ctx)
	default:
		result = fmt.Errorf(
			"jobmgr test: unknown Process scenario %q",
			scenario,
		)
	}

	driver.mu.Lock()
	driver.results[scenario] = result
	driver.mu.Unlock()
	return result
}

// CollectorBoundaryDriver treats collectors as opaque V1/V2/Cleanup
// implementations and observes only the public composition disposition.
type CollectorBoundaryDriver struct {
	mu      sync.Mutex
	results map[string]error
}

func (driver *CollectorBoundaryDriver) Run(
	ctx context.Context,
	scenario string,
) error {
	if driver == nil || ctx == nil {
		return errors.New("jobmgr test: invalid collector-boundary driver")
	}
	driver.mu.Lock()
	if result, ok := driver.results[scenario]; ok {
		driver.mu.Unlock()
		return result
	}
	if driver.results == nil {
		driver.results = make(map[string]error)
	}
	driver.mu.Unlock()

	var result error
	switch scenario {
	case "cleanup-once":
		result = runProcessInputFence(ctx)
	case "retained-cleanup":
		result = runProcessNoncooperativeShutdown(ctx)
	default:
		result = fmt.Errorf(
			"jobmgr test: unknown collector-boundary scenario %q",
			scenario,
		)
	}

	driver.mu.Lock()
	driver.results[scenario] = result
	driver.mu.Unlock()
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
