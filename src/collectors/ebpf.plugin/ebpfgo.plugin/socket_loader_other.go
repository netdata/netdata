//go:build !netdata_ebpf_libbpf

package main

import "github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"

func LoadSocketLegacy(cfg SocketLegacyConfig) (*SocketLegacyHandle, error) {
	_ = cfg
	return nil, libbpfloader.ErrDisabled
}

func LoadSocketLegacyFromSystem() (*SocketLegacyHandle, error) {
	return nil, libbpfloader.ErrDisabled
}
