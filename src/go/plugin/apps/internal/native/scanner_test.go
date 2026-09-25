//go:build cgo

// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/apps/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type procFixture struct {
	root   string
	clock  float64
	system uint64
}

func newFixture(t testing.TB) *procFixture {
	t.Helper()
	f := &procFixture{root: t.TempDir()}
	f.write(t, "uptime", "1000 0\n")
	return f
}
func (f *procFixture) write(t testing.TB, name, data string) {
	t.Helper()
	p := filepath.Join(f.root, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0755))
	require.NoError(t, os.WriteFile(p, []byte(data), 0644))
}
func (f *procFixture) stat(t testing.TB, pid, ppid int, start, user, child uint64, comm string) {
	t.Helper()
	fields := make([]string, 50)
	for i := range fields {
		fields[i] = "0"
	}
	set := func(field int, v uint64) { fields[field-3] = strconv.FormatUint(v, 10) }
	fields[0] = "S"
	set(4, uint64(ppid))
	set(10, user*2)
	set(11, child*2)
	set(12, user)
	set(13, child)
	set(14, user)
	set(16, child)
	set(20, 2)
	set(22, start)
	set(23, 1048576)
	set(24, 20)
	f.write(t, fmt.Sprintf("%d/stat", pid), fmt.Sprintf("%d (%s) %s\n", pid, comm, strings.Join(fields, " ")))
}
func (f *procFixture) proc(t testing.TB, pid, ppid int, start, user, child uint64) {
	t.Helper()
	f.stat(t, pid, ppid, start, user, child, "worker (helper) with ) spaces")
	f.write(t, fmt.Sprintf("%d/status", pid), "Uid:\t1000 1000 1000 1000\nGid:\t1001 1001 1001 1001\nVmSize: 1024 kB\nVmRSS: 100 kB\nRssFile: 20 kB\nRssShmem: 10 kB\nVmSwap: 5 kB\nvoluntary_ctxt_switches: 10\nnonvoluntary_ctxt_switches: 2\n")
	f.write(t, fmt.Sprintf("%d/io", pid), fmt.Sprintf("read_bytes: %d\nwrite_bytes: %d\nrchar: %d\nwchar: %d\nsyscr: %d\nsyscw: %d\n", user*1024, user*512, user*2048, user*1024, user*2, user))
	f.write(t, fmt.Sprintf("%d/cmdline", pid), "worker\x00--flag\x00")
	f.write(t, fmt.Sprintf("%d/limits", pid), "Limit                     Soft Limit           Hard Limit           Units\nMax open files            100                  100                  files\n")
	f.write(t, fmt.Sprintf("%d/smaps_rollup", pid), "0000-ffff ---p 00000000 00:00 0 [rollup]\nPss: 60 kB\n")
	require.NoError(t, os.MkdirAll(filepath.Join(f.root, strconv.Itoa(pid), "fd"), 0755))
}
func (f *procFixture) scanner(t testing.TB, fds, pss bool) *Scanner {
	t.Helper()
	s, err := New(Options{ProcPath: f.root, CollectFDs: fds, CollectPSS: pss})
	require.NoError(t, err)
	t.Cleanup(s.Close)
	return s
}
func (f *procFixture) scan(t testing.TB, s *Scanner) model.Snapshot {
	t.Helper()
	f.clock++
	f.system += 1000000
	f.write(t, "stat", fmt.Sprintf("cpu %d 0 %d 0 0 0 0 0 0 0\ncpu0 0\ncpu1 0\n", f.system, f.system))
	snap, err := s.scan(context.Background(), f.clock)
	require.NoError(t, err)
	return snap
}
func finish(t testing.TB, s *Scanner, snap model.Snapshot) []model.GroupFD {
	t.Helper()
	assignments := make([]model.Assignment, len(snap.Processes))
	for i, p := range snap.Processes {
		assignments[i] = model.Assignment{Key: p.Key, Groups: [3]uint32{1, 2, 3}}
	}
	groups, err := s.Finalize(snap.Generation, assignments)
	require.NoError(t, err)
	return groups
}

