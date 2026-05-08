package main

import "testing"

func TestCgoScaffoldReady(t *testing.T) {
	if got := cgoScaffoldReady(); got != 0 {
		t.Fatalf("unexpected cgo scaffold state: %d", got)
	}
}
