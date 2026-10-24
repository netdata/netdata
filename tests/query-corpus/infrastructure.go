// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
)

type infrastructureLedger struct {
	mu     sync.Mutex
	failed map[string]struct{}
}

func newInfrastructureLedger() *infrastructureLedger {
	return &infrastructureLedger{failed: make(map[string]struct{})}
}

func (l *infrastructureLedger) record(phase string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failed[phase] = struct{}{}
}

func (l *infrastructureLedger) phases() []string {
	l.mu.Lock()
	defer l.mu.Unlock()

	phases := make([]string, 0, len(l.failed))
	for phase := range l.failed {
		phases = append(phases, phase)
	}
	sort.Strings(phases)
	return phases
}

func (l *infrastructureLedger) run(phase string, fn func() error) error {
	err := fn()
	if err != nil {
		l.record(phase)
	}
	return err
}

func (l *infrastructureLedger) summary() (report string, failed bool) {
	phases := l.phases()
	if len(phases) == 0 {
		return "", false
	}

	var b strings.Builder
	fmt.Fprintf(&b, "\nquery corpus infrastructure: %d phase(s) FAILED\n", len(phases))
	for _, phase := range phases {
		fmt.Fprintf(&b, "  FAILED  %s\n", phase)
	}
	fmt.Fprintln(&b, "\nInfrastructure failures are not query-engine contract verdicts.")
	return b.String(), true
}

type infrastructureSetupTracker struct {
	ledger    *infrastructureLedger
	phase     string
	failed    func() bool
	completed bool
}

func newInfrastructureSetupTracker(
	ledger *infrastructureLedger,
	phase string,
	failed func() bool,
) *infrastructureSetupTracker {
	return &infrastructureSetupTracker{
		ledger: ledger,
		phase:  phase,
		failed: failed,
	}
}

func (s *infrastructureSetupTracker) complete() {
	if s.completed {
		return
	}
	if s.failed() {
		s.ledger.record(s.phase)
	}
	s.completed = true
}

func (s *infrastructureSetupTracker) finish() {
	if !s.completed {
		s.ledger.record(s.phase)
	}
}

func trackInfrastructureSetup(t *testing.T, ledger *infrastructureLedger, phase string) func() {
	t.Helper()

	// Call the returned completion function after the setup proof and before
	// query-contract assertions, while t.Failed() still describes setup only.
	tracker := newInfrastructureSetupTracker(ledger, phase, t.Failed)
	t.Cleanup(func() {
		if !t.Skipped() {
			tracker.finish()
		}
	})
	return tracker.complete
}

var infrastructureFailures = newInfrastructureLedger()
