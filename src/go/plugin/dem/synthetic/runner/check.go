// SPDX-License-Identifier: GPL-3.0-or-later
package runner

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

func runtimeAllowed() error {
	if runtime.GOOS != "linux" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		return errors.New("DEM browser execution requires prepared Linux amd64 or arm64 runtime")
	}
	if os.Geteuid() == 0 {
		return errors.New(
			"DEM browser execution requires the plugin to run as an unprivileged user; root RUM operation does not enable browser jobs",
		)
	}
	return nil
}

func (e *Engine) checkFiles(kind synthetic.Kind, paths Paths) error {
	if kind != synthetic.Journey && kind != synthetic.Lighthouse {
		return errors.New("unsupported synthetic kind")
	}
	if paths.Identity.Contract != SupportedContract {
		return fmt.Errorf("unsupported runtime execution contract %d (Agent supports %d)", paths.Identity.Contract, SupportedContract)
	}
	digest, err := hex.DecodeString(paths.Identity.SHA256)
	if paths.Identity.Version == "" || err != nil || len(digest) != 32 {
		return errors.New("runtime source supplied an invalid trusted release identity")
	}
	for name, value := range map[string]string{"node": paths.Node, "entrypoint": paths.EntryPoint} {
		if !filepath.IsAbs(value) {
			return fmt.Errorf("prepared %s must be an absolute path", name)
		}
		info, err := os.Stat(value)
		if err != nil {
			return fmt.Errorf("prepared %s: %w", name, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("prepared %s is not a regular file", name)
		}
		if name == "node" && info.Mode().Perm()&0111 == 0 {
			return errors.New("prepared node is not executable")
		}
	}
	return nil
}

type checkOwner struct {
	mu     sync.Mutex
	flight *checkFlight
}

type checkFlight struct {
	done     chan struct{}
	cancel   context.CancelFunc
	waiters  int
	retiring bool
	err      error
}

// Check verifies the release contract through its own bounded supervised check.
// Component versions and private dependency checks belong to the runtime release.
// It never loads the configured workflow or launches a browser session.
func (e *Engine) Check(ctx context.Context, kind synthetic.Kind) error {
	if kind != synthetic.Journey && kind != synthetic.Lighthouse {
		return errors.New("unsupported synthetic kind")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if e.isPoisoned() {
		return ndexec.ErrTreeNotDrained
	}
	if err := e.platform(); err != nil {
		return err
	}
	waiterCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	owner := e.checks
	for {
		owner.mu.Lock()
		flight := owner.flight
		if flight != nil && flight.retiring {
			owner.mu.Unlock()
			// The last waiter owns cancellation and physical join. A replacement
			// cannot acquire another runtime use until that join is complete.
			select {
			case <-flight.done:
				continue
			case <-waiterCtx.Done():
				return waiterCtx.Err()
			case <-e.poisoned:
				return ndexec.ErrTreeNotDrained
			}
		}
		if err := waiterCtx.Err(); err != nil {
			owner.mu.Unlock()
			return err
		}
		if e.isPoisoned() {
			owner.mu.Unlock()
			return ndexec.ErrTreeNotDrained
		}
		if flight == nil {
			probeCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			flight = &checkFlight{done: make(chan struct{}), cancel: stop}
			owner.flight = flight
			go func(f *checkFlight) {
				err := e.check(probeCtx, kind)
				stop()
				owner.mu.Lock()
				f.err = err
				owner.flight = nil
				close(f.done)
				owner.mu.Unlock()
			}(flight)
		}
		flight.waiters++
		owner.mu.Unlock()
		var err error
		select {
		case <-flight.done:
			return flight.err
		case <-waiterCtx.Done():
			err = waiterCtx.Err()
		case <-e.poisoned:
			err = ndexec.ErrTreeNotDrained
		}
		owner.mu.Lock()
		// Completion may race cancellation. The caller still owns its deadline,
		// while the joined physical result remains available to other waiters.
		flight.waiters--
		last := flight.waiters == 0
		if last {
			flight.retiring = true
			flight.cancel()
		}
		owner.mu.Unlock()
		if last {
			<-flight.done
		}
		return err
	}
}

func (e *Engine) check(checkCtx context.Context, kind synthetic.Kind) error {
	var identity synthetic.RuntimeIdentity
	output, err := e.probe(checkCtx, kind, func(paths Paths) []string {
		identity = paths.Identity
		return []string{paths.EntryPoint, "--check"}
	})
	if err != nil {
		return fmt.Errorf("prepared runtime check: %w", err)
	}
	var actual struct {
		Contract int    `json:"contract"`
		Version  string `json:"version"`
		UID      *int   `json:"uid"`
		GID      *int   `json:"gid"`
	}
	if err := json.Unmarshal(output, &actual); err != nil {
		return errors.New("invalid prepared runtime check response")
	}
	if actual.Contract != identity.Contract || actual.Version != identity.Version {
		return errors.New("prepared runtime check does not match its trusted release identity")
	}
	if actual.UID == nil || actual.GID == nil || *actual.UID == 0 || *actual.UID != os.Geteuid() || *actual.GID != os.Getegid() {
		return errors.New("nd-run must preserve the plugin's unprivileged identity for private run directories")
	}
	return nil
}

func (e *Engine) beginUse() (Use, error) {
	if e.source == nil {
		return nil, errors.New("runtime source is unavailable")
	}
	return e.source.BeginUse()
}

func (e *Engine) probe(ctx context.Context, kind synthetic.Kind, arguments func(Paths) []string) ([]byte, error) {
	use, err := e.beginUse()
	if err != nil {
		return nil, err
	}
	drained := true
	defer func() { _ = use.Finish(drained) }()
	paths := use.Paths()
	if err := e.checkFiles(kind, paths); err != nil {
		return nil, err
	}

	var output []byte
	var parseErr error
	var diagnostic string
	result, err := e.executeProcess(ctx, paths, use.Completion(), arguments(paths), nil, func(reader io.Reader) error {
		output, parseErr = io.ReadAll(io.LimitReader(reader, 4097))
		if len(output) > 4096 {
			parseErr = errors.New("runtime check response exceeds 4096 bytes")
		}
		return parseErr
	}, func(reader io.Reader) {
		raw, _ := io.ReadAll(io.LimitReader(reader, 2001))
		diagnostic = clip(string(raw))
		_, _ = io.Copy(io.Discard, reader)
	})
	if err != nil {
		return nil, err
	}
	drained = result.Drained
	if !drained {
		return nil, ndexec.ErrTreeNotDrained
	}
	if parseErr != nil {
		return nil, parseErr
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if result.Err != nil {
		return nil, fmt.Errorf("runtime check failed: %w (%s)", result.Err, diagnostic)
	}
	return output, nil
}
