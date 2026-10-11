// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/stretchr/testify/require"
)

func testStore(t *testing.T) (*Store, *time.Time) {
	t.Helper()
	s, err := Open(DefaultConfig(filepath.Join(t.TempDir(), "store")))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	return s, &now
}

func importFixture(t *testing.T, s *Store, label string) Manifest {
	t.Helper()
	m, archive := bundleFixture(t, label)
	file := filepath.Join(t.TempDir(), "archive.tar.gz")
	require.NoError(t, os.WriteFile(file, archive, 0600))
	require.NoError(t, s.Import(context.Background(), m, file))
	return m
}

func TestStoreGraceDemandAndRestart(t *testing.T) {
	s, now := testStore(t)
	first := importFixture(t, s, "first")
	*now = now.Add(time.Hour)
	second := importFixture(t, s, "second")
	claim, err := s.Demand(second.Archive.SHA256)
	require.NoError(t, err)
	*now = now.Add(8 * 24 * time.Hour)
	require.NoError(t, s.Sweep())
	_, err = s.BeginUse(first)
	require.ErrorIs(t, err, ErrUnavailable)
	use, err := s.BeginUse(second)
	require.NoError(t, err)
	require.NoError(t, use.Finish(true))
	require.NoError(t, claim.Close())
	preparedPath := filepath.Join(s.path, "generations", second.Archive.SHA256, "prepared.json")
	before, err := os.ReadFile(preparedPath)
	require.NoError(t, err)
	*now = now.Add(20 * 24 * time.Hour)
	other, err := Open(s.config)
	require.NoError(t, err)
	defer other.Close()
	other.now = s.now
	_, err = other.Inspect()
	require.NoError(t, err)
	after, err := os.ReadFile(preparedPath)
	require.NoError(t, err)
	require.Equal(t, before, after, "status/restart must not reset idle deadline")
	require.NoError(t, other.Sweep())
	*now = now.Add(11 * 24 * time.Hour)
	require.NoError(t, other.Sweep())
	_, err = s.BeginUse(second)
	require.ErrorIs(t, err, ErrUnavailable)
}

func TestStorePressureNeverEvictsDemandOrUncertainUse(t *testing.T) {
	s, now := testStore(t)
	s.config.MaxBytes = 128 << 10
	first := importFixture(t, s, strings.Repeat("A", 80<<10))
	claim, err := s.Demand(first.Archive.SHA256)
	require.NoError(t, err)
	second, archive := bundleFixture(t, strings.Repeat("B", 80<<10))
	file := filepath.Join(t.TempDir(), "archive")
	require.NoError(t, os.WriteFile(file, archive, 0600))
	require.ErrorIs(t, s.Import(context.Background(), second, file), ErrAllowance)
	use, err := s.BeginUse(first)
	require.NoError(t, err)
	require.NoError(t, use.Finish(false)) // no launch proof is supplied
	require.NoError(t, claim.Close())
	*now = now.Add(365 * 24 * time.Hour)
	require.NoError(t, s.Sweep())
	require.ErrorIs(t, s.Import(context.Background(), second, file), ErrAllowance)
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Equal(t, 1, status.UncertainPins)
	require.FileExists(t, filepath.Join(s.path, "generations", first.Archive.SHA256, "prepared.json"))
}

func TestStorePressureReclaimsSoftGrace(t *testing.T) {
	s, _ := testStore(t)
	s.config.MaxBytes = 128 << 10
	first := importFixture(t, s, strings.Repeat("A", 80<<10))
	second := importFixture(t, s, strings.Repeat("B", 80<<10))
	_, err := s.BeginUse(first)
	require.ErrorIs(t, err, ErrUnavailable)
	use, err := s.BeginUse(second)
	require.NoError(t, err)
	require.NoError(t, use.Finish(true))
	status, err := s.Inspect()
	require.NoError(t, err)
	require.LessOrEqual(t, status.UsedBytes+status.ReservedBytes, s.config.MaxBytes)
}

