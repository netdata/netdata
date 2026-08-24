package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// kallsymsOpener opens the kernel symbol table.  It exists so the degrade paths
// (unreadable or truncated symbol table) are reachable from tests without
// depending on the host's /proc.
type kallsymsOpener func() (io.ReadCloser, error)

func openProcKallsyms() (io.ReadCloser, error) {
	return os.Open("/proc/kallsyms")
}

// isProbeableKallsymsType reports whether a /proc/kallsyms type letter denotes a
// symbol a kprobe can attach to: text and weak-text, global or local.
func isProbeableKallsymsType(value string) bool {
	switch value {
	case "T", "t", "W", "w":
		return true
	default:
		return false
	}
}

// selectKallsymsPrefix returns the first probeable symbol whose name starts with
// prefix, or "" when none matches.
//
// Used for static kernel functions the compiler frequently emits with a suffix
// (lookup_fast.isra.0, lookup_fast.constprop.0, ...), mirroring
// ebpf_find_symbol() in the C collector.
func selectKallsymsPrefix(prefix string, r io.Reader) (string, error) {
	if prefix == "" {
		return "", fmt.Errorf("no kallsyms prefix configured")
	}

	return scanKallsyms(r, func(symbol string) (string, bool) {
		if strings.HasPrefix(symbol, prefix) {
			return symbol, true
		}
		return "", false
	})
}

// selectKallsymsCandidate returns the first name in candidates that exists in
// the symbol table as a probeable symbol, or "" when none does.
//
// The result follows CANDIDATE order, not symbol-table order: candidate lists
// encode a kernel-version preference (do_sys_openat2 before do_sys_open,
// close_fd before __close_fd), and /proc/kallsyms is ordered by address, so
// returning the first table line that matched anything would pick arbitrarily
// whenever two candidates coexist.  That is why the whole table is scanned
// before choosing instead of returning on first hit.
func selectKallsymsCandidate(candidates []string, r io.Reader) (string, error) {
	resolved, err := selectKallsymsCandidateSets(r, candidates)
	if err != nil {
		return "", err
	}

	return resolved[0], nil
}

// selectKallsymsCandidateSets resolves several candidate lists in ONE pass over
// the symbol table, returning one name per list ("" for a list with no match).
//
// /proc/kallsyms is not seekable and is several MB on a typical kernel, so a
// caller needing two targets (fd's open and close) must not scan it twice nor
// buffer it whole.  Per-list semantics are identical to selectKallsymsCandidate.
func selectKallsymsCandidateSets(r io.Reader, lists ...[]string) ([]string, error) {
	if len(lists) == 0 {
		return nil, fmt.Errorf("no kallsyms candidate lists configured")
	}

	// Only the candidates are retained, so the map is bounded by the callers'
	// lists rather than by the size of the symbol table.
	wanted := make(map[string]struct{})
	for _, candidates := range lists {
		for _, candidate := range candidates {
			if candidate != "" {
				wanted[candidate] = struct{}{}
			}
		}
	}
	if len(wanted) == 0 {
		return nil, fmt.Errorf("no kallsyms candidates configured")
	}

	found := make(map[string]struct{}, len(wanted))
	if _, err := scanKallsyms(r, func(symbol string) (string, bool) {
		if _, ok := wanted[symbol]; ok {
			found[symbol] = struct{}{}
		}
		// Never stop early: a later line may hold a higher-priority candidate,
		// and other lists still need their symbols from the rest of the table.
		return "", false
	}); err != nil {
		return nil, err
	}

	resolved := make([]string, len(lists))
	for i, candidates := range lists {
		for _, candidate := range candidates {
			if _, ok := found[candidate]; ok {
				resolved[i] = candidate
				break
			}
		}
	}

	return resolved, nil
}

// scanKallsyms walks a /proc/kallsyms stream and hands every probeable symbol to
// match, returning the first value for which match reports true.  It returns ""
// with a nil error when the stream is exhausted without a match, so callers can
// distinguish "not found" from a read failure.
func scanKallsyms(r io.Reader, match func(symbol string) (string, bool)) (string, error) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}

		if !isProbeableKallsymsType(fields[1]) {
			continue
		}

		if value, ok := match(fields[2]); ok {
			return value, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	return "", nil
}
