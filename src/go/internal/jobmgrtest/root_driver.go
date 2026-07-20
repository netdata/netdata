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

	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/runner"
)

type ShippedRoot struct {
	Executable string
	Module     string
	ConfigDir  string
}

// ShippedRootDriver executes the real command roots. It accepts only prebuilt
// executables so the runtime transcript cannot accidentally exercise a `go
// run` wrapper or an unsupported IBM stub.
type ShippedRootDriver struct {
	Roots map[string]ShippedRoot
}

func shippedRootAvailable(root ShippedRoot) bool {
	if !filepath.IsAbs(root.Executable) ||
		!filepath.IsAbs(root.ConfigDir) ||
		root.Module == "" {
		return false
	}
	info, err := os.Stat(root.Executable)
	return err == nil &&
		info.Mode().IsRegular() &&
		info.Mode()&0o111 != 0
}

var shippedRootNames = [...]string{
	"godplugin",
	"ibmdplugin",
	"scriptsdplugin",
}

var shippedRootScenarios = [...]string{
	"quit",
	"repeated-hup",
	"shutdown",
}

// RunMatrixAvailable executes every supported physical root under each
// production-relevant process lifecycle.
func (driver ShippedRootDriver) RunMatrixAvailable(
	ctx context.Context,
) ([]string, error) {
	if ctx == nil {
		return nil, errors.New(
			"jobmgr test: nil shipped-root matrix context",
		)
	}
	var missing []string
	for _, name := range shippedRootNames {
		root, ok := driver.Roots[name]
		if !ok || !shippedRootAvailable(root) {
			missing = append(missing, name)
			continue
		}
		for _, scenario := range shippedRootScenarios {
			if err := runShippedRoot(ctx, root, scenario); err != nil {
				return missing, fmt.Errorf(
					"%s %s: %w",
					name,
					scenario,
					err,
				)
			}
		}
	}
	return missing, nil
}

func runShippedRoot(
	ctx context.Context,
	root ShippedRoot,
	scenario string,
) error {
	observer := newRootProtocolObservation()
	spec := runner.Spec{
		Executable: root.Executable,
		Arguments: []string{
			"-m", root.Module,
			"-c", root.ConfigDir,
		},
		Directory:  root.ConfigDir,
		ObserveOut: observer.observe,
	}
	process, err := runner.Start(spec)
	if err != nil {
		return err
	}
	defer process.Kill()

	if err := observer.wait(ctx, func(
		publications, _, _ int,
	) bool {
		return publications >= 1
	}); err != nil {
		return err
	}
	switch scenario {
	case "quit":
		_, _, keepalives := observer.snapshot()
		if err := observer.wait(ctx, func(
			_, _, observedKeepalives int,
		) bool {
			return observedKeepalives > keepalives
		}); err != nil {
			return fmt.Errorf(
				"nonterminal root emitted no process keepalive: %w",
				err,
			)
		}
		if _, err := process.WriteContext(
			ctx,
			[]byte("QUIT\n"),
		); err != nil {
			return err
		}
	case "shutdown":
		if err := process.Signal(os.Signal(syscallSIGTERM())); err != nil {
			return err
		}
	case "repeated-hup":
		for index := range 3 {
			if err := process.Signal(os.Signal(
				syscallSIGHUP(),
			)); err != nil {
				return err
			}
			if err := observer.wait(
				ctx,
				func(publications, withdrawals, _ int) bool {
					return publications >= index+2 &&
						withdrawals >= index+1
				},
			); err != nil {
				return err
			}
		}
		if _, err := process.WriteContext(
			ctx,
			[]byte("QUIT\n"),
		); err != nil {
			return err
		}
	default:
		return fmt.Errorf(
			"jobmgr test: unknown shipped-root scenario %q",
			scenario,
		)
	}
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := process.Wait(waitCtx)
	if err != nil {
		publications, withdrawals, _ := observer.snapshot()
		return errors.Join(
			err,
			fmt.Errorf(
				"publications=%d withdrawals=%d stderr=%q",
				publications,
				withdrawals,
				result.Stderr,
			),
		)
	}
	publications, withdrawals, _ := observer.snapshot()
	want := 1
	if scenario == "repeated-hup" {
		want = 4
	}
	if publications != want || withdrawals != want {
		return fmt.Errorf(
			"jobmgr test: shipped-root protocol lifecycle publications=%d withdrawals=%d, want %d/%d",
			publications,
			withdrawals,
			want,
			want,
		)
	}
	if scenario == "repeated-hup" {
		if err := observer.validateAlternatingLifecycle(); err != nil {
			return err
		}
	}
	return nil
}

const maximumRootProtocolLineBytes = 64 * 1024

type rootProtocolObservation struct {
	mu           sync.Mutex
	pending      []byte
	publications int
	withdrawals  int
	keepalives   int
	lifecycle    []byte
	changed      chan struct{}
}

func newRootProtocolObservation() *rootProtocolObservation {
	return &rootProtocolObservation{changed: make(chan struct{}, 1)}
}

func (observation *rootProtocolObservation) observe(
	chunk []byte,
	_ int64,
) error {
	observation.mu.Lock()
	defer observation.mu.Unlock()
	observation.pending = append(observation.pending, chunk...)
	for {
		newline := bytes.IndexByte(observation.pending, '\n')
		if newline < 0 {
			if len(observation.pending) > maximumRootProtocolLineBytes {
				return errors.New(
					"jobmgr test: shipped-root protocol line exceeds bound",
				)
			}
			return nil
		}
		line := observation.pending[:newline]
		observation.pending = observation.pending[newline+1:]
		switch {
		case len(line) == 0:
			observation.keepalives++
		case bytes.HasPrefix(
			line,
			[]byte(`FUNCTION GLOBAL "config"`),
		):
			observation.publications++
			observation.lifecycle = append(
				observation.lifecycle,
				'P',
			)
		case bytes.Equal(
			line,
			[]byte(`FUNCTION_DEL GLOBAL "config"`),
		):
			observation.withdrawals++
			observation.lifecycle = append(
				observation.lifecycle,
				'D',
			)
		default:
			continue
		}
		select {
		case observation.changed <- struct{}{}:
		default:
		}
	}
}

func (observation *rootProtocolObservation) snapshot() (
	publications, withdrawals, keepalives int,
) {
	observation.mu.Lock()
	defer observation.mu.Unlock()
	return observation.publications,
		observation.withdrawals,
		observation.keepalives
}

func (observation *rootProtocolObservation) wait(
	ctx context.Context,
	predicate func(publications, withdrawals, keepalives int) bool,
) error {
	for {
		publications, withdrawals, keepalives := observation.snapshot()
		if predicate(publications, withdrawals, keepalives) {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf(
				"jobmgr test: wait for shipped-root protocol lifecycle (%d publications, %d withdrawals): %w",
				publications,
				withdrawals,
				ctx.Err(),
			)
		case <-observation.changed:
		}
	}
}

func (observation *rootProtocolObservation) validateAlternatingLifecycle() error {
	observation.mu.Lock()
	defer observation.mu.Unlock()
	if string(observation.lifecycle) != "PDPDPDPD" {
		return fmt.Errorf(
			"jobmgr test: shipped-root HUP lifecycle order=%q, want %q",
			observation.lifecycle,
			"PDPDPDPD",
		)
	}
	return nil
}
