//go:build netdata_ebpf_libbpf

package libbpfloader

// Thin Go wrappers around the C test helpers in dns_libbpf.c.
//
// CGo is not allowed in _test.go files of packages that are imported during
// go build (Go restriction: "use of cgo in test … not supported").  This file
// is a normal (non-test) source file that holds the CGo preamble and exports
// unexported Go functions; dns_parser_libbpf_test.go calls these instead of
// calling C directly.

/*
#include <stdlib.h>
#include <stdint.h>

extern void *netdata_dns_alloc_test_runtime(void);
extern void  netdata_dns_free_test_runtime(void *p);
extern int   netdata_dns_test_parse_raw_packet(void *p, const char *buf, int n);
extern int   netdata_dns_test_read_name(const char *msg, int msg_len, int offset,
                                        char *out, int out_size);
*/
import "C"

import (
	"unsafe"
)

func dnsAllocTestRuntime() unsafe.Pointer {
	return unsafe.Pointer(C.netdata_dns_alloc_test_runtime())
}

func dnsFreeTestRuntime(p unsafe.Pointer) {
	C.netdata_dns_free_test_runtime(p)
}

func dnsTestParseRawPacket(p unsafe.Pointer, buf []byte) bool {
	var ptr *C.char
	if len(buf) > 0 {
		ptr = (*C.char)(unsafe.Pointer(&buf[0]))
	}
	return C.netdata_dns_test_parse_raw_packet(p, ptr, C.int(len(buf))) != 0
}

// dnsTestReadName calls dns_read_name via the C test wrapper.
// out_size controls the maximum output buffer size — pass a small value to
// exercise the overflow guard.  Returns the number of bytes consumed (0 on
// error), matching the C function's contract.
func dnsTestReadName(msg []byte, offset, outSize int) int {
	if outSize <= 0 {
		return 0
	}
	out := make([]byte, outSize)
	var msgPtr *C.char
	if len(msg) > 0 {
		msgPtr = (*C.char)(unsafe.Pointer(&msg[0]))
	}
	return int(C.netdata_dns_test_read_name(
		msgPtr, C.int(len(msg)), C.int(offset),
		(*C.char)(unsafe.Pointer(&out[0])), C.int(outSize),
	))
}
