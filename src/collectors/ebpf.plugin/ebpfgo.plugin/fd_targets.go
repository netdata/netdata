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

// fdOpenCandidates is ordered by preference, newest kernel symbol first,
// mirroring open_targets[] in the C module.  Both names are ABI-agnostic inner
// functions: open(2), openat(2) and openat2(2) all funnel through
// do_sys_openat2() (linux fs/open.c), so one probe covers every entry point.
var fdOpenCandidates = []string{"do_sys_openat2", "do_sys_open"}

// fdCloseCandidates prefers the architecture's close(2) syscall wrapper over the
// inner helpers, and is the one place this collector deliberately diverges from
// the C module it replaces.
//
// The C module used {"close_fd", "__close_fd"} only, and that list went stale:
// since the close(2) refactor that introduced file_close_fd(), the syscall no
// longer calls close_fd().  On linux 6.18 fs/open.c reads
//
//	SYSCALL_DEFINE1(close, unsigned int, fd) {
//	        file = file_close_fd(fd);            // not close_fd()
//	        retval = filp_flush(file, current->files);
//
// while fs/file.c still defines close_fd(), so the symbol resolves, the probe
// attaches, the module loads — and the counter never moves.  Its only surviving
// callers under fs/ are autofs' dev-ioctl and one path in fs/file.c, neither of
// which runs in normal operation.  That is why the close charts read a flat zero
// on any kernel with the refactor, in the C module as much as here.
//
// file_close_fd() is NOT a usable replacement: it returns struct file * (NULL for
// a bad fd), and the shipped BPF program classifies errors with
// `(int)PT_REGS_RC(ctx) < 0`.  On a pointer that test is effectively random, and
// NULL is not negative, so real failures would be missed.  The syscall wrapper
// returns the long the caller sees — 0 or -errno — so both the call count and the
// error count are exact, on every kernel version, which is why it goes first
// rather than being gated behind a version check.
//
// Only one wrapper name can exist on a given host, so their relative order is
// irrelevant; what matters is that all of them precede the fallbacks.  Archs
// without CONFIG_ARCH_HAS_SYSCALL_WRAPPER (32-bit arm, powerpc, and everything
// before the wrappers existed) expose the plain sys_close.  The two historical
// helpers stay last so a kernel predating the wrappers still resolves.
//
// KNOWN GAP: on x86_64 this selects __x64_sys_close, so closes issued by 32-bit
// processes through the ia32 compat entry (__ia32_sys_close) are not counted.
// Open does not have this asymmetry because it probes an inner function.
// Counting both would need a second close link; see the SOW follow-up.
var fdCloseCandidates = []string{
	"__x64_sys_close",   // x86_64
	"__ia32_sys_close",  // x86 32-bit (also the compat entry on x86_64)
	"__arm64_sys_close", // aarch64
	"__s390x_sys_close", // s390x
	"__s390_sys_close",  // s390 compat
	"__riscv_sys_close", // riscv
	"sys_close",         // powerpc, 32-bit arm, and any kernel without syscall wrappers
	"close_fd",          // inner helper: the close(2) path before file_close_fd() landed
	"__close_fd",        // inner helper: the same, before it was renamed to close_fd()
}

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
