//go:build !netdata_ebpf_libbpf

package main

import "github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"

func LoadDCStatLegacy(cfg DCStatLegacyConfig) (*DCStatLegacyHandle, error) {
	_ = cfg
	return nil, libbpfloader.ErrDisabled
}

func LoadDCStatLegacyFromSystem() (*DCStatLegacyHandle, error) {
	return nil, libbpfloader.ErrDisabled
}
