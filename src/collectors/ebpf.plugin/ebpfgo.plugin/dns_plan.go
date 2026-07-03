package main

import (
	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

const (
	dnsKernelMask          uint32 = (1 << 12) - 1
	dnsDefaultUpdateEvery         = 10
	dnsDefaultObjectFlavor        = "buffer"
	// dnsMaxBaseSelector is the highest SelectKernelName index for which a
	// base-flavor (no suffix) dns object file is shipped.
	dnsMaxBaseSelector = 7 // 5.14
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
	// PerQueryTracking controls whether the dedicated AF_PACKET capture
	// socket used for per-query DNS payload parsing is opened during attach.
	// When false, the DNS runtime still emits aggregate counters via the ring
	// buffer but per-query flow records are empty. Defaults to true to keep
	// behavior unchanged for operators who rely on the dns-queries function.
	PerQueryTracking bool
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
		PluginsDir:       defaultPluginsDir(),
		Kernels:          dnsKernelMask,
		IsRHF:            -1,
		IsDebian:         IsDebianFlavor(),
		UpdateEvery:      dnsDefaultUpdateEvery,
		ObjectFlavor:     dnsDefaultObjectFlavor,
		Enabled:          false, // stock ebpf.d.conf: dns = no
		PerQueryTracking: true,  // preserve current behavior; opt-out path is plumbing-ready
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

	kver, isRHF, err := resolveKernelAndRH()
	if err != nil {
		return DNSLegacyConfig{}, err
	}
	cfg.KernelVersion = kver
	cfg.IsRHF = isRHF

	return cfg, nil
}

func BuildDNSLegacyPlan(cfg DNSLegacyConfig) LoadPlan {
	flavor := selectConfiguredObjectFlavor(cfg.ObjectFlavor, cfg.KernelVersion, cfg.IsDebian)
	selector := SelectIndex(cfg.Kernels, cfg.IsRHF, cfg.KernelVersion)
	// Base-flavor DNS objects are not built beyond 5.14; cap the selector so we
	// never construct a path that does not exist.
	if flavor == ObjectFlavorBase && int(selector) > dnsMaxBaseSelector {
		selector = uint32(dnsMaxBaseSelector)
	}
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
