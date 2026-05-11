package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

const cachestatKernelMask uint32 = (1 << 12) - 1
const cachestatDefaultPIDTableSize uint32 = 32768
const cachestatDefaultBTFPath = "/sys/kernel/btf"
const cachestatDefaultBTFFile = "vmlinux"

type CachestatLegacyConfig struct {
	PluginsDir      string
	Kernels         uint32
	IsRHF           int
	KernelVersion   uint32
	IsDebian        bool
	HasBTF          bool
	PidTableSize    uint32
	MapsPerCore     bool
	AccountFunction string
}

type CachestatLegacyHandle struct {
	Plan    LoadPlan
	Runtime *libbpfloader.CachestatRuntime
}

func (h *CachestatLegacyHandle) Close() {
	if h == nil || h.Runtime == nil {
		return
	}

	h.Runtime.Close()
	h.Runtime = nil
}

func defaultPluginsDir() string {
	if dir := os.Getenv("NETDATA_PLUGINS_DIR"); dir != "" {
		return dir
	}

	return filepath.Join(netdataRuntimePrefix, "usr/libexec/netdata/plugins.d")
}

func defaultCachestatLegacyConfig() CachestatLegacyConfig {
	return CachestatLegacyConfig{
		PluginsDir:      defaultPluginsDir(),
		Kernels:         cachestatKernelMask,
		IsRHF:           -1,
		IsDebian:        IsDebianFlavor(),
		HasBTF:          kernelBTFSupported(),
		PidTableSize:    cachestatDefaultPIDTableSize,
		MapsPerCore:     true,
		AccountFunction: selectCachestatDirtyAccountFunction(),
	}
}

func resolveCachestatLegacyConfig() (CachestatLegacyConfig, error) {
	cfg := defaultCachestatLegacyConfig()

	kver, err := KernelVersion()
	if err != nil {
		return CachestatLegacyConfig{}, err
	}
	cfg.KernelVersion = kver

	if rhf, err := RedHatRelease(); err == nil {
		cfg.IsRHF = rhf
	}

	if cfg.AccountFunction == "" {
		cfg.AccountFunction = selectCachestatDirtyAccountFunction()
	}

	return cfg, nil
}

func BuildCachestatLegacyPlan(cfg CachestatLegacyConfig) LoadPlan {
	return BuildLoadPlan(
		cfg.PluginsDir,
		cfg.Kernels,
		cfg.IsRHF,
		cfg.KernelVersion,
		"cachestat",
		false,
		true,
		cfg.IsDebian,
		cfg.HasBTF,
		LoadCore,
		"",
		RunModeEntry,
	)
}

func kernelBTFSupported() bool {
	_, err := os.Stat(filepath.Join(cachestatDefaultBTFPath, cachestatDefaultBTFFile))
	return err == nil
}

func selectCachestatDirtyAccountFunction() string {
	candidates := []string{
		"__folio_mark_dirty",
		"__set_page_dirty",
		"account_page_dirtied",
	}

	file, err := os.Open("/proc/kallsyms")
	if err != nil {
		return candidates[len(candidates)-1]
	}
	defer file.Close()

	seen := make(map[string]struct{}, len(candidates))
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		for _, candidate := range candidates {
			if strings.Contains(line, candidate) {
				seen[candidate] = struct{}{}
			}
		}
	}

	for _, candidate := range candidates {
		if _, ok := seen[candidate]; ok {
			return candidate
		}
	}

	return candidates[len(candidates)-1]
}
