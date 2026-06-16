package main

import (
	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

const (
	dnsKernelMask          uint32 = (1 << 12) - 1
	dnsDefaultUpdateEvery         = 10
	dnsDefaultObjectFlavor        = "buffer"
)

type DNSLegacyConfig struct {
	PluginsDir    string
	Kernels       uint32
	IsRHF         int
	KernelVersion uint32
	IsDebian      bool
	ConfigFound   bool
	Enabled       bool
	UpdateEvery   int
	ObjectFlavor  string
}

type DNSLegacyHandle struct {
	Plan        LoadPlan
	Runtime     *libbpfloader.DNSRuntime
	UpdateEvery int
	ConfigFound bool
}

func (h *DNSLegacyHandle) Close() {
	if h == nil || h.Runtime == nil {
		return
	}
	h.Runtime.Close()
	h.Runtime = nil
}

func defaultDNSLegacyConfig() DNSLegacyConfig {
	return DNSLegacyConfig{
		PluginsDir:   defaultPluginsDir(),
		Kernels:      dnsKernelMask,
		IsRHF:        -1,
		IsDebian:     IsDebianFlavor(),
		UpdateEvery:  dnsDefaultUpdateEvery,
		ObjectFlavor: dnsDefaultObjectFlavor,
		Enabled:      false, // stock ebpf.d.conf: dns = no
	}
}

func resolveDNSLegacyConfig() (DNSLegacyConfig, error) {
	cfg := defaultDNSLegacyConfig()

	fileCfg, found, err := loadDNSConfigFiles()
	if err != nil {
		return DNSLegacyConfig{}, err
	}
	cfg.ConfigFound = found
	if fileCfg.DNS != nil {
		cfg.Enabled = *fileCfg.DNS
	}
	if fileCfg.UpdateEvery != nil && *fileCfg.UpdateEvery > 0 {
		cfg.UpdateEvery = *fileCfg.UpdateEvery
	}
	if fileCfg.ObjectFlavor != nil && *fileCfg.ObjectFlavor != "" {
		cfg.ObjectFlavor = *fileCfg.ObjectFlavor
	}

	kver, err := KernelVersion()
	if err != nil {
		return DNSLegacyConfig{}, err
	}
	cfg.KernelVersion = kver

	if rhf, err := RedHatRelease(); err == nil {
		cfg.IsRHF = rhf
	}

	return cfg, nil
}

func BuildDNSLegacyPlan(cfg DNSLegacyConfig) LoadPlan {
	flavor := selectConfiguredObjectFlavor(cfg.ObjectFlavor, cfg.KernelVersion, cfg.IsDebian)
	selector := SelectIndex(cfg.Kernels, cfg.IsRHF, cfg.KernelVersion)
	return LoadPlan{
		KernelVersion: cfg.KernelVersion,
		IsRHF:         cfg.IsRHF,
		Selector:      selector,
		Flavor:        flavor,
		ObjectPath:    BuildObjectPathWithFlavor(cfg.PluginsDir, selector, "dns", false, cfg.IsRHF, flavor),
		// DNS socket filter programs do not use CO-RE relocations, so LoadCore
		// (plain bpf_object__load) works universally.
		LoadMode:    LoadCore,
		ProgramMode: LoadProbe,
	}
}
