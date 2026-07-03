// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"io"
	"sync"
)

// emissionGateway wraps a job's output writer so the "is output allowed"
// check and the write happen under one lock: Close waits out any in-flight
// write, and no write that starts after Close returns can reach the wire.
// A plain flag or writer swap cannot give this guarantee - a write that
// already passed its check could still flush afterwards.
//
// The gate is installed on every created job and stays open for the job's
// whole life on the normal path; nothing closes it in the current runtime.
type emissionGateway struct {
	mu         sync.Mutex
	out        io.Writer
	closed     bool
	suppressed int
}

func newEmissionGateway(out io.Writer) *emissionGateway {
	if out == nil {
		out = io.Discard
	}
	return &emissionGateway{out: out}
}

func (g *emissionGateway) Write(p []byte) (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		g.suppressed++
		return len(p), nil
	}
	return g.out.Write(p)
}

// Close shuts the gate. It blocks until any in-flight write completes; every
// later write is suppressed and counted instead of reaching the writer.
func (g *emissionGateway) Close() {
	g.mu.Lock()
	g.closed = true
	g.mu.Unlock()
}

// SuppressedWrites reports how many writes were swallowed after Close.
func (g *emissionGateway) SuppressedWrites() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.suppressed
}

// emissionGates tracks the gateway of the most recently created job per full
// name (jobs are created and stopped on the manager loop; validation-only
// factory clones register nothing). Entries are removed when the job stops.
type emissionGates struct {
	mu    sync.Mutex
	items map[string]*emissionGateway
}

func newEmissionGates() *emissionGates {
	return &emissionGates{items: make(map[string]*emissionGateway)}
}

func (s *emissionGates) add(name string, gate *emissionGateway) {
	if gate == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[name] = gate
}

func (s *emissionGates) remove(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, name)
}

func (s *emissionGates) lookup(name string) (*emissionGateway, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	gate, ok := s.items[name]
	return gate, ok
}
