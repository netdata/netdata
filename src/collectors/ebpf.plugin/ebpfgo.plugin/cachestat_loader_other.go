//go:build !netdata_ebpf_libbpf

package main

import "github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"

func LoadCachestatLegacy(cfg CachestatLegacyConfig) (*CachestatLegacyHandle, error) {
	_ = cfg
	return nil, libbpfloader.ErrDisabled
}

func LoadCachestatLegacyFromSystem() (*CachestatLegacyHandle, error) {
	return nil, libbpfloader.ErrDisabled
}
