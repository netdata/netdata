package monotime

import (
	"testing"
	"time"
)

func TestNowDoesNotGoBackwards(t *testing.T) {
	prev := Now()
	for range 1000 {
		now := Now()
		if now < prev {
			t.Fatalf("Now went backwards: %d after %d", now, prev)
		}
		prev = now
	}
}

func TestAfterIsAheadOfNow(t *testing.T) {
	d := time.Millisecond
	// Bracket the call with clock reads, so a descheduled test cannot fail.
	before := Now()
	deadline := After(d)
	after := Now()
	if deadline == 0 {
		t.Fatal("After returned the no-deadline value")
	}
	if deadline < before+int64(d) || deadline > after+int64(d) {
		t.Fatalf("After(%v) = %d, want within [%d, %d]", d, deadline, before+int64(d), after+int64(d))
	}
	if expired := After(-d); expired >= Now() {
		t.Fatalf("After(-d) = %d is not in the past", expired)
	}
}
