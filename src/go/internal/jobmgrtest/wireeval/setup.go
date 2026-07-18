package wireeval

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/contract"
)

const maximumSetupOutputBytes = 4 * 1024 * 1024

type setupTracker struct {
	mu            sync.Mutex
	pending       []byte
	functions     [contract.PerformanceFixtureCount]bool
	configCreated [contract.PerformanceFixtureCount]bool
	configRunning [contract.PerformanceFixtureCount]bool
	functionCount int
	createdCount  int
	runningCount  int
	complete      bool
}

func (tracker *setupTracker) Feed(payload []byte) (bool, error) {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	if len(payload) > maximumSetupOutputBytes-len(tracker.pending) {
		return false, errors.New("wire evaluator: setup output exceeds bound")
	}
	tracker.pending = append(tracker.pending, payload...)
	becameComplete := false
	for {
		end := bytes.Index(tracker.pending, []byte("\n\n"))
		if end < 0 {
			return becameComplete, nil
		}
		frame := tracker.pending[:end]
		tracker.pending = tracker.pending[end+2:]
		for _, line := range bytes.Split(frame, []byte{'\n'}) {
			if err := tracker.observeLine(string(line)); err != nil {
				return false, err
			}
		}
		if !tracker.complete && tracker.functionCount == contract.PerformanceFixtureCount &&
			tracker.createdCount == contract.PerformanceFixtureCount && tracker.runningCount == contract.PerformanceFixtureCount {
			tracker.complete = true
			becameComplete = true
		}
	}
}

func (tracker *setupTracker) Status() string {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	missing := -1
	for ordinal := range contract.PerformanceFixtureCount {
		if !tracker.configCreated[ordinal] || !tracker.configRunning[ordinal] {
			missing = ordinal
			break
		}
	}
	return fmt.Sprintf(
		"functions=%d/%d created=%d/%d running=%d/%d missing=%d pending=%d",
		tracker.functionCount,
		contract.PerformanceFixtureCount,
		tracker.createdCount,
		contract.PerformanceFixtureCount,
		tracker.runningCount,
		contract.PerformanceFixtureCount,
		missing,
		len(tracker.pending),
	)
}

func (tracker *setupTracker) observeLine(line string) error {
	const functionPrefix = `FUNCTION GLOBAL "`
	if strings.HasPrefix(line, functionPrefix) {
		rest := line[len(functionPrefix):]
		end := strings.IndexByte(rest, '"')
		if end < 0 {
			return errors.New("wire evaluator: malformed Function setup record")
		}
		name := rest[:end]
		if strings.HasPrefix(name, "perf:work-") {
			ordinal, err := setupOrdinal(name, "perf:work-")
			if err != nil {
				return err
			}
			if tracker.functions[ordinal] {
				return fmt.Errorf("wire evaluator: duplicate setup Function %s", name)
			}
			tracker.functions[ordinal] = true
			tracker.functionCount++
		}
		return nil
	}

	const configPrefix = "CONFIG "
	if !strings.HasPrefix(line, configPrefix) {
		return nil
	}
	rest := line[len(configPrefix):]
	space := strings.IndexByte(rest, ' ')
	if space < 0 {
		return nil
	}
	id := rest[:space]
	const fixturePrefix = "poc:collector:perf:job-"
	if !strings.HasPrefix(id, fixturePrefix) {
		return nil
	}
	ordinal, err := setupOrdinal(id, fixturePrefix)
	if err != nil {
		return err
	}
	action := rest[space+1:]
	switch {
	case strings.HasPrefix(action, "create accepted "):
		if tracker.configCreated[ordinal] {
			return fmt.Errorf("wire evaluator: duplicate setup config create %s", id)
		}
		tracker.configCreated[ordinal] = true
		tracker.createdCount++
	case strings.HasPrefix(action, "create running "):
		if tracker.configCreated[ordinal] || tracker.configRunning[ordinal] {
			return fmt.Errorf("wire evaluator: duplicate setup config create %s", id)
		}
		tracker.configCreated[ordinal] = true
		tracker.configRunning[ordinal] = true
		tracker.createdCount++
		tracker.runningCount++
	case action == "status running":
		if !tracker.configCreated[ordinal] {
			return fmt.Errorf("wire evaluator: setup config running precedes create %s", id)
		}
		if tracker.configRunning[ordinal] {
			return fmt.Errorf("wire evaluator: duplicate setup config running %s", id)
		}
		tracker.configRunning[ordinal] = true
		tracker.runningCount++
	default:
		return fmt.Errorf("wire evaluator: unexpected setup config record %q", line)
	}
	return nil
}

func setupOrdinal(value, prefix string) (int, error) {
	suffix := strings.TrimPrefix(value, prefix)
	if len(suffix) != 3 {
		return 0, fmt.Errorf("wire evaluator: unknown setup identity %q", value)
	}
	ordinal, err := strconv.Atoi(suffix)
	if err != nil || ordinal < 0 || ordinal >= contract.PerformanceFixtureCount {
		return 0, fmt.Errorf("wire evaluator: unknown setup identity %q", value)
	}
	fixture, err := contract.Fixture(ordinal)
	if err != nil || value != prefix+fixture.KeyToken {
		return 0, fmt.Errorf("wire evaluator: unknown setup identity %q", value)
	}
	return ordinal, nil
}