func TestFailedImportAndReadOnlyInspection(t *testing.T) {
	s, _ := testStore(t)
	m, archive := bundleFixture(t, "broken")
	file := filepath.Join(t.TempDir(), "truncated")
	require.NoError(t, os.WriteFile(file, archive[:len(archive)-1], 0600))
	require.Error(t, s.Import(context.Background(), m, file))
	_, err := s.BeginUse(m)
	require.ErrorIs(t, err, ErrUnavailable)
	require.NoError(t, s.Sweep())
	for _, parent := range []string{"uses", "claims", "staging", "trash", "initializing", "generations"} {
		entries, err := os.ReadDir(filepath.Join(s.path, parent))
		require.NoError(t, err)
		require.Empty(t, entries)
	}
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Zero(t, status.UsedBytes)
}

func TestPersistentSupervisorCompletionReleasesPin(t *testing.T) {
	helper := os.Getenv("DEM_RUNTIME_TEST_HELPER")
	if helper == "" {
		t.Skip("real-helper integration requires DEM_RUNTIME_TEST_HELPER")
	}
	t.Cleanup(ndexec.SetRunnerPathsForTests(helper, ""))
	s, now := testStore(t)
	m := importFixture(t, s, "proof")
	use, err := s.BeginUse(m)
	require.NoError(t, err)
	process, err := ndexec.StartUnprivilegedProcessTree(context.Background(), ndexec.TreeOptions{Completion: use.Completion}, "/bin/true")
	require.NoError(t, err)
	require.True(t, process.Wait().Drained)
	// Ignore live Go proof: the next store must recover the helper's receipt.
	require.NoError(t, use.Finish(false))
	*now = now.Add(31 * 24 * time.Hour)
	require.NoError(t, s.Sweep())
	_, err = s.BeginUse(m)
	require.ErrorIs(t, err, ErrUnavailable)
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Zero(t, status.ExecutionPins)
}

func TestCallerFinishesAfterReceiptReconciliation(t *testing.T) {
	helper := os.Getenv("DEM_RUNTIME_TEST_HELPER")
	if helper == "" {
		t.Skip("real-helper integration requires DEM_RUNTIME_TEST_HELPER")
	}
	t.Cleanup(ndexec.SetRunnerPathsForTests(helper, ""))
	s, _ := testStore(t)
	m := importFixture(t, s, "receipt-race")
	use, err := s.BeginUse(m)
	require.NoError(t, err)
	process, err := ndexec.StartUnprivilegedProcessTree(context.Background(), ndexec.TreeOptions{Completion: use.Completion}, "/bin/true")
	require.NoError(t, err)
	require.True(t, process.Wait().Drained)
	require.NoError(t, s.Sweep())
	require.NoError(t, use.Finish(true), "a receipt may be reconciled before its caller finishes")
	require.Empty(t, s.completedUses)
}

func TestReservationReusesPublishedGeneration(t *testing.T) {
	s, _ := testStore(t)
	m := importFixture(t, s, "published")
	name, owner, err := s.reserve(m)
	require.NoError(t, err)
	if owner != nil {
		defer owner.Close()
	}
	require.Empty(t, name, "publication must be checked in the same transaction as reservation")
	require.Nil(t, owner)
}

