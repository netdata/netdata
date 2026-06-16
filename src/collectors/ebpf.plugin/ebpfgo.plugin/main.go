package main

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"sync"
	"syscall"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
)

func main() {
	// Cap the Go scheduler to 5 OS threads: one per active collector goroutine
	// (cachestat, socket, dns), one for the signal handler, and one for the
	// stdin dispatcher goroutine that blocks on os.Stdin reads.  The default
	// GOMAXPROCS = NumCPU allocates O(ncpus) scheduler threads, and CGO calls
	// on blocked goroutines cause the runtime to create up to O(ncpus)
	// additional threads — each carrying an 8 MB Linux stack.  On a 64-core
	// host that is ~130 threads and ~1 GB of stack RSS for no benefit.
	runtime.GOMAXPROCS(5)

	updateEvery := 0
	if len(os.Args) > 1 {
		if parsed, err := strconv.Atoi(os.Args[1]); err == nil && parsed > 0 {
			updateEvery = parsed
		}
	}

	cachestatCfg, err := resolveCachestatLegacyConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ebpf-go.plugin: cachestat config load failed: %v\n", err)
		os.Exit(1)
	}

	socketCfg, err := resolveSocketLegacyConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ebpf-go.plugin: socket config load failed: %v\n", err)
		os.Exit(1)
	}

	dnsCfg, err := resolveDNSLegacyConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ebpf-go.plugin: dns config load failed: %v\n", err)
		os.Exit(1)
	}

	if !anyProgramEnabled(cachestatCfg, socketCfg, dnsCfg) {
		fmt.Fprintf(os.Stderr, "ebpf-go.plugin: all eBPF programs disabled by configuration\n")
		os.Exit(0)
	}

	// Shared stop channel: closed on SIGINT/SIGTERM or stdin QUIT.
	// closeStop uses sync.Once so both the signal handler and the stdin
	// dispatcher can call it safely without a double-close panic.
	stop := make(chan struct{})
	var closeStopOnce sync.Once
	closeStop := func() { closeStopOnce.Do(func() { close(stop) }) }

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		signal.Stop(sigCh)
		closeStop()
	}()

	api := netdataapi.New(os.Stdout)
	var wg sync.WaitGroup
	anyStarted := false

	// The shared store must exist before both collectors start so socket data
	// can be merged into SHM entries that cachestat apps/cgroups populate.
	var store *cachestatSharedMemoryStore
	needsStore := socketCfg.Enabled ||
		(cachestatCfg.Enabled && (cachestatCfg.AppsEnabled || cachestatCfg.CgroupsEnabled))
	if needsStore {
		store = NewCachestatSharedMemoryStore()
	}

	// The stdin dispatcher is always started so it can handle function calls
	// from whichever subset of collectors is enabled.
	var fnStore *socketFunctionStore
	var dnsFnStore *dnsFunctionStore

	// ---- cachestat ----
	if cachestatCfg.Enabled {
		ue := resolveUpdateEvery(updateEvery, cachestatCfg.UpdateEvery, cachestatDefaultUpdateEvery)
		cachestatCfg.UpdateEvery = ue

		handle, herr := LoadCachestatLegacy(cachestatCfg)
		if herr != nil {
			fmt.Fprintf(os.Stderr, "ebpf-go.plugin: cachestat load failed: %v\n", herr)
		} else if handle != nil && handle.Runtime != nil {
			// Only propagate store to cachestat when it has apps/cgroups consumers;
			// that is what triggers per-PID collection and SHM publishing.
			var cachestatStore *cachestatSharedMemoryStore
			if handle.AppsEnabled || handle.CgroupsEnabled {
				cachestatStore = store
			}
			anyStarted = true
			wg.Add(1)
			go func() {
				defer wg.Done()
				runCachestatGlobalCollector(api, handle, stop, cachestatStore, ue)
				handle.Close()
			}()
		}
	}

	// ---- socket ----
	if socketCfg.Enabled {
		ue := resolveUpdateEvery(updateEvery, socketCfg.UpdateEvery, socketDefaultUpdateEvery)
		socketCfg.UpdateEvery = ue

		handle, herr := LoadSocketLegacy(socketCfg)
		if herr != nil {
			fmt.Fprintf(os.Stderr, "ebpf-go.plugin: socket load failed: %v\n", herr)
		} else if handle != nil && handle.Runtime != nil {
			fnStore = newSocketFunctionStore(ue)

			pluginOutputMu.Lock()
			api.FUNCTIONGLOBAL(netdataapi.FunctionGlobalOpts{
				Name:     socketFunctionName,
				Timeout:  socketFunctionTimeout,
				Help:     socketFunctionHelp,
				Tags:     socketFunctionTags,
				Access:   socketFunctionAccess,
				Priority: socketFunctionPriority,
				Version:  socketFunctionVersion,
			})
			pluginOutputMu.Unlock()

			anyStarted = true

			wg.Add(1)
			go func() {
				defer wg.Done()
				runSocketGlobalCollector(handle, stop, ue, store, fnStore)
				handle.Close()
			}()
		}
	}

	// ---- dns ----
	if dnsCfg.Enabled {
		ue := resolveUpdateEvery(updateEvery, dnsCfg.UpdateEvery, dnsDefaultUpdateEvery)
		dnsCfg.UpdateEvery = ue

		handle, herr := LoadDNSLegacy(dnsCfg)
		if herr != nil {
			fmt.Fprintf(os.Stderr, "ebpf-go.plugin: dns load failed: %v\n", herr)
		} else if handle != nil && handle.Runtime != nil {
			dnsFnStore = newDNSFunctionStore(ue)

			pluginOutputMu.Lock()
			api.FUNCTIONGLOBAL(netdataapi.FunctionGlobalOpts{
				Name:     dnsFunctionName,
				Timeout:  dnsFunctionTimeout,
				Help:     dnsFunctionHelp,
				Tags:     dnsFunctionTags,
				Access:   dnsFunctionAccess,
				Priority: dnsFunctionPriority,
				Version:  dnsFunctionVersion,
			})
			pluginOutputMu.Unlock()

			anyStarted = true

			wg.Add(1)
			go func() {
				defer wg.Done()
				runDNSGlobalCollector(handle, stop, ue, dnsFnStore)
				handle.Close()
			}()
		}
	}

	// Start stdin dispatcher after all function stores are initialised so the
	// dispatcher sees consistent fnStore / dnsFnStore pointers.
	go runStdinDispatcher(api, fnStore, dnsFnStore, closeStop)

	if !anyStarted {
		fmt.Fprintf(os.Stderr, "ebpf-go.plugin: all enabled programs failed to load\n")
		os.Exit(1)
	}

	wg.Wait()
}

// resolveUpdateEvery returns the first positive value from: CLI arg, config, default.
func resolveUpdateEvery(cliArg, cfgVal, fallback int) int {
	if cliArg > 0 {
		return cliArg
	}
	if cfgVal > 0 {
		return cfgVal
	}
	return fallback
}

// anyProgramEnabled returns true when at least one eBPF program is enabled.
// The plugin exits early only when every known program is disabled so that
// adding a new program requires only a new field here, not a structural change.
func anyProgramEnabled(cachestatCfg CachestatLegacyConfig, socketCfg SocketLegacyConfig, dnsCfg DNSLegacyConfig) bool {
	return cachestatCfg.Enabled || socketCfg.Enabled || dnsCfg.Enabled
}
