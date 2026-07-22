//go:build !windows

package jobmgrtest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/internal/jobmgrtest/runner"
)

func shippedRootAvailable(root shippedRoot) bool {
	if !filepath.IsAbs(root.executable) {
		return false
	}
	info, err := os.Stat(root.executable)
	return err == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0
}

type shippedRootScenario string

const (
	shippedRootQuit        shippedRootScenario = "quit"
	shippedRootRepeatedHUP shippedRootScenario = "repeated-hup"
	shippedRootShutdown    shippedRootScenario = "shutdown"
)

var shippedRootScenarios = [...]shippedRootScenario{shippedRootQuit, shippedRootRepeatedHUP, shippedRootShutdown}

// RunMatrixAvailable executes every supported physical root under each
// production-relevant process lifecycle.
func (srd ShippedRootDriver) RunMatrixAvailable(ctx context.Context) ([]string, error) {
	if ctx == nil {
		return nil, errors.New("jobmgr test: nil shipped-root matrix context")
	}
	if err := srd.validateConfigs(); err != nil {
		return nil, err
	}
	var missing []string
	for _, root := range srd.roots() {
		if !shippedRootAvailable(root) {
			missing = append(missing, root.name)
			continue
		}
		for _, scenario := range shippedRootScenarios {
			if err := runShippedRoot(ctx, srd.ConfigDir, root, scenario); err != nil {
				return missing, fmt.Errorf("%s %s: %w", root.name, scenario, err)
			}
		}
	}
	return missing, nil
}

func runShippedRoot(ctx context.Context, configDir string, root shippedRoot, scenario shippedRootScenario) error {
	observer := newRootProtocolObservation(root.templateID)
	spec := runner.Spec{
		Executable: root.executable,
		Arguments:  []string{"-m", root.module, "-c", configDir},
		Directory:  configDir,
		ObserveOut: observer.observe,
	}
	process, err := runner.Start(spec)
	if err != nil {
		return err
	}
	defer func() {
		_ = process.Kill()
		joinCtx, cancel := context.WithTimeout(context.Background(), fixtureJoinPeriod)
		defer cancel()
		_, _ = process.Wait(joinCtx)
	}()

	if err := observer.wait(ctx, process, func(generations, _ int) bool {
		return generations >= 1
	}); err != nil {
		return err
	}
	switch scenario {
	case shippedRootQuit:
		_, keepalives := observer.snapshot()
		if err := observer.wait(ctx, process, func(_, observedKeepalives int) bool {
			return observedKeepalives > keepalives
		}); err != nil {
			return fmt.Errorf("nonterminal root emitted no process keepalive: %w", err)
		}
		if err := process.WriteContext(ctx, []byte("QUIT\n")); err != nil {
			return err
		}
	case shippedRootShutdown:
		if err := process.Signal(os.Signal(syscallSIGTERM())); err != nil {
			return err
		}
	case shippedRootRepeatedHUP:
		for index := range 3 {
			if err := process.Signal(os.Signal(syscallSIGHUP())); err != nil {
				return err
			}
			if err := observer.wait(
				ctx,
				process,
				func(generations, _ int) bool {
					return generations >= index+2
				},
			); err != nil {
				return err
			}
		}
		if err := process.WriteContext(ctx, []byte("QUIT\n")); err != nil {
			return err
		}
	default:
		return fmt.Errorf("jobmgr test: unknown shipped-root scenario %q", scenario)
	}
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := process.Wait(waitCtx)
	if err != nil {
		generations, _ := observer.snapshot()
		return errors.Join(
			err,
			fmt.Errorf("generations=%d stderr=%q", generations, result.Stderr),
		)
	}
	generations, _ := observer.snapshot()
	want := 1
	if scenario == shippedRootRepeatedHUP {
		want = 4
	}
	if generations != want {
		return fmt.Errorf(
			"jobmgr test: shipped-root protocol lifecycle generations=%d, want %d",
			generations,
			want,
		)
	}
	return nil
}

const maximumRootProtocolLineBytes = 64 * 1024

type rootProtocolObservation struct {
	mu            sync.Mutex
	pending       []byte
	templateLine  []byte
	generations   int
	keepalives    int
	previousEmpty bool
	changed       chan struct{}
}

func newRootProtocolObservation(templateID string) *rootProtocolObservation {
	return &rootProtocolObservation{
		templateLine: []byte("CONFIG " + templateID + " create "),
		changed:      make(chan struct{}, 1),
	}
}

func (rpo *rootProtocolObservation) observe(chunk []byte) error {
	rpo.mu.Lock()
	defer rpo.mu.Unlock()
	rpo.pending = append(rpo.pending, chunk...)
	for {
		newline := bytes.IndexByte(rpo.pending, '\n')
		if newline < 0 {
			if len(rpo.pending) > maximumRootProtocolLineBytes {
				return errors.New("jobmgr test: shipped-root protocol line exceeds bound")
			}
			return nil
		}
		line := rpo.pending[:newline]
		rpo.pending = rpo.pending[newline+1:]
		observed := true
		switch {
		case len(line) == 0:
			if rpo.previousEmpty {
				rpo.keepalives++
			} else {
				observed = false
			}
			rpo.previousEmpty = true
		case bytes.HasPrefix(line, []byte(`FUNCTION GLOBAL "config"`)),
			bytes.Equal(line, []byte(`FUNCTION_DEL GLOBAL "config"`)):
			return errors.New("jobmgr test: shipped root announced daemon-owned config Function")
		case bytes.HasPrefix(line, rpo.templateLine):
			rpo.previousEmpty = false
			rpo.generations++
		default:
			rpo.previousEmpty = false
			continue
		}
		if !observed {
			continue
		}
		select {
		case rpo.changed <- struct{}{}:
		default:
		}
	}
}

func (rpo *rootProtocolObservation) snapshot() (generations, keepalives int) {
	rpo.mu.Lock()
	defer rpo.mu.Unlock()
	return rpo.generations, rpo.keepalives
}

func (rpo *rootProtocolObservation) wait(
	ctx context.Context,
	process *runner.Process,
	predicate func(generations, keepalives int) bool,
) error {
	for {
		generations, keepalives := rpo.snapshot()
		if predicate(generations, keepalives) {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf(
				"jobmgr test: wait for shipped-root protocol lifecycle (%d generations): %w",
				generations,
				ctx.Err(),
			)
		case <-process.Done():
			generations, keepalives = rpo.snapshot()
			if predicate(generations, keepalives) {
				return nil
			}
			result, err := process.Wait(context.Background())
			return errors.Join(
				err,
				fmt.Errorf(
					"jobmgr test: shipped root exited before expected protocol lifecycle (%d generations), stderr=%q truncated=%t",
					generations,
					result.Stderr,
					result.StderrTruncated,
				),
			)
		case <-rpo.changed:
		}
	}
}
