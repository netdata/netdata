package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

type DCStatTarget struct {
	Name string
	Mode RunMode
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
		LookupFast: DCStatTarget{
			Name: "lookup_fast",
			Mode: RunModeEntry,
		},
		DLookup: DCStatTarget{
			Name: "d_lookup",
			Mode: RunModeReturn,
		},
	}
}

func resolveDCStatTargets() (DCStatTargets, error) {
	targets := defaultDCStatTargets()
	if err := targets.ResolveLookupFastTarget(); err != nil {
		return DCStatTargets{}, err
	}

	return targets, nil
}

// ResolveLookupFastTarget replaces LookupFast.Name with the concrete kallsyms
// symbol when a suffixed variant is present.  A missing symbol is not an error:
// the plain name is kept so attach fails loudly at load time instead of hiding
// a kallsyms read problem behind a config error.
func (t *DCStatTargets) ResolveLookupFastTarget() error {
	name, err := selectDCStatKallsymsPrefix(t.LookupFast.Name)
	if err != nil {
		return err
	}

	if name != "" {
		t.LookupFast.Name = name
	}
	return nil
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