func TestSnapshotRatesAndOwnedValues(t *testing.T) {
	f := newFixture(t)
	f.proc(t, 100, 0, 10, 100, 0)
	s := f.scanner(t, true, true)
	first := f.scan(t, s)
	require.Len(t, first.Processes, 1)
	p := first.Processes[0]
	assert.Equal(t, "worker (helper) with ) spaces", p.Comm)
	assert.Equal(t, "worker --flag", p.Cmdline)
	assert.Equal(t, uint32(1000), p.UID)
	assert.Equal(t, uint32(1001), p.GID)
	assert.False(t, p.Has(model.CPUUser))
	assert.False(t, p.Has(model.ReadBytes))
	assert.True(t, p.FDValid)
	assert.Equal(t, 102400.0, p.Values[model.ResidentMemory])
	assert.Equal(t, 61440.0, p.Values[model.ProportionalMemory])
	assert.Equal(t, 61440.0, p.Values[model.EstimatedMemory])
	finish(t, s, first)
	f.proc(t, 100, 0, 10, 120, 0)
	second := f.scan(t, s)
	p = second.Processes[0]
	assert.True(t, p.Has(model.CPUUser))
	assert.Greater(t, p.Values[model.CPUUser], 0.0)
	assert.Equal(t, 40.0, p.Values[model.MinorFaults])
	assert.Equal(t, 20480.0, p.Values[model.ReadBytes])
	assert.True(t, second.SystemCPUValid)
	assert.Equal(t, 2, second.CPUCount)
	finish(t, s, second)
	s.Close()
	assert.Equal(t, "worker --flag", first.Processes[0].Cmdline)
	s.Close()
	_, err := s.Scan(context.Background())
	require.ErrorContains(t, err, "closed")
}
func TestCounterResetAndPIDReuse(t *testing.T) {
	f := newFixture(t)
	f.proc(t, 100, 0, 10, 100, 0)
	s := f.scanner(t, true, true)
	finish(t, s, f.scan(t, s))
	f.proc(t, 100, 0, 10, 90, 0)
	snap := f.scan(t, s)
	assert.False(t, snap.Processes[0].Has(model.CPUUser))
	assert.False(t, snap.Processes[0].Has(model.ReadBytes))
	finish(t, s, snap)
	f.proc(t, 100, 0, 10, 95, 0)
	snap = f.scan(t, s)
	assert.True(t, snap.Processes[0].Has(model.CPUUser))
	finish(t, s, snap)
	f.proc(t, 100, 0, 20, 1000, 0)
	snap = f.scan(t, s)
	assert.Equal(t, uint64(20), snap.Processes[0].Key.StartTime)
	assert.False(t, snap.Processes[0].Has(model.CPUUser))
	assert.False(t, snap.Processes[0].Has(model.ReadBytes))
	finish(t, s, snap)
}
func TestReadFailuresAndRecovery(t *testing.T) {
	f := newFixture(t)
	f.proc(t, 100, 0, 10, 100, 0)
	s := f.scanner(t, true, true)
	finish(t, s, f.scan(t, s))
	require.NoError(t, os.Remove(filepath.Join(f.root, "100/io")))
	require.NoError(t, os.Remove(filepath.Join(f.root, "100/smaps_rollup")))
	require.NoError(t, os.Remove(filepath.Join(f.root, "100/fd")))
	f.stat(t, 100, 0, 10, 110, 0, "worker")
	snap := f.scan(t, s)
	p := snap.Processes[0]
	assert.False(t, p.Has(model.ReadBytes))
	assert.False(t, p.Has(model.ProportionalMemory))
	assert.False(t, p.FDValid)
	assert.True(t, p.Has(model.CPUUser))
	assert.False(t, finish(t, s, snap)[0].Valid)
	f.proc(t, 100, 0, 10, 130, 0)
	snap = f.scan(t, s)
	assert.Equal(t, 15360.0, snap.Processes[0].Values[model.ReadBytes])
	finish(t, s, snap)
	require.NoError(t, os.Remove(filepath.Join(f.root, "100/status")))
	snap = f.scan(t, s)
	assert.Empty(t, snap.Processes)
	finish(t, s, snap)
	f.proc(t, 100, 0, 10, 140, 0)
	snap = f.scan(t, s)
	require.Len(t, snap.Processes, 1)
	assert.True(t, snap.Processes[0].Has(model.CPUUser))
	finish(t, s, snap)
}
func TestMalformedStatAndDisappearance(t *testing.T) {
	f := newFixture(t)
	f.proc(t, 100, 0, 10, 100, 0)
	s := f.scanner(t, false, false)
	finish(t, s, f.scan(t, s))
	f.write(t, "100/stat", "100 (unterminated name S 0 0\n")
	snap := f.scan(t, s)
	assert.Empty(t, snap.Processes)
	finish(t, s, snap)
	require.NoError(t, os.RemoveAll(filepath.Join(f.root, "100")))
	snap = f.scan(t, s)
	assert.Empty(t, snap.Processes)
	finish(t, s, snap)
	f.proc(t, 100, 0, 10, 150, 0)
	snap = f.scan(t, s)
	require.Len(t, snap.Processes, 1)
	assert.False(t, snap.Processes[0].Has(model.CPUUser))
	finish(t, s, snap)
}
func TestExitedChildReconciliation(t *testing.T) {
	f := newFixture(t)
	f.proc(t, 100, 0, 10, 100, 0)
	f.proc(t, 101, 100, 11, 20, 0)
	s := f.scanner(t, false, false)
	finish(t, s, f.scan(t, s))
	f.proc(t, 100, 0, 10, 110, 0)
	f.proc(t, 101, 100, 11, 30, 0)
	snap := f.scan(t, s)
	childCPU := snap.Processes[1].Values[model.CPUUser]
	finish(t, s, snap)
	require.NoError(t, os.RemoveAll(filepath.Join(f.root, "101")))
	f.proc(t, 100, 0, 10, 120, 40)
	snap = f.scan(t, s)
	require.Len(t, snap.Processes, 1)
	assert.InDelta(t, childCPU, snap.Processes[0].Values[model.CPUChildrenUser], 0.001)
	assert.Equal(t, 20.0, snap.Processes[0].Values[model.ChildrenMinorFaults])
	assert.Equal(t, 10.0, snap.Processes[0].Values[model.ChildrenMajorFaults])
	finish(t, s, snap)
}
func TestDelayedChildReap(t *testing.T) {
	f := newFixture(t)
	f.proc(t, 100, 0, 10, 100, 0)
	f.proc(t, 101, 100, 11, 20, 0)
	s := f.scanner(t, false, false)
	finish(t, s, f.scan(t, s))
	require.NoError(t, os.RemoveAll(filepath.Join(f.root, "101")))
	f.proc(t, 100, 0, 10, 110, 0)
	finish(t, s, f.scan(t, s))
	f.proc(t, 100, 0, 10, 120, 20)
	snap := f.scan(t, s)
	assert.Zero(t, snap.Processes[0].Values[model.CPUChildrenUser])
	assert.Zero(t, snap.Processes[0].Values[model.ChildrenMinorFaults])
	finish(t, s, snap)
}
func TestFDDedupCacheAndReuse(t *testing.T) {
	f := newFixture(t)
	f.proc(t, 100, 0, 10, 100, 0)
	f.proc(t, 101, 0, 10, 100, 0)
	for _, pid := range []int{100, 101} {
		for fd, name := range []string{"/shared/file", "socket:[42]", "pipe:[7]", "anon_inode:[eventfd]"} {
			require.NoError(t, os.Symlink(name, filepath.Join(f.root, strconv.Itoa(pid), "fd", strconv.Itoa(fd))))
		}
	}
	s := f.scanner(t, true, false)
	snap := f.scan(t, s)
	assert.Equal(t, uint64(1), snap.Processes[0].FDCounts[model.FDFile])
	assert.Equal(t, 4.0, snap.Processes[0].Values[model.FDLimitPercent])
	groups := finish(t, s, snap)
	require.Len(t, groups, 3)
	assert.Equal(t, uint64(1), groups[0].Counts[model.FDFile])
	assert.Equal(t, uint64(1), groups[0].Counts[model.FDSocket])
	assert.True(t, groups[0].Valid)
	finish(t, s, f.scan(t, s))
	finish(t, s, f.scan(t, s))
	cached := f.scan(t, s)
	assert.Zero(t, cached.Stats.FDLinksRead)
	finish(t, s, cached)
	require.NoError(t, os.Remove(filepath.Join(f.root, "100/fd/0")))
	require.NoError(t, os.Symlink("socket:[99]", filepath.Join(f.root, "100/fd/0")))
	f.stat(t, 100, 0, 20, 200, 0, "reused")
	snap = f.scan(t, s)
	assert.Zero(t, snap.Processes[0].FDCounts[model.FDFile])
	assert.Equal(t, uint64(2), snap.Processes[0].FDCounts[model.FDSocket])
	finish(t, s, snap)
}
func TestPSSAgeAndGroupRefresh(t *testing.T) {
	f := newFixture(t)
	for pid := 100; pid < 104; pid++ {
		f.proc(t, pid, 0, 10, 100, 0)
	}
	s := f.scanner(t, false, true)
	first := f.scan(t, s)
	finish(t, s, first)
	second := f.scan(t, s)
	finish(t, s, second)
	assert.Greater(t, second.Processes[3].Values[model.PSSAge], 0.0)
	f.write(t, "103/smaps_rollup", "Pss: 25 kB\n")
	assignments := make([]model.Assignment, len(second.Processes))
	for i, p := range second.Processes {
		assignments[i] = model.Assignment{Key: p.Key, Groups: [3]uint32{1, 2, 3}}
	}
	assignments[3].Groups = [3]uint32{4, 5, 6}
	_, err := s.Finalize(second.Generation, assignments)
	require.NoError(t, err)
	var latest model.Snapshot
	for i := 0; i < 4; i++ {
		latest = f.scan(t, s)
		_, err = s.Finalize(latest.Generation, assignments)
		require.NoError(t, err)
	}
	assert.Equal(t, 25600.0, latest.Processes[3].Values[model.ProportionalMemory])
}
func TestFinalizeValidationCancellationAndInstances(t *testing.T) {
	f := newFixture(t)
	f.proc(t, 100, 0, 10, 100, 0)
	s := f.scanner(t, false, false)
	other := f.scanner(t, false, false)
	snap := f.scan(t, s)
	_, err := s.Finalize(snap.Generation+1, nil)
	require.Error(t, err)
	_, err = s.Finalize(snap.Generation, []model.Assignment{{Key: model.Key{PID: 100, StartTime: 11}}})
	require.Error(t, err)
	_, err = s.Finalize(snap.Generation, nil)
	require.Error(t, err)
	finish(t, s, snap)
	assert.False(t, f.scan(t, other).Processes[0].Has(model.CPUUser))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = s.Scan(ctx)
	require.ErrorIs(t, err, context.Canceled)
	require.NoError(t, os.RemoveAll(f.root))
	_, err = s.Scan(context.Background())
	require.Error(t, err)
}
func BenchmarkScan100Processes(b *testing.B) {
	f := newFixture(b)
	for i := 1; i <= 100; i++ {
		f.proc(b, i, 0, 1, 100, 0)
	}
	s := f.scanner(b, true, true)
	f.write(b, "stat", "cpu 100 0 100 0 0 0 0 0 0 0\ncpu0 0\n")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		snap, err := s.scan(context.Background(), float64(i+1))
		if err != nil {
			b.Fatal(err)
		}
		finish(b, s, snap)
	}
}

