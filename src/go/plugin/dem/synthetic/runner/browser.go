// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

const browserIdleTimeout = time.Minute

// A process consumer and every candidate qualification get distinct affinity,
// even when their prepared paths and release identity happen to match.
type browserSource struct{ ephemeral bool }

type browserLoan struct {
	OwnerID  string `json:"owner_id"`
	Epoch    string `json:"epoch"`
	Endpoint string `json:"endpoint"`
}

type browserReport struct {
	OwnerID string `json:"owner_id"`
	Epoch   string `json:"epoch"`
	State   string `json:"state"`
}

func (l browserLoan) redactor(secrets map[string]string) *textRedactor {
	endpoint, _ := url.Parse(l.Endpoint) // Validated during owner readiness.
	return newRedactor(secrets, l.Endpoint, endpoint.EscapedPath()[1:])
}

type browserOwner struct {
	scope     *browserSource
	paths     Paths
	id        string
	endpoint  string
	epoch     uint64
	stop      chan struct{}
	force     context.CancelFunc
	exited    chan struct{}
	done      chan struct{}
	unhealthy atomic.Bool
	result    ndexec.TreeResult
}

func (o *browserOwner) alive() bool {
	if o.unhealthy.Load() {
		return false
	}
	select {
	case <-o.exited:
		return false
	default:
		return true
	}
}

// The slot owns this call exclusively. EOF requests graceful shutdown; only
// the supervisor can attest physical drainage after graceful or forced stop.
func (o *browserOwner) retire() ndexec.TreeResult {
	close(o.stop)
	timer := time.NewTimer(cleanupTimeout)
	defer timer.Stop()
	select {
	case <-o.done:
		return o.result
	case <-timer.C:
		o.force()
		<-o.done
		return o.result
	}
}

func (slot *browserSlot) retireOwner() bool {
	if slot.owner == nil {
		return true
	}
	if !slot.owner.retire().Drained {
		return false
	}
	slot.owner = nil
	return true
}

func (e *Engine) prepareBrowser(ctx context.Context, slot *browserSlot, scope *browserSource, paths Paths) (*browserLoan, error) {
	if owner := slot.owner; owner != nil &&
		(scope == nil || owner.scope != scope || owner.paths != paths || !owner.alive()) {
		if !slot.retireOwner() {
			return nil, ndexec.ErrTreeNotDrained
		}
	}
	if scope == nil {
		return nil, nil
	}
	if slot.owner == nil {
		owner, err := e.startBrowserOwner(ctx, paths)
		slot.owner = owner
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				err = ctxErr
			}
			if !slot.retireOwner() {
				return nil, ndexec.ErrTreeNotDrained
			}
			return nil, err
		}
	}
	slot.owner.epoch++
	return &browserLoan{OwnerID: slot.owner.id, Epoch: strconv.FormatUint(slot.owner.epoch, 10), Endpoint: slot.owner.endpoint}, nil
}

// The caller has already joined the CLI supervisor and all its local readers.
func (slot *browserSlot) finishLoan(executionErr error, snapshot sinkSnapshot) (bool, error) {
	var browserErr error
	ephemeral := slot.owner.scope.ephemeral
	switch {
	case !slot.owner.alive():
		browserErr = errors.New("browser owner exited during execution")
	case snapshot.browser == nil || snapshot.browser.State == "retire":
		browserErr = errors.New("managed browser cleanup was not verified")
	case ephemeral && snapshot.browser.State != "clean":
		browserErr = errors.New("runtime qualification did not use the managed browser")
	}
	terminalTimeout := snapshot.result != nil &&
		(snapshot.result.Status == synthetic.Timeout || snapshot.result.Status == synthetic.Cancelled)
	if browserErr != nil || snapshot.err != nil || snapshot.result == nil || terminalTimeout || executionErr != nil || ephemeral {
		return slot.retireOwner(), browserErr
	}
	return true, nil
}

