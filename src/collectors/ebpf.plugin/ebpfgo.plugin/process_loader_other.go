//go:build !netdata_ebpf_libbpf

package main

import (
	"fmt"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

func LoadProcessLegacy(cfg ProcessLegacyConfig) (*ProcessLegacyHandle, error) {
	_ = cfg
	return nil, fmt.Errorf("process collector requires libbpf build (netdata_ebpf_libbpf tag)")
}

func processLoadFunction(cfg ProcessLegacyConfig, plan LoadPlan) (*libbpfloader.ProcessRuntime, error) {
	return nil, fmt.Errorf("process collector requires libbpf build (netdata_ebpf_libbpf tag)")
}
