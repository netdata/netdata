//go:build netdata_ebpf_libbpf

package libbpfloader

import "testing"

// TestAccumulatorSelfTest drives the C self-test for the shared per-TGID
// accumulator used by the buffer and arena flavors.
//
// The accumulator is pure C reached only through CGo, so it is invisible to the
// default (CGO_ENABLED=0) test run.  It shipped once with a missing
// nd_ebpf_acc_init(), which aborted the plugin with "malloc(): corrupted top
// size" on the first per-PID event; this test guards that class of regression.
func TestAccumulatorSelfTest(t *testing.T) {
	if rc := AccumulatorSelfTest(); rc != 0 {
		t.Fatalf("AccumulatorSelfTest() = %d, want 0 (see nd_ebpf_acc_selftest.c for the code)", rc)
	}
}