func TestMalformedNumericStat(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(string) string
	}{
		{"negative pid", func(s string) string { return "-" + s }},
		{"extra prefix", func(s string) string { return strings.Replace(s, "100 (", "100 bad (", 1) }},
		{"invalid counter", func(s string) string { return strings.Replace(s, " 200 ", " 200x ", 1) }},
		{"overflow counter", func(s string) string { return strings.Replace(s, " 200 ", " 18446744073709551616 ", 1) }},
		{"short fields", func(s string) string { return "100 (name) S 0 0 0\n" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newFixture(t)
			f.proc(t, 100, 0, 10, 100, 0)
			data, err := os.ReadFile(filepath.Join(f.root, "100/stat"))
			require.NoError(t, err)
			f.write(t, "100/stat", test.mutate(string(data)))
			s := f.scanner(t, false, false)
			assert.Empty(t, f.scan(t, s).Processes)
		})
	}
}
func TestProcFilesDoNotFollowSymlinks(t *testing.T) {
	f := newFixture(t)
	f.proc(t, 100, 0, 10, 100, 0)
	s := f.scanner(t, false, false)
	require.NoError(t, os.Rename(filepath.Join(f.root, "100/stat"), filepath.Join(f.root, "stat-target")))
	require.NoError(t, os.Symlink(filepath.Join(f.root, "stat-target"), filepath.Join(f.root, "100/stat")))
	assert.Empty(t, f.scan(t, s).Processes)
}
func TestFinalizeRejectsDuplicateAssignments(t *testing.T) {
	f := newFixture(t)
	f.proc(t, 100, 0, 10, 100, 0)
	f.proc(t, 101, 0, 10, 100, 0)
	s := f.scanner(t, false, false)
	snap := f.scan(t, s)
	_, err := s.Finalize(snap.Generation, []model.Assignment{{Key: snap.Processes[0].Key}, {Key: snap.Processes[0].Key}})
	require.Error(t, err)
	finish(t, s, snap)
}
func BenchmarkScanWarmFDs200x20(b *testing.B) {
	f := newFixture(b)
	for i := 1; i <= 200; i++ {
		f.proc(b, i, 0, 1, 100, 0)
		for fd := 0; fd < 20; fd++ {
			require.NoError(b, os.Symlink(fmt.Sprintf("/shared/file/%d", fd), filepath.Join(f.root, strconv.Itoa(i), "fd", strconv.Itoa(fd))))
		}
	}
	s := f.scanner(b, true, true)
	f.write(b, "stat", "cpu 100 0 100 0 0 0 0 0 0 0\ncpu0 0\n")
	for i := 0; i < 20; i++ {
		snap, err := s.scan(context.Background(), float64(i+1))
		require.NoError(b, err)
		finish(b, s, snap)
	}
	assignments := make([]model.Assignment, 200)
	for i := range assignments {
		assignments[i] = model.Assignment{Key: model.Key{PID: int32(i + 1), StartTime: 1}, Groups: [3]uint32{1, 2, 3}}
	}
	b.ReportAllocs()
	b.ResetTimer()
	var links, reads uint64
	for i := 0; i < b.N; i++ {
		snap, err := s.scan(context.Background(), 20+float64(i+1)/1000)
		if err != nil {
			b.Fatal(err)
		}
		_, err = s.Finalize(snap.Generation, assignments)
		if err != nil {
			b.Fatal(err)
		}
		links += snap.Stats.FDLinksRead
		reads += snap.Stats.FileReads
	}
	b.ReportMetric(float64(links)/float64(b.N), "readlink/op")
	b.ReportMetric(float64(reads)/float64(b.N), "proc-reads/op")
}

