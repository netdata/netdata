//go:build cgo

// SPDX-License-Identifier: GPL-3.0-or-later

package native

/*
#cgo CFLAGS: -std=c11 -Wall -Wextra
#cgo linux LDFLAGS: -lm
#include <stdlib.h>
#include "native.h"
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/netdata/netdata/go/plugins/plugin/apps/internal/model"
)

// Fail compilation if the private C ABI and Go model disagree on array sizes.
var (
	_ [model.MetricCount - int(C.METRIC_COUNT)]struct{}
	_ [int(C.METRIC_COUNT) - model.MetricCount]struct{}
	_ [model.FDTypeCount - int(C.FD_COUNT)]struct{}
	_ [int(C.FD_COUNT) - model.FDTypeCount]struct{}
)

// Scanner owns all C state. Calls serialize; Close can safely race a caller.
type Scanner struct {
	mu  sync.Mutex
	ptr *C.apps_scanner
}

func New(options Options) (*Scanner, error) {
	if options.ProcPath == "" {
		options.ProcPath = "/proc"
	}
	if strings.ContainsRune(options.ProcPath, '\x00') {
		return nil, errors.New("proc path contains a NUL byte")
	}
	root := C.CString(options.ProcPath)
	defer C.free(unsafe.Pointer(root))
	var fds, pss C.int
	if options.CollectFDs {
		fds = 1
	}
	if options.CollectPSS {
		pss = 1
	}
	ptr := C.apps_new(root, fds, pss)
	if ptr == nil {
		return nil, errors.New("allocate native process scanner")
	}
	return &Scanner{ptr: ptr}, nil
}

// Scan performs one synchronous native batch. Cancellation is checked before and
// after the call; it cannot interrupt a kernel procfs read already in progress.
func (s *Scanner) Scan(ctx context.Context) (model.Snapshot, error) { return s.scan(ctx, 0) }

func (s *Scanner) scan(ctx context.Context, now float64) (model.Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return model.Snapshot{}, err
	}
	if s.ptr == nil {
		return model.Snapshot{}, errors.New("native scanner is closed")
	}
	var snapshot *C.apps_snapshot
	if rc := C.apps_scan(s.ptr, C.double(now), &snapshot); rc != 0 {
		return model.Snapshot{}, fmt.Errorf("scan procfs: %w", syscall.Errno(rc))
	}
	if err := ctx.Err(); err != nil {
		return model.Snapshot{}, err
	}
	out := model.Snapshot{
		Generation:     uint64(snapshot.generation),
		CollectedAt:    time.Now(),
		Processes:      make([]model.Process, int(snapshot.count)),
		SystemCPUValid: snapshot.cpu_valid != 0,
		CPUCount:       int(snapshot.cpu_count),
		Stats:          model.ScanStats{FileReads: uint64(snapshot.file_reads), FDLinksRead: uint64(snapshot.link_reads), ReadErrors: uint64(snapshot.read_errors)},
	}
	for i := range out.SystemCPU {
		out.SystemCPU[i] = float64(snapshot.cpu[i])
	}
	for i, row := range unsafe.Slice(snapshot.rows, int(snapshot.count)) {
		p := &out.Processes[i]
		p.Key = model.Key{PID: int32(row.pid), StartTime: uint64(row.start)}
		p.PPID, p.UID, p.GID = int32(row.ppid), uint32(row.uid), uint32(row.gid)
		p.Comm, p.Cmdline, p.State = C.GoString(row.comm), C.GoString(row.cmdline), string(byte(row.state))
		p.Valid, p.FDValid = uint64(row.valid), row.fd_valid != 0
		for j := range p.Values {
			p.Values[j] = float64(row.values[j])
		}
		for j := range p.FDCounts {
			p.FDCounts[j] = uint64(row.fds[j])
		}
	}
	return out, nil
}

// Finalize requires exactly one assignment for each row in the current scan.
func (s *Scanner) Finalize(generation uint64, assignments []model.Assignment) ([]model.GroupFD, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ptr == nil {
		return nil, errors.New("native scanner is closed")
	}
	var data *C.apps_assignment
	if len(assignments) > 0 {
		data = (*C.apps_assignment)(C.calloc(C.size_t(len(assignments)), C.size_t(C.sizeof_apps_assignment)))
		if data == nil {
			return nil, errors.New("allocate native assignments")
		}
		defer C.free(unsafe.Pointer(data))
		for i, a := range assignments {
			row := &unsafe.Slice(data, len(assignments))[i]
			row.pid, row.start = C.int32_t(a.Key.PID), C.uint64_t(a.Key.StartTime)
			for j := range a.Groups {
				row.groups[j] = C.uint32_t(a.Groups[j])
			}
		}
	}
	var groups *C.apps_group_fd
	var count C.size_t
	if rc := C.apps_finalize(s.ptr, C.uint64_t(generation), data, C.size_t(len(assignments)), &groups, &count); rc != 0 {
		return nil, fmt.Errorf("finalize process snapshot: %w", syscall.Errno(rc))
	}
	out := make([]model.GroupFD, int(count))
	for i, group := range unsafe.Slice(groups, int(count)) {
		out[i].ID, out[i].Valid = uint32(group.id), group.valid != 0
		for j := range out[i].Counts {
			out[i].Counts[j] = uint64(group.counts[j])
		}
	}
	return out, nil
}

func (s *Scanner) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ptr != nil {
		C.apps_close(s.ptr)
		s.ptr = nil
	}
}
