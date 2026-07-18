package wireeval

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"testing"

	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/oracle"
)

func TestExperimentPreloadsUniqueEntropyAndSharesOneNonceAcrossPairSides(t *testing.T) {
	reads := 0
	readFull := func(destination []byte) (int, error) {
		reads++
		for index := range destination {
			destination[index] = byte(reads)
		}
		binary.BigEndian.PutUint64(destination[8:], uint64(reads))
		return len(destination), nil
	}
	stop := errors.New("stop after first pair")
	var runs []RunSpec
	_, err := runExperiment(context.Background(), ExperimentSpec{
		Baseline: ChildSpec{Executable: "/current"}, Production: ChildSpec{Executable: "/candidate"},
	}, experimentDeps{
		readFull: readFull,
		deriveEnvironment: func(ChildSpec, ChildSpec) (string, error) {
			return testEnvironmentDigest, nil
		},
		run: func(_ context.Context, spec RunSpec) (oracle.RunSummary, error) {
			runs = append(runs, spec)
			if len(runs) == 2 {
				return oracle.RunSummary{}, stop
			}
			return oracle.RunSummary{}, nil
		},
	})
	if !errors.Is(err, stop) {
		t.Fatalf("experiment stop result differs: %v", err)
	}
	if reads != 135 {
		t.Fatalf("entropy reads=%d, want 135 before first child", reads)
	}
	if len(runs) != 2 || runs[0].WorkloadID != runs[1].WorkloadID || runs[0].Population != runs[1].Population ||
		runs[0].PairIndex != runs[1].PairIndex || runs[0].Side == runs[1].Side ||
		runs[0].PairNonce != runs[1].PairNonce || runs[0].Schedule == nil ||
		runs[0].Schedule != runs[1].Schedule {
		t.Fatalf("first pair did not share one exact nonce: %#v", runs)
	}
	if err := runs[0].Schedule.Validate(runs[0].WorkloadID, runs[0].PairNonce); err != nil {
		t.Fatalf("first pair schedule differs: %v", err)
	}
}

func TestExperimentEntropyFailurePrecedesEveryChildLaunch(t *testing.T) {
	sentinel := errors.New("entropy failed")
	reads := 0
	runs := 0
	_, err := runExperiment(context.Background(), ExperimentSpec{
		Baseline: ChildSpec{Executable: "/current"}, Production: ChildSpec{Executable: "/candidate"},
	}, experimentDeps{
		readFull: func(destination []byte) (int, error) {
			reads++
			if reads == 135 {
				return 8, sentinel
			}
			binary.BigEndian.PutUint64(destination[8:], uint64(reads))
			return len(destination), nil
		},
		deriveEnvironment: func(ChildSpec, ChildSpec) (string, error) {
			return testEnvironmentDigest, nil
		},
		run: func(context.Context, RunSpec) (oracle.RunSummary, error) {
			runs++
			return oracle.RunSummary{}, nil
		},
	})
	if !errors.Is(err, sentinel) || runs != 0 || reads != 135 {
		t.Fatalf("entropy failure did not fail before launch: reads=%d runs=%d err=%v", reads, runs, err)
	}
}

const testEnvironmentDigest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestPairEntropyRejectsPartialErrorAndReuse(t *testing.T) {
	sentinel := errors.New("source error")
	for name, readFull := range map[string]exactRead{
		"short nil":   func([]byte) (int, error) { return 8, nil },
		"short error": func([]byte) (int, error) { return 8, io.ErrUnexpectedEOF },
		"full error":  func(destination []byte) (int, error) { return len(destination), sentinel },
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := preloadPairNonces(readFull); err == nil {
				t.Fatal("invalid exact entropy read was accepted")
			}
		})
	}
	if _, err := preloadPairNonces(func(destination []byte) (int, error) {
		for index := range destination {
			destination[index] = 0xA5
		}
		return len(destination), nil
	}); err == nil {
		t.Fatal("reused pair nonce was accepted")
	}
}
