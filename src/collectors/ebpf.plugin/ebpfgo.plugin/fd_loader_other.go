//go:build !netdata_ebpf_libbpf

package main

import "github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"

func LoadFDLegacy(cfg FDLegacyConfig) (*FDLegacyHandle, error) {
	_ = cfg
	return nil, libbpfloader.ErrDisabled
}

func LoadFDLegacyFromSystem() (*FDLegacyHandle, error) {
	return nil, libbpfloader.ErrDisabled
}
