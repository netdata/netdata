package main

import (
	"fmt"
	"os"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

const (
	dnsKernelMask uint32 = (1 << 12) - 1
	// dnsDefaultUpdateEvery is intentionally half of dnsDefaultFlowTTL so the SHM
	// stays fresh mid-window; NV_DNS_UPDATE_EVERY must equal the TTL to keep
	// consecutive NV snapshots non-overlapping (each query counted once).
	dnsDefaultUpdateEvery = 10
	dnsNVUpdateEvery      = 20
	// dnsDefaultFlowTTL is the default DNS flow record lifetime in seconds.
	// Must match DNS_FLOW_TTL_US in dns_libbpf.c.  update every must not exceed
	// this value or records expire in the kernel ring before Go collects them.
	dnsDefaultFlowTTL      = 20
	dnsDefaultObjectFlavor = "buffer"
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
	// PerQueryTracking controls whether the dedicated AF_PACKET flow-capture
	// socket is opened during attach. When false, per-query flow records are
	// empty and the dns-queries network-viewer function returns no rows.
	// Configurable via "per query tracking" in ebpf.d/dns.conf [global].
	PerQueryTracking bool
	// FlowTTL is the DNS flow record lifetime in seconds; must match
	// DNS_FLOW_TTL_US in dns_libbpf.c.  UpdateEvery is clamped to this value.
	// Configurable via "flow ttl" in ebpf.d/dns.conf [global].
	FlowTTL int
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
		PerQueryTracking: true,
		FlowTTL:          dnsDefaultFlowTTL,
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
	if fileCfg.PerQueryTracking != nil {
		cfg.PerQueryTracking = *fileCfg.PerQueryTracking
	}
	if fileCfg.FlowTTL != nil && *fileCfg.FlowTTL > 0 {
		cfg.FlowTTL = *fileCfg.FlowTTL
	}

	// NV_DNS_UPDATE_EVERY is hardcoded to 20 in network-viewer.c.  If FlowTTL
	// differs, dns-queries results will be double-counted (TTL > 20) or silently
	// dropped (TTL < 20).  Warn early so operators catch the misconfiguration.
	if cfg.FlowTTL != dnsNVUpdateEvery {
		fmt.Fprintf(os.Stderr,
			"ebpf-go.plugin: dns: flow ttl %d != NV_DNS_UPDATE_EVERY (%d); "+
				"dns-queries results may be double-counted or missing\n",
			cfg.FlowTTL, dnsNVUpdateEvery)
	}

	// Clamp update_every to the flow TTL: records older than FlowTTL seconds are
	// dropped by FlowSnapshot.  The TTL is propagated to the C runtime via
	// SetFlowTTL after loading so both layers enforce the same live window.
	if cfg.UpdateEvery > cfg.FlowTTL {
		fmt.Fprintf(os.Stderr,
			"ebpf-go.plugin: dns: update every %d exceeds flow ttl %d; clamping to %d\n",
			cfg.UpdateEvery, cfg.FlowTTL, cfg.FlowTTL)
		cfg.UpdateEvery = cfg.FlowTTL
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