func TestPIDReuseBetweenStatAndStatus(t *testing.T) {
	f := newFixture(t)
	f.proc(t, 100, 0, 10, 100, 0)
	s := f.scanner(t, false, false)
	status := filepath.Join(f.root, "100/status")
	data, err := os.ReadFile(status)
	require.NoError(t, err)
	require.NoError(t, os.Remove(status))
	require.NoError(t, syscall.Mkfifo(status, 0600))
	type result struct {
		snap model.Snapshot
		err  error
	}
	done := make(chan result, 1)
	go func() { snap, err := s.scan(context.Background(), 1); done <- result{snap, err} }()
	// The writer opens only after the scanner has read stat and opened status.
	writer, err := os.OpenFile(status, os.O_WRONLY, 0600)
	require.NoError(t, err)
	f.stat(t, 100, 0, 20, 500, 0, "replacement")
	_, err = writer.Write(data)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	select {
	case got := <-done:
		require.NoError(t, got.err)
		assert.Empty(t, got.snap.Processes)
	case <-time.After(5 * time.Second):
		t.Fatal("native scan did not complete")
	}
	require.NoError(t, os.Remove(status))
	f.write(t, "100/status", string(data))
	next := f.scan(t, s)
	require.Len(t, next.Processes, 1)
	assert.Equal(t, uint64(20), next.Processes[0].Key.StartTime)
	assert.False(t, next.Processes[0].Has(model.CPUUser))
}

