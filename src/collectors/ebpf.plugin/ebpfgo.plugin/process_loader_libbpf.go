//go:build netdata_ebpf_libbpf

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

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
		CgroupsEnabled: cfg.CgroupsEnabled, AppsLevel: cfg.AppsLevel, ReportErrors: cfg.ReportErrors}, nil
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
	if err := CanLoadCode(cfg.KernelVersion, cfg.IsRHF, rejected, IsRoot(), true, "ebpf-go.plugin"); err != nil {
		return nil, err
	}
	plan := BuildProcessLegacyPlan(cfg)
	if !libbpfloader.ProcessRuntimeSupportsCore() {
		selector := selectHighestIndexAtOrBelow(cfg.Kernels, cfg.IsRHF, cfg.KernelVersion,
			effectiveMaxBaseSelectorForModule(processMaxBaseSelector, "process", cfg.IsRHF, cfg.KernelVersion), "process")
		plan.ObjectPath = BuildObjectPathWithFlavor(cfg.PluginsDir, selector, "process", true, cfg.IsRHF, ObjectFlavorBase)
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
			handle, legacyErr := tryLoadProcessPlan(cfg, legacy)
			if legacyErr == nil {
				return handle, nil
			}
			// Keep both causes: the CO-RE error alone hides why the
			// tracepoint/kprobe retry of the same object failed.
			loadErr = errors.Join(loadErr, fmt.Errorf("legacy retry: %w", legacyErr))
		}
		lastErr = loadErr
		if i < len(plans)-1 {
			fmt.Fprintf(os.Stderr, "ebpf-go.plugin: process %s unavailable (%v), trying fallback\n", filepath.Base(candidate.ObjectPath), loadErr)
		}
	}
	return nil, lastErr
}
