//go:build !netdata_ebpf_libbpf

package main

import "github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"

func LoadDNSLegacy(cfg DNSLegacyConfig) (*DNSLegacyHandle, error) {
	_ = cfg
	return nil, libbpfloader.ErrDisabled
}

func LoadDNSLegacyFromSystem() (*DNSLegacyHandle, error) {
	return nil, libbpfloader.ErrDisabled
}
