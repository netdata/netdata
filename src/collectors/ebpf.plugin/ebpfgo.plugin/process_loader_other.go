//go:build !netdata_ebpf_libbpf

package main

import "github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"

func LoadProcessLegacy(cfg ProcessLegacyConfig) (*ProcessLegacyHandle, error) {
	_ = cfg
	return nil, libbpfloader.ErrDisabled
}
