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
	deadline := After(d)
	if deadline == 0 {
		t.Fatal("After returned the no-deadline value")
	}
	if left := time.Duration(deadline - Now()); left <= 0 || left > d {
		t.Fatalf("time left = %v, want (0, %v]", left, d)
	}
	if expired := After(-d); expired >= Now() {
		t.Fatalf("After(-d) = %d is not in the past", expired)
	}
}
