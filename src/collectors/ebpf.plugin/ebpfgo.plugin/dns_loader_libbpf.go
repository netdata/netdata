//go:build netdata_ebpf_libbpf

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

// tryLoadDNSPlan opens, prepares, loads, and attaches a single DNS plan.
// On any failure the partially-initialised runtime is closed before returning.
func tryLoadDNSPlan(plan LoadPlan, perQuery bool) (*DNSLegacyHandle, error) {
	rt, err := libbpfloader.NewDNSRuntime(plan.ObjectPath, plan.LoadMode == LoadCore, perQuery)
	if err != nil {
		return nil, err
	}

	if err := rt.Prepare(); err != nil {
		rt.Close()
		return nil, err
	}

	if err := rt.Load(); err != nil {
		rt.Close()
		return nil, err
	}

	if err := rt.Attach(); err != nil {
		rt.Close()
		return nil, err
	}

	return &DNSLegacyHandle{
		Plan:    plan,
		Runtime: rt,
	}, nil
}

func LoadDNSLegacy(cfg DNSLegacyConfig) (*DNSLegacyHandle, error) {
	versionStr, err := KernelVersionString()
	if err != nil {
		return nil, fmt.Errorf("kernel version string: %w", err)
	}
	rejected, err := KernelRejected(versionStr, filepath.Join(cfg.PluginsDir, "ebpf_kernel_reject_list.txt"))
	if err != nil {
		return nil, fmt.Errorf("kernel reject check: %w", err)
	}
	if err := CanLoadCode(cfg.KernelVersion, cfg.IsRHF, rejected, IsRoot(), "ebpf-go.plugin"); err != nil {
		return nil, err
	}

	plan := BuildDNSLegacyPlan(cfg)

	plans := buildFallbackPlans(plan, cfg.PluginsDir, cfg.IsRHF, "dns", dnsMaxBaseSelector)
	var lastErr error
	for i, fp := range plans {
		handle, err := tryLoadDNSPlan(fp, cfg.PerQueryTracking)
		if err == nil {
			handle.UpdateEvery = cfg.UpdateEvery
			handle.ConfigFound = cfg.ConfigFound
			handle.Runtime.SetFlowTTL(cfg.FlowTTL)
			return handle, nil
		}
		lastErr = err
		if i < len(plans)-1 {
			fmt.Fprintf(os.Stderr,
				"ebpf-go.plugin: dns %s unavailable (%v), trying fallback\n",
				filepath.Base(fp.ObjectPath), err)
		}
	}
	return nil, lastErr
}

func LoadDNSLegacyFromSystem() (*DNSLegacyHandle, error) {
	cfg, err := resolveDNSLegacyConfig()
	if err != nil {
		return nil, err
	}

	return LoadDNSLegacy(cfg)
}
