// SPDX-License-Identifier: GPL-3.0-or-later
//go:build linux && cgo && netdata_ebpf_libbpf

package cachestat

import (
	"context"
	"fmt"
	"os"

	"github.com/netdata/netdata/go/plugins/plugin/ebpf/internal/libbpfloader"
)

func probeNative(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !libbpfloader.SupportsCore() {
		return "", fmt.Errorf("POC requires the CO-RE skeleton build; see plugin/ebpf/README.md")
	}
	f, err := os.Open("/sys/kernel/btf/vmlinux")
	if err != nil {
		return "", fmt.Errorf("kernel BTF: %w", err)
	}
	_ = f.Close()
	symbols, err := os.Open("/proc/kallsyms")
	if err != nil {
		return "", err
	}
	defer symbols.Close()
	return resolveTargets(ctx, symbols)
}

func openNative(ctx context.Context, target string) (nativeRuntime, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// The base CO-RE object is embedded in the reused backend's skeleton.
	rt, err := libbpfloader.NewCachestatRuntime("cachestat", true)
	if err != nil {
		return nil, err
	}
	steps := []struct {
		name string
		run  func() error
	}{
		{"prepare", func() error { return rt.Prepare(1, true, target) }},
		{"load", rt.Load},
		{"controller", func() error { return rt.UpdateController(false, 0) }},
		{"attach", func() error { return rt.Attach(target) }},
	}
	for _, step := range steps {
		if err := ctx.Err(); err != nil {
			rt.Close()
			return nil, err
		}
		if err := step.run(); err != nil {
			rt.Close()
			return nil, fmt.Errorf("%s: %w", step.name, err)
		}
	}
	if err := ctx.Err(); err != nil {
		rt.Close()
		return nil, err
	}
	return &libbpfRuntime{rt: rt}, nil
}

type libbpfRuntime struct {
	rt *libbpfloader.CachestatRuntime
}

func (r *libbpfRuntime) Snapshot() (snapshot, error) {
	s, err := r.rt.Snapshot(true)
	return snapshot{Accessed: s.MarkPageAccessed, BufferDirty: s.MarkBufferDirty,
		Added: s.AddToPageCacheLru, AccountDirtied: s.AccountPageDirtied}, err
}

func (r *libbpfRuntime) Close() { r.rt.Close() }