func TestConcurrentPreparationAccountingAndReuse(t *testing.T) {
	s, _ := testStore(t)
	other, err := Open(s.config)
	require.NoError(t, err)
	defer other.Close()
	payload := make([]byte, 64<<10)
	_, err = rand.Read(payload)
	require.NoError(t, err)
	m, archive := bundleFixture(t, string(payload))
	started, resume := make(chan struct{}), make(chan struct{})
	result := make(chan error, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		result <- s.prepare(context.Background(), m, func() (io.ReadCloser, error) {
			return io.NopCloser(io.MultiReader(bytes.NewReader(archive[:len(archive)/2]), &resumeReader{ready: started, resume: resume, tail: bytes.NewReader(archive[len(archive)/2:])})), nil
		})
	}()
	t.Cleanup(func() {
		select {
		case <-resume:
		default:
			close(resume)
		}
		<-finished
	})
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("preparation did not reach archive midpoint")
	}
	status, err := other.Inspect()
	require.NoError(t, err)
	needed, err := s.preparationBytes(m)
	require.NoError(t, err)
	require.Positive(t, status.PreparingBytes)
	require.GreaterOrEqual(t, status.UsedBytes+status.ReservedBytes, needed)
	require.Equal(t, status.UsedBytes, status.ProtectedBytes)
	require.Zero(t, status.ReclaimableBytes)
	require.ErrorIs(t, other.Import(context.Background(), m, "/must-not-open"), ErrPreparing)
	require.NoError(t, other.Sweep())
	close(resume)
	require.NoError(t, <-result)
	// A successor reuses the published result without touching its input.
	require.NoError(t, other.Import(context.Background(), m, "/must-not-open"))
	status, err = other.Inspect()
	require.NoError(t, err)
	require.Zero(t, status.ReservedBytes)
	require.Zero(t, status.ProtectedBytes)
	require.Equal(t, status.UsedBytes, status.ReclaimableBytes)
}

type resumeReader struct {
	ready   chan struct{}
	resume  <-chan struct{}
	tail    io.Reader
	resumed bool
}

func (r *resumeReader) Read(p []byte) (int, error) {
	if !r.resumed {
		close(r.ready)
		<-r.resume
		r.resumed = true
	}
	return r.tail.Read(p)
}

func TestAllowanceExplainsProtectedGenerationBytes(t *testing.T) {
	s, _ := testStore(t)
	s.config.MaxBytes = 128 << 10
	m := importFixture(t, s, strings.Repeat("a", 80<<10))
	claim, err := s.Demand(m.Archive.SHA256)
	require.NoError(t, err)
	defer claim.Close()
	use, err := s.BeginUse(m)
	require.NoError(t, err)
	require.NoError(t, use.Finish(false))
	next, _ := bundleFixture(t, strings.Repeat("b", 80<<10))
	err = s.Import(context.Background(), next, "/must-not-open")
	var allowance *AllowanceError
	require.True(t, errors.As(err, &allowance))
	require.Positive(t, allowance.NeededBytes)
	require.GreaterOrEqual(t, allowance.Status.DemandBytes, m.Archive.ExpandedBytes)
	require.Equal(t, allowance.Status.DemandBytes, allowance.Status.ExecutionBytes)
	require.Equal(t, allowance.Status.ExecutionBytes, allowance.Status.UncertainBytes)
	require.Equal(t, allowance.Status.UsedBytes, allowance.Status.ProtectedBytes)
	require.Zero(t, allowance.Status.ReclaimableBytes)
}

