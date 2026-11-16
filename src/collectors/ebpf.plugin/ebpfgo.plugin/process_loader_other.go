//go:build !netdata_ebpf_libbpf

package main

import "fmt"

func LoadProcessLegacy(cfg ProcessLegacyConfig) (*ProcessLegacyHandle, error) {
	_ = cfg
	return nil, fmt.Errorf("process collector requires libbpf build (netdata_ebpf_libbpf tag)")
}
