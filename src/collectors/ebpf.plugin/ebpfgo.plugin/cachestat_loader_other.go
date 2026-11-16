//go:build !netdata_ebpf_libbpf

package main

import "github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"

func LoadCachestatLegacy(cfg CachestatLegacyConfig) (*CachestatLegacyHandle, error) {
	if cfg.TargetResolutionErr != nil {
		return nil, cfg.TargetResolutionErr
	}
	return nil, libbpfloader.ErrDisabled
}

func LoadCachestatLegacyFromSystem() (*CachestatLegacyHandle, error) {
	return nil, libbpfloader.ErrDisabled
}
