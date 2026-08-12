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

// resolveDCStatTargets returns the attach targets for this kernel.
//
// A /proc/kallsyms read failure is never fatal: the configured default name is
// kept and a warning is emitted.  dcstat is disabled by default, and even when
// enabled it must not take down the collectors sharing this process — if the
// symbol really is wrong, attach fails and only dcstat is lost.
func resolveDCStatTargets() DCStatTargets {
	targets := defaultDCStatTargets()

	name, err := selectDCStatKallsymsPrefix(targets.LookupFast.Name)
	if err != nil {
		rateLimitedStderr("dcstat.kallsyms",
			fmt.Sprintf("ebpf-go.plugin: dcstat: cannot read /proc/kallsyms (%v); attaching to %q as-is\n",
				err, targets.LookupFast.Name))
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

func selectDCStatKallsymsPrefix(prefix string) (string, error) {
	if prefix == "" {
		return "", fmt.Errorf("no dcstat kallsyms prefix configured")
	}

	file, err := os.Open("/proc/kallsyms")
	if err != nil {
		return "", err
	}
	defer file.Close()

	return selectDCStatKallsymsPrefixFromReader(prefix, file)
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