// Set the manifest/archive environment only in bundle qualification. This uses
// the same Store and supervisor API as the service, not an alternate extractor.
func TestRealBundlePreparation(t *testing.T) {
	manifestPath := os.Getenv("DEM_RUNTIME_TEST_MANIFEST")
	if manifestPath == "" {
		t.Skip("real bundle qualification requires a manifest/archive")
	}
	helper := os.Getenv("DEM_RUNTIME_TEST_HELPER")
	require.NotEmpty(t, helper)
	t.Cleanup(ndexec.SetRunnerPathsForTests(helper, ""))
	data, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	var m Manifest
	require.NoError(t, json.Unmarshal(data, &m))
	s, _ := testStore(t)
	start := time.Now()
	require.NoError(t, s.Import(context.Background(), m, os.Getenv("DEM_RUNTIME_TEST_ARCHIVE")))
	t.Logf("prepared archive_bytes=%d expanded_bytes=%d entries=%d duration=%s", m.Archive.Bytes, m.Archive.ExpandedBytes, m.Archive.Entries, time.Since(start))
	for i := 0; i < 6; i++ {
		start = time.Now()
		status, err := s.Inspect()
		require.NoError(t, err)
		t.Logf("inspect duration=%s used_bytes=%d", time.Since(start), status.UsedBytes)
	}
	for _, command := range []string{"node", "browser", "reexec"} {
		use, err := s.BeginUse(m)
		require.NoError(t, err)
		program, args := use.Paths.Node, []string{"--version"}
		if command == "browser" {
			program = use.Paths.Browser
		}
		if command == "reexec" {
			args = []string{"-e", "require('node:child_process').execFileSync(process.execPath,['--version'],{stdio:'inherit'})"}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		process, err := ndexec.StartUnprivilegedProcessTree(ctx, ndexec.TreeOptions{Completion: use.Completion, Stdout: os.Stdout, Stderr: os.Stderr}, program, args...)
		require.NoError(t, err)
		result := process.Wait()
		cancel()
		require.True(t, result.Drained)
		require.NoError(t, use.Finish(result.Drained))
		require.NoError(t, result.Err)
	}
}

// The child exits without Go cleanup to exercise process-lifetime ownership.
func TestRuntimeOwnerProcess(t *testing.T) {
	mode := os.Getenv("DEM_RUNTIME_OWNER_MODE")
	if mode == "" {
		return
	}
	s, err := Open(DefaultConfig(os.Getenv("DEM_RUNTIME_OWNER_STORE")))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(os.Getenv("DEM_RUNTIME_OWNER_MANIFEST"))
	if err != nil {
		t.Fatal(err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	ready := func() { fmt.Println("ready"); _, _ = io.Copy(io.Discard, os.Stdin); os.Exit(0) }
	if mode == "claim" {
		if _, err := s.Demand(m.Archive.SHA256); err != nil {
			t.Fatal(err)
		}
		ready()
	} else if mode == "prepare" || mode == "partial" {
		err := s.prepare(context.Background(), m, func() (io.ReadCloser, error) {
			if mode == "prepare" {
				ready()
			}
			fixture, archive := bundleFixture(t, "incoming")
			require.Equal(t, m.Archive.SHA256, fixture.Archive.SHA256)
			return io.NopCloser(io.MultiReader(bytes.NewReader(archive[:len(archive)/2]), readFunc(func([]byte) (int, error) { ready(); return 0, io.EOF }))), nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

type readFunc func([]byte) (int, error)

func (f readFunc) Read(p []byte) (int, error) { return f(p) }

func startRuntimeOwner(t *testing.T, s *Store, m Manifest, mode string) func() {
	t.Helper()
	file := filepath.Join(t.TempDir(), "manifest.json")
	data, err := json.Marshal(m)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(file, data, 0600))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRuntimeOwnerProcess$")
	cmd.Env = append(os.Environ(), "DEM_RUNTIME_OWNER_MODE="+mode, "DEM_RUNTIME_OWNER_STORE="+s.path, "DEM_RUNTIME_OWNER_MANIFEST="+file)
	input, err := cmd.StdinPipe()
	require.NoError(t, err)
	output, err := cmd.StdoutPipe()
	require.NoError(t, err)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	require.NoError(t, cmd.Start())
	stopped := false
	stop := func() {
		if stopped {
			return
		}
		stopped = true
		input.Close()
		err := cmd.Wait()
		cancel()
		require.NoError(t, err, "%s", stderr.String())
	}
	t.Cleanup(stop)
	line, err := bufio.NewReader(output).ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "ready\n", line)
	return stop
}

func TestAbandonedPreparationAndProcessDemand(t *testing.T) {
	s, now := testStore(t)
	first := importFixture(t, s, "existing")
	stop := startRuntimeOwner(t, s, first, "claim")
	*now = now.Add(100 * 24 * time.Hour)
	require.NoError(t, s.Sweep())
	require.FileExists(t, filepath.Join(s.path, "generations", first.Archive.SHA256, "prepared.json"))
	stop()
	require.NoError(t, s.Sweep())
	*now = now.Add(31 * 24 * time.Hour)
	require.NoError(t, s.Sweep())
	_, err := s.BeginUse(first)
	require.ErrorIs(t, err, ErrUnavailable)
	second, _ := bundleFixture(t, "incoming")
	stop = startRuntimeOwner(t, s, second, "partial")
	partials, err := filepath.Glob(filepath.Join(s.path, "staging", "*", "archive.tar.gz"))
	require.NoError(t, err)
	require.Len(t, partials, 1)
	partial, err := os.Stat(partials[0])
	require.NoError(t, err)
	require.Positive(t, partial.Size(), "owner must exit after actual archive writes")
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Positive(t, status.ReservedBytes)
	require.NoError(t, s.Sweep())
	status, err = s.Inspect()
	require.NoError(t, err)
	require.Positive(t, status.ReservedBytes)
	stop()
	require.NoError(t, s.Sweep())
	status, err = s.Inspect()
	require.NoError(t, err)
	require.Zero(t, status.ReservedBytes)
	staging, err := os.ReadDir(filepath.Join(s.path, "staging"))
	require.NoError(t, err)
	require.Empty(t, staging)
}

func TestKernelBootProvenance(t *testing.T) {
	for name, fixture := range map[string]struct {
		mount string
		valid bool
	}{
		"normal":           {"20 1 0:1 / /proc rw - proc proc rw", true},
		"readonly-sys":     {"20 1 0:1 /sys /proc/sys ro - proc proc rw", true},
		"container-file":   {"20 1 0:2 /boot-id /proc/sys/kernel/random/boot_id ro - tmpfs tmpfs rw", false},
		"random-uuid-bind": {"20 1 0:1 /sys/kernel/random/uuid /proc/sys/kernel/random/boot_id ro - proc proc rw", false},
		"wrong-id":         {"21 1 0:1 / /proc rw - proc proc rw", false},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, fixture.valid, kernelBootMount(strings.NewReader(fixture.mount+"\n"), 20, "/proc/sys/kernel/random/boot_id"))
		})
	}
	actual := hostBoot()
	if actual.Verified {
		require.True(t, validBootID(actual.ID))
	}
	s, now := testStore(t)
	m := importFixture(t, s, "boot")
	s.boot = bootIdentity{ID: "11111111-1111-1111-1111-111111111111", Verified: true}
	use, err := s.BeginUse(m)
	require.NoError(t, err)
	require.NoError(t, use.Finish(false))
	*now = now.Add(100 * 24 * time.Hour)
	s.boot = bootIdentity{ID: "22222222-2222-2222-2222-222222222222", Verified: false}
	require.NoError(t, s.Sweep())
	require.FileExists(t, filepath.Join(s.path, "generations", m.Archive.SHA256, "prepared.json"))
	s.boot.Verified = true
	require.NoError(t, s.Sweep())
	_, err = s.BeginUse(m)
	require.ErrorIs(t, err, ErrUnavailable)
}

// Includes durable use creation, actual supervisor launch/drain/join and pin
// retirement. Local filesystem trend only; bulk preparation/GC is separate.
func BenchmarkRuntimeUse(b *testing.B) {
	helper := os.Getenv("DEM_RUNTIME_TEST_HELPER")
	if helper == "" {
		b.Skip("real-helper benchmark requires DEM_RUNTIME_TEST_HELPER")
	}
	b.Cleanup(ndexec.SetRunnerPathsForTests(helper, ""))
	s, err := Open(DefaultConfig(filepath.Join(b.TempDir(), "store")))
	require.NoError(b, err)
	defer s.Close()
	m, archive := bundleFixture(b, "benchmark")
	file := filepath.Join(b.TempDir(), "archive")
	require.NoError(b, os.WriteFile(file, archive, 0600))
	require.NoError(b, s.Import(context.Background(), m, file))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		use, err := s.BeginUse(m)
		require.NoError(b, err)
		process, err := ndexec.StartUnprivilegedProcessTree(context.Background(), ndexec.TreeOptions{Completion: use.Completion}, "/bin/true")
		require.NoError(b, err)
		result := process.Wait()
		require.True(b, result.Drained)
		require.NoError(b, result.Err)
		require.NoError(b, use.Finish(true))
	}
	b.StopTimer()
	require.NoError(b, s.Sweep())
}
