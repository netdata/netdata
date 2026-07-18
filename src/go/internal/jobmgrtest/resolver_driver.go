package jobmgrtest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
)

// ResolverDriver exercises the shipped default AtomicResolver and verifies
// that cancellation contains the complete command process group.
type ResolverDriver struct {
	Helper  string
	PIDFile string
}

func (driver ResolverDriver) Run(
	ctx context.Context,
	scenario string,
) error {
	if ctx == nil ||
		scenario != "process-group-containment" ||
		driver.Helper == "" ||
		driver.PIDFile == "" {
		return errors.New("jobmgr test: invalid Resolver driver")
	}
	resolver, err := secretresolver.NewDefaultAtomicResolver()
	if err != nil {
		return err
	}
	input := map[string]any{
		"secret": "${cmd:" + driver.Helper + " " + driver.PIDFile + "}",
	}
	resolveCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	type resolveResult struct {
		resolved any
		err      error
	}
	result := make(chan resolveResult, 1)
	go func() {
		resolved, err := resolver.Resolve(resolveCtx, input, nil)
		result <- resolveResult{resolved: resolved, err: err}
	}()
	var pidData []byte
	if err := waitUntil(ctx, func() bool {
		pidData, err = os.ReadFile(driver.PIDFile)
		return err == nil && len(pidData) != 0
	}); err != nil {
		cancel()
		return fmt.Errorf(
			"resolver helper did not publish PIDs: %w",
			err,
		)
	}
	cancel()
	var resolved any
	var resolveErr error
	select {
	case outcome := <-result:
		resolved, resolveErr = outcome.resolved, outcome.err
	case <-ctx.Done():
		return fmt.Errorf("resolver did not stop after cancellation: %w", ctx.Err())
	}
	if resolveErr == nil {
		return fmt.Errorf(
			"resolver command unexpectedly succeeded: %#v",
			resolved,
		)
	}
	if input["secret"] !=
		"${cmd:"+driver.Helper+" "+driver.PIDFile+"}" {
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
