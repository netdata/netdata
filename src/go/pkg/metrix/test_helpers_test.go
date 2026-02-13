// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import "testing"

func cycleController(t *testing.T, s CollectorStore) CycleController {
	t.Helper()
	managed, ok := AsCycleManagedStore(s)
	if !ok {
		t.Fatalf("store does not expose cycle control")
	}
	return managed.CycleController()
}

func expectPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatalf("expected panic")
		}
	}()
	fn()
}

func mustValue(t *testing.T, r Reader, name string, labels Labels, want SampleValue) {
	t.Helper()
	got, ok := r.Value(name, labels)
	if !ok {
		t.Fatalf("expected value for %s", name)
	}
	if got != want {
		t.Fatalf("unexpected value for %s: got=%v want=%v", name, got, want)
	}
}

func mustDelta(t *testing.T, r Reader, name string, labels Labels, want SampleValue) {
	t.Helper()
	got, ok := r.Delta(name, labels)
	if !ok {
		t.Fatalf("expected delta for %s", name)
	}
	if got != want {
		t.Fatalf("unexpected delta for %s: got=%v want=%v", name, got, want)
	}
}

func mustNoDelta(t *testing.T, r Reader, name string, labels Labels) {
	t.Helper()
	if _, ok := r.Delta(name, labels); ok {
		t.Fatalf("expected no delta for %s", name)
	}
}
