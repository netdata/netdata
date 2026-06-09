package main

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
)

func main() {
	// Cap the Go scheduler to 2 OS threads.  This plugin has exactly two
	// active goroutines (global metric collector and SHM publisher) plus a
	// blocked signal-handler goroutine.  The default GOMAXPROCS = NumCPU
	// allocates O(ncpus) scheduler threads, and CGO calls on blocked
	// goroutines cause the runtime to create up to O(ncpus) additional
	// threads — each carrying an 8 MB Linux stack.  On a 64-core host that
	// is ~130 threads and ~1 GB of stack RSS for no benefit.
	runtime.GOMAXPROCS(2)
	updateEvery := 0
	if len(os.Args) > 1 {
		if parsed, err := strconv.Atoi(os.Args[1]); err == nil && parsed > 0 {
			updateEvery = parsed
		}
	}

	cfg, err := resolveCachestatLegacyConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ebpf-go.plugin: cachestat config load failed: %v\n", err)
		os.Exit(1)
	}
	if !cfg.Enabled {
		fmt.Fprintf(os.Stderr, "ebpf-go.plugin: cachestat collection disabled by configuration\n")
		os.Exit(0)
	}

	handle, err := LoadCachestatLegacy(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ebpf-go.plugin: cachestat load failed: %v\n", err)
		os.Exit(1)
	}
	if handle == nil || handle.Runtime == nil {
		fmt.Fprintf(os.Stderr, "ebpf-go.plugin: no eBPF program loaded\n")
		os.Exit(1)
	}

	if updateEvery <= 0 {
		updateEvery = handle.UpdateEvery
	}
	if updateEvery <= 0 {
		updateEvery = cachestatDefaultUpdateEvery
	}
	handle.UpdateEvery = updateEvery

	runCachestatPlugin(handle, updateEvery)
}
