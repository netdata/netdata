package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// DCStatTarget is the kernel symbol one dcstat probe attaches to.  There is no
// mode field: which of the two probes is a return probe is fixed by the BPF
// programs themselves (d_lookup reads PT_REGS_RC), not by configuration, so a
// mode here would be a setting that cannot actually change anything.
type DCStatTarget struct {
	Name string
}

type DCStatTargets struct {
	// LookupFast is a static kernel function, so the compiler frequently emits it
	// with a suffix (lookup_fast.isra.0, lookup_fast.constprop.0, ...).  The name
	// is resolved by prefix match against /proc/kallsyms, mirroring
	// ebpf_find_symbol() + dc_optional_name in the C collector.
	LookupFast DCStatTarget
	// DLookup is an exported symbol, so its name is used verbatim — the same
	// choice the C collector makes (dc_optional_name covers lookup_fast only).
	DLookup DCStatTarget
}

func defaultDCStatTargets() DCStatTargets {
	return DCStatTargets{
		LookupFast: DCStatTarget{Name: "lookup_fast"},
		DLookup:    DCStatTarget{Name: "d_lookup"},
	}
}

// kallsymsOpener opens the kernel symbol table.  It exists so the degrade path
// (unreadable or truncated symbol table) is reachable from tests without
// depending on the host's /proc.
type kallsymsOpener func() (io.ReadCloser, error)

func openProcKallsyms() (io.ReadCloser, error) {
	return os.Open("/proc/kallsyms")
}

// resolveDCStatTargets returns the attach targets for this kernel.
func resolveDCStatTargets() DCStatTargets {
	return resolveDCStatTargetsFrom(openProcKallsyms)
}

// resolveDCStatTargetsFrom resolves the targets from an arbitrary symbol table.
//
// A read failure is never fatal: the configured default name is kept and a
// warning is emitted.  dcstat is disabled by default, and even when enabled it
// must not take down the collectors sharing this process — if the symbol really
// is wrong, attach fails and only dcstat is lost.
func resolveDCStatTargetsFrom(open kallsymsOpener) DCStatTargets {
	targets := defaultDCStatTargets()

	warn := func(err error) {
		rateLimitedStderr("dcstat.kallsyms",
			"ebpf-go.plugin: dcstat: cannot resolve %q from the kernel symbol table (%v); attaching to it as-is\n",
			targets.LookupFast.Name, err)
	}

	symbols, err := open()
	if err != nil {
		warn(err)
		return targets
	}
	defer symbols.Close()

	name, err := selectDCStatKallsymsPrefixFromReader(targets.LookupFast.Name, symbols)
	if err != nil {
		warn(err)
		return targets
	}

	targets.ResolveLookupFastTarget(name)
	return targets
}

// ResolveLookupFastTarget adopts a resolved kallsyms symbol name.  An empty
// name means the symbol was not found, in which case the configured default is
// kept so the failure surfaces at attach time rather than as a config error.
func (t *DCStatTargets) ResolveLookupFastTarget(resolved string) {
	if resolved != "" {
		t.LookupFast.Name = resolved
	}
}

// selectDCStatKallsymsPrefixFromReader returns the first probeable symbol whose
// name starts with prefix, or "" when none matches.
func selectDCStatKallsymsPrefixFromReader(prefix string, r io.Reader) (string, error) {
	if prefix == "" {
		return "", fmt.Errorf("no dcstat kallsyms prefix configured")
	}

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}

		if !isProbeableKallsymsType(fields[1]) {
			continue
		}

		if strings.HasPrefix(fields[2], prefix) {
			return fields[2], nil
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	return "", nil
}
