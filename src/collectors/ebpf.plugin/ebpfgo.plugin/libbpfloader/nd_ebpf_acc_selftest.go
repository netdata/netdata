//go:build netdata_ebpf_libbpf

package libbpfloader

/*
int netdata_ebpf_acc_selftest(void);
*/
import "C"

// AccumulatorSelfTest exercises the shared per-TGID accumulator in C and returns
// 0 on success, or the failing assertion's code (documented in
// nd_ebpf_acc_selftest.c).
//
// It lives in a non-test file because cgo is not permitted in _test.go files:
// `go test` rejects them with "use of cgo in test <file> not supported".
// The accumulator is only reachable through CGo, so this is the only way to
// cover it from `go test`.
func AccumulatorSelfTest() int {
	return int(C.netdata_ebpf_acc_selftest())
}
