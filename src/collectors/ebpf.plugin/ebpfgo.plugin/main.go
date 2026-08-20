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
	// Cap the Go scheduler to 6 OS threads: one per active collector goroutine
	// (cachestat, dcstat, socket, dns), one for the signal handler, and one for
	// the stdin dispatcher goroutine that blocks on os.Stdin reads.  The default
	// GOMAXPROCS = NumCPU allocates O(ncpus) scheduler threads, and CGO calls
	// on blocked goroutines cause the runtime to create up to O(ncpus)
	// additional threads — each carrying an 8 MB Linux stack.  On a 64-core
	// host that is ~130 threads and ~1 GB of stack RSS for no benefit.
	runtime.GOMAXPROCS(6)

	updateEvery, dcstatOnly := parsePluginArgs(os.Args[1:])

	cachestatCfg, err := resolveCachestatLegacyConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ebpf-go.plugin: cachestat config load failed: %v\n", err)
		os.Exit(1)
	}

	dcstatCfg, err := resolveDCStatLegacyConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ebpf-go.plugin: dcstat config load failed: %v\n", err)
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
	if dcstatOnly {
		// The legacy C plugin treated --dcstat as a module-selection flag: it
		// disabled every other collector and enabled dcstat regardless of config.
		cachestatCfg.Enabled = false
		socketCfg.Enabled = false
		dnsCfg.Enabled = false
		dcstatCfg.Enabled = true
		// resolveDCStatLegacyConfig() skips /proc/kallsyms when the module is
		// disabled in config, so --dcstat has to resolve the targets itself or it
		// keeps the hardcoded lookup_fast name.  lookup_fast is a static kernel
		// function and is frequently emitted suffixed (lookup_fast.isra.0), and
		// legacy mode has no attach fallback, so skipping this makes --dcstat fail
		// to load on those kernels.  The C module resolved names unconditionally.
		dcstatCfg.Targets = resolveDCStatTargets()
	}

	if !anyProgramEnabled(cachestatCfg, dcstatCfg, socketCfg, dnsCfg) {
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

	// The shared store must exist before any collector starts so every module's
	// per-PID rows land in the same shared-memory snapshot.
	var store *ebpfSharedMemoryStore
	needsStore := socketCfg.Enabled ||
		(cachestatCfg.Enabled && (cachestatCfg.AppsEnabled || cachestatCfg.CgroupsEnabled)) ||
		(dcstatCfg.Enabled && (dcstatCfg.AppsEnabled || dcstatCfg.CgroupsEnabled))
	if needsStore {
		store = NewEbpfSharedMemoryStore()
	}

	// The stdin dispatcher is always started so it can handle function calls
	// from whichever subset of collectors is enabled.
	var fnStore *socketFunctionStore

	// Exactly one module owns the shared-memory segment; the others only
	// contribute rows to the shared store.  Ownership order is cachestat,
	// then dcstat, then socket, so a module's cgroup/apps charts keep working
	// whichever subset of modules the operator enabled.
	var cachestatWillPublish bool
	var dcstatWillPublish bool

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
			var cachestatStore *ebpfSharedMemoryStore
			if handle.AppsEnabled || handle.CgroupsEnabled {
				cachestatStore = store
				cachestatWillPublish = true
			}
			anyStarted = true
			wg.Go(func() {
				runCachestatGlobalCollector(api, handle, stop, cachestatStore, ue)
				handle.Close()
			})
		}
	}

	// ---- dcstat ----
	if dcstatCfg.Enabled {
		ue := resolveUpdateEvery(updateEvery, dcstatCfg.UpdateEvery, dcstatDefaultUpdateEvery)
		dcstatCfg.UpdateEvery = ue

		handle, herr := LoadDCStatLegacy(dcstatCfg)
		if herr != nil {
			fmt.Fprintf(os.Stderr, "ebpf-go.plugin: dcstat load failed: %v\n", herr)
		} else if handle != nil && handle.Runtime != nil {
			// Only propagate store to dcstat when it has apps/cgroups consumers;
			// that is what triggers per-PID collection and SHM publishing.
			var dcstatStore *ebpfSharedMemoryStore
			if handle.AppsEnabled || handle.CgroupsEnabled {
				dcstatStore = store
				dcstatWillPublish = !cachestatWillPublish
			} else {
				// dcstat is opt-in, so reaching here means an operator enabled it
				// and will otherwise wonder why only the global charts appeared.
				warnIntegrationDisabled("dcstat")
			}
			shouldPublish := dcstatWillPublish
			anyStarted = true
			wg.Go(func() {
				runDCStatGlobalCollector(api, handle, stop, dcstatStore, ue, shouldPublish)
				if dcstatStore != nil {
					dcstatStore.MarkDCStatInactive()
				}
				handle.Close()
			})
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

			// Socket owns the SHM publisher only when neither cachestat nor
			// dcstat is publishing; this lets socket cgroup charts work
			// independently of the other two modules.
			socketShouldPublish := store != nil && !cachestatWillPublish && !dcstatWillPublish

			wg.Go(func() {
				runSocketGlobalCollector(api, handle, stop, ue, store, fnStore, socketShouldPublish)
				handle.Close()
			})
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
			anyStarted = true

			wg.Go(func() {
				runDNSGlobalCollector(handle, stop, ue)
				handle.Close()
			})
		}
	}

	// Start stdin dispatcher after all function stores are initialised so the
	// dispatcher sees a consistent fnStore pointer.
	go runStdinDispatcher(api, fnStore, closeStop)

	if !anyStarted {
		fmt.Fprintf(os.Stderr, "ebpf-go.plugin: all enabled programs failed to load\n")
		os.Exit(1)
	}

	wg.Wait()
}

// parsePluginArgs keeps the legacy numeric pluginsd interval and dcstat's
// module-selection flag. Unknown arguments remain ignored as they were before
// this compatibility parser was added.
func parsePluginArgs(args []string) (updateEvery int, dcstatOnly bool) {
	for _, arg := range args {
		switch arg {
		case "--dcstat", "-dcstat":
			dcstatOnly = true
		default:
			if parsed, err := strconv.Atoi(arg); err == nil && parsed > 0 && updateEvery == 0 {
				updateEvery = parsed
			}
		}
	}
	return updateEvery, dcstatOnly
}

// resolveUpdateEvery returns the first positive value from: config file, CLI arg, fallback.
// Config is the operator-controlled source of truth. The legacy numeric CLI
// interval is only a fallback when no config value is set.
func resolveUpdateEvery(cliArg, cfgVal, fallback int) int {
	if cfgVal > 0 {
		return cfgVal
	}
	if cliArg > 0 {
		return cliArg
	}
	return fallback
}

// anyProgramEnabled returns true when at least one eBPF program is enabled.
// The plugin exits early only when every known program is disabled so that
// adding a new program requires only a new field here, not a structural change.
func anyProgramEnabled(
	cachestatCfg CachestatLegacyConfig,
	dcstatCfg DCStatLegacyConfig,
	socketCfg SocketLegacyConfig,
	dnsCfg DNSLegacyConfig,
) bool {
	return cachestatCfg.Enabled || dcstatCfg.Enabled || socketCfg.Enabled || dnsCfg.Enabled
}
