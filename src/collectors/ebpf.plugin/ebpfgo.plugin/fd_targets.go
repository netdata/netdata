package main

import (
	"fmt"
	"io"
)

// FDTargets holds the two kernel symbols the fd probes attach to.
//
// There is no mode field.  Both probes are attached as return probes: the BPF
// programs read PT_REGS_RC to tell a failed syscall from a successful one, and
// the buffer and arena objects ship only the return-reading variant (see
// fd_loader_libbpf.go).  `ebpf load mode` therefore controls chart visibility
// only, exactly as the C module did — it never influenced attachment there
// either (ebpf_fd_attach_probe attached kprobe and kretprobe unconditionally).
type FDTargets struct {
	Open  string
	Close string
}

// fdOpenCandidates and fdCloseCandidates are ordered by preference, newest
// kernel symbol first, mirroring open_targets[] / close_targets[] in the C
// module.  Order matters: a kernel that exports both names must pick the newer
// one, because that is the symbol the shipped BPF objects were built against.
var (
	fdOpenCandidates  = []string{"do_sys_openat2", "do_sys_open"}
	fdCloseCandidates = []string{"close_fd", "__close_fd"}
)

// resolveFDTargets resolves both attach targets from the live symbol table.
//
// Unlike dcstat, an unresolved target is fatal: there is no usable fallback
// name, and attaching to a symbol that does not exist would fail anyway with a
// far less actionable error.  This mirrors ebpf_fd_set_target_values(), which
// returned -1 and aborted the module load.
func resolveFDTargets() (FDTargets, error) {
	return resolveFDTargetsFrom(openProcKallsyms)
}

func resolveFDTargetsFrom(open kallsymsOpener) (FDTargets, error) {
	symbols, err := open()
	if err != nil {
		return FDTargets{}, fmt.Errorf("fd: cannot read the kernel symbol table: %w", err)
	}
	defer symbols.Close()

	return resolveFDTargetsFromReader(symbols)
}

// resolveFDTargetsFromReader resolves both symbols in a single pass, so the
// multi-MB symbol table is neither scanned twice nor buffered whole.
func resolveFDTargetsFromReader(r io.Reader) (FDTargets, error) {
	resolved, err := selectKallsymsCandidateSets(r, fdOpenCandidates, fdCloseCandidates)
	if err != nil {
		return FDTargets{}, fmt.Errorf("fd: reading the kernel symbol table: %w", err)
	}

	targets := FDTargets{Open: resolved[0], Close: resolved[1]}
	if targets.Open == "" {
		return FDTargets{}, fmt.Errorf(
			"fd: none of the open targets %v exist in the kernel symbol table", fdOpenCandidates)
	}
	if targets.Close == "" {
		return FDTargets{}, fmt.Errorf(
			"fd: none of the close targets %v exist in the kernel symbol table", fdCloseCandidates)
	}

	return targets, nil
}
