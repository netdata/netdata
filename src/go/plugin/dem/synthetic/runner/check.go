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

// checkTimeout bounds each caller's wait and each runtime contract probe.
const checkTimeout = 10 * time.Second

// checkReplyLimit caps the stdout read from `run.mjs --check`. Its reply is
// one JSON object with the release contract, version, uid and gid
// (src/dem-runtime/runner/runtime.mjs checkRuntime); the cap keeps a
// misbehaving runtime from growing Agent memory.
const checkReplyLimit = 4096

// A failing check writes {"error": text} to stderr with text clipped to
// textLimit characters (src/dem-runtime/runner/protocol.cjs). The Agent reads
// one byte past textLimit and clips the result to textLimit characters for the
// check error's diagnostic.
const checkDiagnosticLimit = textLimit + 1

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

// checkFiles verifies the acquired release identity and its executable files
// before anything is launched.
func checkFiles(paths Paths) error {
	if paths.Identity.Contract != synthetic.ExecutionContract {
		return fmt.Errorf("unsupported runtime execution contract %d (Agent supports %d)", paths.Identity.Contract, synthetic.ExecutionContract)
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

// Check verifies the release contract through its own bounded supervised check.
// Component versions and private dependency checks belong to the runtime release.
// It never loads a workflow or launches a browser session.
func (e *Engine) Check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if e.isPoisoned() {
		return ndexec.ErrTreeNotDrained
	}
	if err := e.platform(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	return e.checks.do(ctx, e.poisoned, e.check)
}

// checkOwner coalesces the concurrent checks of one source view into one
// probe. Probes run outside browser slots, so this also keeps at most one
// probe per view running.
type checkOwner struct {
	mu     sync.Mutex
	flight *checkFlight
}

type checkFlight struct {
	done   chan struct{} // closed once the probe returned and its tree joined
	cancel context.CancelFunc
	// waiters counts callers still waiting. A flight left with none is
	// retiring: its last caller cancelled it and waits for its join.
	waiters int
	err     error
}

// do returns the result of a probe shared with concurrent callers. Each caller
// keeps its own deadline. The last caller to leave cancels the probe and
// returns only after it joined, so its runtime use is released by then, and no
// replacement probe starts before that join.
func (o *checkOwner) do(ctx context.Context, poisoned <-chan struct{}, probe func(context.Context) error) error {
	for {
		o.mu.Lock()
		f := o.flight
		if f != nil && f.waiters == 0 {
			o.mu.Unlock()
			if err := awaitCheck(ctx, poisoned, f.done); err != nil {
				return err
			}
			continue
		}
		if err := ctx.Err(); err != nil {
			o.mu.Unlock()
			return err
		}
		if closed(poisoned) {
			o.mu.Unlock()
			return ndexec.ErrTreeNotDrained
		}
		if f == nil {
			f = o.start(ctx, probe)
		}
		f.waiters++
		o.mu.Unlock()
		if err := awaitCheck(ctx, poisoned, f.done); err != nil {
			// Completion may race this departure: the caller still reports its
			// own stop, while other waiters keep the joined result.
			return o.leave(f, err)
		}
		return f.err
	}
}

// start launches a probe under o.mu. The probe outlives its first caller's
// cancellation and stops at checkTimeout or when its last caller leaves.
func (o *checkOwner) start(ctx context.Context, probe func(context.Context) error) *checkFlight {
	probeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), checkTimeout)
	f := &checkFlight{done: make(chan struct{}), cancel: cancel}
	o.flight = f
	go func() {
		err := probe(probeCtx)
		cancel()
		o.mu.Lock()
		f.err, o.flight = err, nil
		close(f.done)
		o.mu.Unlock()
	}()
	return f
}

func (o *checkOwner) leave(f *checkFlight, err error) error {
	o.mu.Lock()
	f.waiters--
	last := f.waiters == 0
	if last {
		f.cancel()
	}
	o.mu.Unlock()
	if last {
		<-f.done
		// An undrained probe may still hold the runtime; callers must see it.
		if errors.Is(f.err, ndexec.ErrTreeNotDrained) {
			return errors.Join(err, f.err)
		}
	}
	return err
}

func awaitCheck(ctx context.Context, poisoned, done <-chan struct{}) error {
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-poisoned:
		return ndexec.ErrTreeNotDrained
	}
}

func closed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func (e *Engine) check(checkCtx context.Context) error {
	var identity synthetic.RuntimeIdentity
	output, err := e.probe(checkCtx, func(paths Paths) []string {
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

func (e *Engine) probe(ctx context.Context, arguments func(Paths) []string) ([]byte, error) {
	use, err := e.beginUse()
	if err != nil {
		return nil, err
	}
	drained := true
	defer func() { _ = use.Finish(drained) }()
	paths := use.Paths()
	if err := checkFiles(paths); err != nil {
		return nil, err
	}

	var output []byte
	var parseErr error
	var diagnostic string
	result, err := e.executeProcess(ctx, paths, use.Completion(), arguments(paths), nil, func(reader io.Reader) error {
		output, parseErr = io.ReadAll(io.LimitReader(reader, checkReplyLimit+1))
		if len(output) > checkReplyLimit {
			parseErr = fmt.Errorf("runtime check response exceeds %d bytes", checkReplyLimit)
		}
		return parseErr
	}, func(reader io.Reader) {
		raw, _ := io.ReadAll(io.LimitReader(reader, checkDiagnosticLimit))
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
