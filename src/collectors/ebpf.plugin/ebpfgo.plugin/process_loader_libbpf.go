//go:build netdata_ebpf_libbpf

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

func processLoadFunction(cfg ProcessLegacyConfig, plan LoadPlan) (*libbpfloader.ProcessRuntime, error) {
	if plan.ObjectPath == "" {
		return nil, fmt.Errorf("invalid plan: object path not set")
	}

	useCore := plan.LoadMode == LoadCore

	rt, err := libbpfloader.ProcessRuntimeOpenMode(plan.ObjectPath, useCore)
	if err != nil {
		return nil, fmt.Errorf("failed to open process eBPF object: %w", err)
	}

	if err := rt.Prepare(cfg.PidTableSize, cfg.MapsPerCore); err != nil {
		rt.Close()
		return nil, fmt.Errorf("failed to prepare process runtime: %w", err)
	}

	if err := rt.Load(); err != nil {
		rt.Close()
		return nil, fmt.Errorf("failed to load process eBPF programs: %w", err)
	}

	if err := rt.Attach(); err != nil {
		rt.Close()
		return nil, fmt.Errorf("failed to attach process eBPF programs: %w", err)
	}

	if err := rt.UpdateController(cfg.AppsEnabled, cfg.AppsLevel); err != nil {
		rt.Close()
		return nil, fmt.Errorf("failed to update process controller: %w", err)
	}

	return rt, nil
}

func tryLoadProcessPlan(cfg ProcessLegacyConfig, plan LoadPlan) (*ProcessLegacyHandle, error) {
	rt, err := libbpfloader.ProcessRuntimeOpenMode(plan.ObjectPath, plan.LoadMode == LoadCore)
	if err != nil {
		return nil, err
	}
	closeOnError := func(err error) (*ProcessLegacyHandle, error) { rt.Close(); return nil, err }
	if err := rt.Prepare(cfg.PidTableSize, cfg.MapsPerCore); err != nil {
		return closeOnError(err)
	}
	if err := rt.Load(); err != nil {
		return closeOnError(err)
	}
	if err := rt.Attach(); err != nil {
		return closeOnError(err)
	}
	if err := rt.UpdateController(cfg.AppsEnabled || cfg.CgroupsEnabled, cfg.AppsLevel); err != nil {
		return closeOnError(err)
	}
	return &ProcessLegacyHandle{Plan: plan, Runtime: rt, UpdateEvery: cfg.UpdateEvery, ConfigFound: cfg.ConfigFound,
		PidTableSize: cfg.PidTableSize, MapsPerCore: cfg.MapsPerCore, AppsEnabled: cfg.AppsEnabled,
		CgroupsEnabled: cfg.CgroupsEnabled, AppsLevel: cfg.AppsLevel}, nil
}

func LoadProcessLegacy(cfg ProcessLegacyConfig) (*ProcessLegacyHandle, error) {
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
	plan := BuildProcessLegacyPlan(cfg)
	if !libbpfloader.ProcessRuntimeSupportsCore() {
		selector := SelectIndex(cfg.Kernels, cfg.IsRHF, cfg.KernelVersion)
		if int(selector) > processMaxBaseSelector {
			selector = processMaxBaseSelector
		}
		plan.ObjectPath = BuildObjectPathWithFlavor(cfg.PluginsDir, selector, "process", false, cfg.IsRHF, ObjectFlavorBase)
		plan.LoadMode = LoadLegacy
		return tryLoadProcessPlan(cfg, plan)
	}
	plans := buildFallbackPlans(plan, cfg.PluginsDir, cfg.IsRHF, "process", processMaxBaseSelector)
	var lastErr error
	for i, candidate := range plans {
		handle, loadErr := tryLoadProcessPlan(cfg, candidate)
		if loadErr == nil {
			return handle, nil
		}
		// BTF availability does not guarantee that every trampoline target
		// referenced by the object exists on the running kernel.  Retry the
		// same object through its tracepoint/kprobe programs before moving on
		// to another flavor or kernel selector.
		if candidate.LoadMode == LoadCore {
			legacy := candidate
			legacy.LoadMode = LoadLegacy
			if handle, legacyErr := tryLoadProcessPlan(cfg, legacy); legacyErr == nil {
				return handle, nil
			}
		}
		lastErr = loadErr
		if i < len(plans)-1 {
			fmt.Fprintf(os.Stderr, "ebpf-go.plugin: process %s unavailable (%v), trying fallback\n", filepath.Base(candidate.ObjectPath), loadErr)
		}
	}
	return nil, lastErr
}
