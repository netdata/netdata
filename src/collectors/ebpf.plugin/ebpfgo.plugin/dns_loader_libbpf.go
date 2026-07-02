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
func tryLoadDNSPlan(plan LoadPlan) (*DNSLegacyHandle, error) {
	rt, err := libbpfloader.NewDNSRuntime(plan.ObjectPath, plan.LoadMode == LoadCore)
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
	plan := BuildDNSLegacyPlan(cfg)

	plans := buildFallbackPlans(plan, cfg.PluginsDir, cfg.IsRHF, "dns", dnsMaxBaseSelector)
	var lastErr error
	for i, fp := range plans {
		handle, err := tryLoadDNSPlan(fp)
		if err == nil {
			handle.UpdateEvery = cfg.UpdateEvery
			handle.ConfigFound = cfg.ConfigFound
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
