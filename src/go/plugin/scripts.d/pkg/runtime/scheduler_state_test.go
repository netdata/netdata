package runtime

import "testing"

func TestJobStateRecordResultKeepsSoftState(t *testing.T) {
	js := &jobState{maxAttempts: 3, hardState: "OK"}

	js.recordResult("warning")

	if js.state != "WARNING" {
		t.Fatalf("expected state=WARNING, got %s", js.state)
	}
	if js.hardState != "OK" {
		t.Fatalf("hard state should remain OK, got %s", js.hardState)
	}
	if js.softAttempts != 1 {
		t.Fatalf("expected softAttempts=1, got %d", js.softAttempts)
	}
	if !js.retrying {
		t.Fatalf("expected retrying=true on soft state")
	}
}

func TestJobStateRecordResultHardensAfterMaxAttempts(t *testing.T) {
	js := &jobState{maxAttempts: 2, hardState: "OK"}

	js.recordResult("critical")
	js.recordResult("critical")

	if js.hardState != "CRITICAL" {
		t.Fatalf("expected hard state CRITICAL, got %s", js.hardState)
	}
	if js.state != "CRITICAL" {
		t.Fatalf("expected state=CRITICAL, got %s", js.state)
	}
	if js.retrying {
		t.Fatalf("expected retrying=false after hard failure")
	}
}