func TestSignedSchedulingFields(t *testing.T) {
	f := newFixture(t)
	f.proc(t, 100, 0, 10, 100, 0)
	data, err := os.ReadFile(filepath.Join(f.root, "100/stat"))
	require.NoError(t, err)
	text := string(data)
	end := strings.LastIndex(text, ")")
	fields := strings.Fields(text[end+2:])
	for _, field := range []int{7, 8, 18, 19} {
		fields[field-3] = "-20"
	}
	f.write(t, "100/stat", text[:end+2]+strings.Join(fields, " ")+"\n")
	s := f.scanner(t, false, false)
	snap := f.scan(t, s)
	require.Len(t, snap.Processes, 1)
	assert.Equal(t, uint64(10), snap.Processes[0].Key.StartTime)
}

func TestScanExportsCPUWithoutHostNormalization(t *testing.T) {
	f := newFixture(t)
	f.proc(t, 100, 0, 10, 100, 0)
	s := f.scanner(t, false, false)
	finish(t, s, f.scan(t, s))
	f.proc(t, 100, 0, 10, 200, 100)
	reference := f.scan(t, s)
	require.Len(t, reference.Processes, 1)
	own := reference.Processes[0].Values[model.CPUUser]
	children := reference.Processes[0].Values[model.CPUChildrenUser]
	require.Positive(t, own)
	require.Positive(t, children)
	finish(t, s, reference)

	for _, hostTicks := range []uint64{100, 0} {
		f.clock++
		f.system += hostTicks
		f.write(t, "stat", fmt.Sprintf("cpu %d 0 %d 0 0 0 0 0 0 0\ncpu0 0\n", f.system, f.system))
		f.proc(t, 100, 0, 10, uint64(f.clock)*100, uint64(f.clock-1)*100)
		snap, err := s.scan(context.Background(), f.clock)
		require.NoError(t, err)
		require.Len(t, snap.Processes, 1)
		require.True(t, snap.SystemCPUValid)
		require.Less(t, snap.SystemCPU[0], own+children)
		assert.Equal(t, own, snap.Processes[0].Values[model.CPUUser], "host ticks %d", hostTicks)
		assert.Equal(t, children, snap.Processes[0].Values[model.CPUChildrenUser], "host ticks %d", hostTicks)
		finish(t, s, snap)
	}
}
