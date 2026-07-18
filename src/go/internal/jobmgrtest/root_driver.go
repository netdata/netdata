//go:build !windows

package jobmgrtest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/creack/pty"
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

func (driver ShippedRootDriver) Available(caseID string) bool {
	runs, err := shippedRootRuns(caseID)
	if err != nil {
		return false
	}
	for _, run := range runs {
		root, ok := driver.Roots[run.root]
		if !ok || !shippedRootAvailable(root) {
			return false
		}
	}
	return true
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

func (driver ShippedRootDriver) Run(
	ctx context.Context,
	caseID string,
) error {
	if ctx == nil {
		return errors.New("jobmgr test: nil shipped-root context")
	}
	runs, err := shippedRootRuns(caseID)
	if err != nil {
		return err
	}
	if !driver.Available(caseID) {
		return fmt.Errorf(
			"jobmgr test: shipped-root executables unavailable for %s",
			caseID,
		)
	}
	for _, run := range runs {
		if err := runShippedRoot(
			ctx,
			driver.Roots[run.root],
			run.scenario,
		); err != nil {
			return fmt.Errorf(
				"%s %s: %w",
				run.root,
				run.scenario,
				err,
			)
		}
	}
	return nil
}

// RunAvailable executes every supported physical root assigned to a case and
// returns the unavailable root names. Callers may retain partial local
// execution evidence, but a nonempty missing set cannot earn case credit.
func (driver ShippedRootDriver) RunAvailable(
	ctx context.Context,
	caseID string,
) ([]string, error) {
	if ctx == nil {
		return nil, errors.New(
			"jobmgr test: nil shipped-root context",
		)
	}
	runs, err := shippedRootRuns(caseID)
	if err != nil {
		return nil, err
	}
	var missing []string
	for _, run := range runs {
		root, ok := driver.Roots[run.root]
		if !ok || !shippedRootAvailable(root) {
			missing = append(missing, run.root)
			continue
		}
		if err := runShippedRoot(
			ctx,
			root,
			run.scenario,
		); err != nil {
			return missing, fmt.Errorf(
				"%s %s: %w",
				run.root,
				run.scenario,
				err,
			)
		}
	}
	return missing, nil
}

type shippedRootRun struct {
	root     string
	scenario string
}

func shippedRootRuns(caseID string) ([]shippedRootRun, error) {
	if caseID == "F06.1" {
		return []shippedRootRun{
			{root: "godplugin", scenario: "shutdown"},
			{root: "ibmdplugin", scenario: "shutdown"},
			{root: "scriptsdplugin", scenario: "shutdown"},
		}, nil
	}
	runs := map[string]shippedRootRun{
		"F24.20-b-godplugin-terminal": {
			root: "godplugin", scenario: "terminal",
		},
		"F24.21-b-godplugin-nonterminal": {
			root: "godplugin", scenario: "all-pipe",
		},
		"F24.22-b-godplugin-hup": {
			root: "godplugin", scenario: "repeated-hup",
		},
		"F24.23-b-ibmdplugin-terminal": {
			root: "ibmdplugin", scenario: "terminal",
		},
		"F24.24-b-ibmdplugin-nonterminal": {
			root: "ibmdplugin", scenario: "all-pipe",
		},
		"F24.25-b-ibmdplugin-hup": {
			root: "ibmdplugin", scenario: "repeated-hup",
		},
		"F24.26-b-scriptsdplugin-terminal": {
			root: "scriptsdplugin", scenario: "terminal",
		},
		"F24.27-b-scriptsdplugin-nonterminal": {
			root: "scriptsdplugin", scenario: "all-pipe",
		},
		"F24.28-b-scriptsdplugin-hup": {
			root: "scriptsdplugin", scenario: "repeated-hup",
		},
	}
	run, ok := runs[caseID]
	if !ok {
		return nil, fmt.Errorf(
			"jobmgr test: unknown shipped-root case %q",
			caseID,
		)
	}
	return []shippedRootRun{run}, nil
}

func runShippedRoot(
	ctx context.Context,
	root ShippedRoot,
	scenario string,
) error {
	observer := newRootProtocolObservation()
	var terminalMaster *os.File
	var terminalSlave *os.File
	var terminalDone chan struct{}
	if scenario == "terminal" {
		var err error
		terminalMaster, terminalSlave, err = pty.Open()
		if err != nil {
			return err
		}
		terminalDone = make(chan struct{})
		go func() {
			_, _ = io.Copy(io.Discard, terminalMaster)
			close(terminalDone)
		}()
	}
	spec := runner.Spec{
		Executable: root.Executable,
		Arguments: []string{
			"-m", root.Module,
			"-c", root.ConfigDir,
		},
		Directory:  root.ConfigDir,
		ObserveOut: observer.observe,
	}
	if terminalSlave != nil {
		spec.Stderr = terminalSlave
	}
	process, err := runner.Start(spec)
	if terminalSlave != nil {
		_ = terminalSlave.Close()
	}
	if err != nil {
		if terminalMaster != nil {
			_ = terminalMaster.Close()
			<-terminalDone
		}
		return err
	}
	defer process.Kill()
	defer func() {
		if terminalMaster != nil {
			_ = terminalMaster.Close()
			<-terminalDone
		}
	}()

	if err := observer.wait(ctx, func(
		publications, _ int,
	) bool {
		return publications >= 1
	}); err != nil {
		return err
	}
	switch scenario {
	case "terminal", "all-pipe":
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
				func(publications, withdrawals int) bool {
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
		publications, withdrawals := observer.snapshot()
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
	publications, withdrawals := observer.snapshot()
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
	return nil
}

const maximumRootProtocolLineBytes = 64 * 1024

type rootProtocolObservation struct {
	mu           sync.Mutex
	pending      []byte
	publications int
	withdrawals  int
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
		case bytes.HasPrefix(
			line,
			[]byte(`FUNCTION GLOBAL "config"`),
		):
			observation.publications++
		case bytes.Equal(
			line,
			[]byte(`FUNCTION_DEL GLOBAL "config"`),
		):
			observation.withdrawals++
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
	publications, withdrawals int,
) {
	observation.mu.Lock()
	defer observation.mu.Unlock()
	return observation.publications, observation.withdrawals
}

func (observation *rootProtocolObservation) wait(
	ctx context.Context,
	predicate func(publications, withdrawals int) bool,
) error {
	for {
		publications, withdrawals := observation.snapshot()
		if predicate(publications, withdrawals) {
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
