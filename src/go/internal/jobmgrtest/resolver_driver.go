package jobmgrtest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
)

// ResolverDriver exercises the shipped default AtomicResolver and verifies
// that cancellation contains the complete command process group.
type ResolverDriver struct {
	Helper  string
	PIDFile string
}

type resolverResult struct {
	resolved any
	err      error
}

func (d ResolverDriver) Run(ctx context.Context) error {
	if ctx == nil ||
		d.Helper == "" ||
		d.PIDFile == "" {
		return errors.New("jobmgr test: invalid Resolver driver")
	}
	resolver, err := secretresolver.NewDefaultAtomicResolver()
	if err != nil {
		return err
	}
	input := map[string]any{
		"secret": "${cmd:" + d.Helper + " " + d.PIDFile + "}",
	}
	resolveCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	result := make(chan resolverResult, 1)
	go func() {
		resolved, err := resolver.Resolve(resolveCtx, input, nil)
		result <- resolverResult{resolved: resolved, err: err}
	}()
	var pidData []byte
	if err := waitUntil(ctx, func() bool {
		pidData, err = os.ReadFile(d.PIDFile)
		return err == nil && len(pidData) != 0
	}); err != nil {
		cancel()
		_, joinErr := waitResolverResult(result)
		return errors.Join(
			fmt.Errorf(
				"resolver helper did not publish PIDs: %w",
				err,
			),
			joinErr,
		)
	}
	cancel()
	outcome, err := waitResolverResult(result)
	if err != nil {
		return err
	}
	resolved, resolveErr := outcome.resolved, outcome.err
	if resolveErr == nil {
		return fmt.Errorf(
			"resolver command unexpectedly succeeded: %#v",
			resolved,
		)
	}
	if input["secret"] !=
		"${cmd:"+d.Helper+" "+d.PIDFile+"}" {
		return errors.New("resolver mutated caller input on cancellation")
	}
	fields := strings.Fields(string(pidData))
	if len(fields) != 2 {
		return fmt.Errorf("resolver helper PID record differs: %q", pidData)
	}
	pids := make([]int, len(fields))
	for index, field := range fields {
		pid, err := strconv.Atoi(field)
		if err != nil || pid <= 0 {
			return fmt.Errorf("resolver helper PID differs: %q", field)
		}
		pids[index] = pid
	}
	if err := waitUntil(ctx, func() bool {
		for _, pid := range pids {
			if !resolverProcessGone(pid) {
				return false
			}
		}
		return true
	}); err != nil {
		return fmt.Errorf(
			"resolver command process group remains live: %v: %w",
			pids,
			err,
		)
	}
	return nil
}

func waitResolverResult(
	result <-chan resolverResult,
) (resolverResult, error) {
	timer := time.NewTimer(fixtureJoinPeriod)
	defer timer.Stop()
	select {
	case outcome := <-result:
		return outcome, nil
	case <-timer.C:
		return resolverResult{}, errors.New(
			"resolver did not stop after cancellation",
		)
	}
}
