package main

/*
#include <stdlib.h>

static int netdata_ebpf_scaffold_ready(void)
{
	return 0;
}
*/
import "C"

// Keep the cgo boundary in place without loading any Linux binary yet.
func cgoScaffoldReady() int {
	return int(C.netdata_ebpf_scaffold_ready())
}