// A browser owner protects its runtime independently of every attempt. No
// request secrets, attempt directory or caller context enter its lifetime.
func (e *Engine) startBrowserOwner(ctx context.Context, paths Paths) (*browserOwner, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	use, err := e.beginUse()
	if err != nil {
		return nil, err
	}
	if use.Paths() != paths {
		_ = use.Finish(true)
		return nil, errors.New("runtime source changed while starting a browser owner")
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		_ = use.Finish(true)
		return nil, err
	}
	work, err := os.MkdirTemp("/tmp", "nd-dem-owner-")
	if err != nil {
		_ = use.Finish(true)
		return nil, err
	}
	lifetime, force := context.WithCancel(context.Background())
	owner := &browserOwner{
		scope:  e.browserSource,
		paths:  paths,
		id:     hex.EncodeToString(random[:]),
		stop:   make(chan struct{}),
		force:  force,
		exited: make(chan struct{}),
		done:   make(chan struct{}),
	}
	request := struct {
		Version int    `json:"version"`
		OwnerID string `json:"owner_id"`
		WorkDir string `json:"work_dir"`
	}{Version: SupportedContract, OwnerID: owner.id, WorkDir: work}
	ready := make(chan string, 1)
	go func() {
		defer close(owner.done)
		defer force()
		input := func(w *os.File, exited <-chan struct{}) error {
			if err := json.NewEncoder(w).Encode(request); err != nil {
				return err
			}
			select {
			case <-owner.stop:
			case <-exited:
			}
			return nil
		}
		result, startErr := e.executeProcessWithInput(lifetime, paths, use.Completion(), []string{paths.EntryPoint, "--browser-owner"}, input, func(r io.Reader) error {
			defer owner.unhealthy.Store(true)
			return readOwnerReady(r, owner.id, ready)
		}, func(r io.Reader) { _, _ = io.Copy(io.Discard, r) }, owner.exited)
		owner.result = result
		if result.Drained {
			_ = os.RemoveAll(work)
		}
		_ = use.Finish(result.Drained)
		if startErr != nil {
			owner.result.Err = startErr
		}
	}()
	select {
	case endpoint := <-ready:
		if !owner.alive() {
			return owner, errors.New("browser owner exited during startup")
		}
		owner.endpoint = endpoint
		return owner, nil
	case <-owner.exited:
		return owner, errors.New("browser owner exited before readiness")
	case <-ctx.Done():
		return owner, ctx.Err()
	}
}

func readOwnerReady(r io.Reader, id string, ready chan<- string) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024), 4096)
	if !scanner.Scan() {
		return errors.New("browser owner did not report readiness")
	}
	var response struct {
		Type     string `json:"type"`
		OwnerID  string `json:"owner_id"`
		Endpoint string `json:"endpoint"`
	}
	decoder := json.NewDecoder(strings.NewReader(scanner.Text()))
	decoder.DisallowUnknownFields()
	var extra any
	if decoder.Decode(&response) != nil || decoder.Decode(&extra) != io.EOF ||
		response.Type != "ready" || response.OwnerID != id || !validBrowserEndpoint(response.Endpoint) {
		return errors.New("invalid browser owner readiness")
	}
	ready <- response.Endpoint
	if scanner.Scan() || scanner.Err() != nil {
		return errors.New("unexpected browser owner output")
	}
	return nil
}

func validBrowserEndpoint(value string) bool {
	endpoint, err := url.Parse(value)
	if err != nil {
		return false
	}
	port, err := strconv.ParseUint(endpoint.Port(), 10, 16)
	return err == nil && port > 0 && endpoint.Scheme == "ws" && endpoint.Hostname() == "127.0.0.1" &&
		endpoint.User == nil && endpoint.RawQuery == "" && !endpoint.ForceQuery && endpoint.Fragment == "" &&
		len(endpoint.Path) > 1
}

// Run fences new grants at process shutdown. Existing monitor and Function
// callers retain their cancellation and join behavior.
func (e *Engine) Run(ctx context.Context) {
	<-ctx.Done()
	e.mu.Lock()
	e.stopping = true
	e.mu.Unlock()
}

// Close follows joined callers. It retires all idle owners in parallel and
// waits for their runtime uses to finish before the caller closes shared stores.
func (e *Engine) Close(ctx context.Context) error {
	e.closeOnce.Do(func() {
		e.mu.Lock()
		e.stopping = true
		close(e.closing)
		for entry := e.idle.Front(); entry != nil; {
			next := entry.Next()
			if slot := entry.Value.(*browserSlot); slot.owner != nil {
				e.retireIdle(slot)
			}
			entry = next
		}
		e.mu.Unlock()
		go func() {
			e.busy.Wait()
			close(e.closed)
		}()
	})
	select {
	case <-e.closed:
		if e.isPoisoned() {
			return ndexec.ErrTreeNotDrained
		}
		return nil
	case <-e.poisoned:
		return ndexec.ErrTreeNotDrained
	case <-ctx.Done():
		return errors.Join(ndexec.ErrTreeNotDrained, ctx.Err())
	}
}
