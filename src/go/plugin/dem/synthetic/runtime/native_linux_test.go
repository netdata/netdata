// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/stretchr/testify/require"
)

type nativeFixture struct {
	config   NativeConfig
	control  *NativeControl
	manifest Manifest
	uid, gid int
}

func newNativeFixture(t *testing.T) *nativeFixture {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("root/service UID boundary test requires an isolated root container")
	}
	account, err := user.Lookup("netdata")
	if err != nil {
		t.Skip("root/service UID test requires the netdata fixture account")
	}
	uid, err := strconv.Atoi(account.Uid)
	require.NoError(t, err)
	gid, err := strconv.Atoi(account.Gid)
	require.NoError(t, err)
	path, err := os.MkdirTemp("/", "dem-native-test-")
	require.NoError(t, err)
	require.NoError(t, os.Chmod(path, 0755))
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(path)) })
	fixture := &nativeFixture{config: NativeConfig{ControlPath: filepath.Join(path, "control"), PayloadPath: filepath.Join(path, "payload")}, uid: uid, gid: gid}
	fixture.control, err = InitializeNativeControl(context.Background(), fixture.config, uid, gid, func(context.Context) error {
		_, err := fixture.command(t, "initialize", nil).CombinedOutput()
		return err
	})
	require.NoError(t, err)
	fixture.manifest = fixture.installPayload(t, "first")
	return fixture
}

func (f *nativeFixture) installPayload(t *testing.T, label string) Manifest {
	t.Helper()
	require.NoError(t, os.Mkdir(f.config.PayloadPath, 0755))
	root, err := os.OpenRoot(f.config.PayloadPath)
	require.NoError(t, err)
	defer root.Close()
	manifest, archive := bundleFixture(t, label)
	require.NoError(t, extractArchive(bytes.NewReader(archive), root, manifest.Archive))
	require.NoError(t, bindPrefix(root, f.config.PayloadPath, manifest))
	require.NoError(t, fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		mode := os.FileMode(0644)
		if entry.IsDir() || info.Mode()&0111 != 0 {
			mode = 0755
		}
		return root.Chmod(name, mode)
	}))
	data, err := json.Marshal(NativePackage{Format: 1, Path: f.config.PayloadPath, Manifest: manifest})
	require.NoError(t, err)
	require.NoError(t, writeNewFile(root, "native.json", data, 0644))
	return manifest
}

func (f *nativeFixture) command(t *testing.T, action string, target *Manifest) *exec.Cmd {
	t.Helper()
	executable, err := os.Executable()
	require.NoError(t, err)
	config, err := json.Marshal(f.config)
	require.NoError(t, err)
	manifest, err := json.Marshal(target)
	require.NoError(t, err)
	command := exec.Command(executable, "-test.run=^TestNativeServiceProcess$")
	command.Env = append(os.Environ(), "DEM_NATIVE_ACTION="+action, "DEM_NATIVE_CONFIG="+string(config), "DEM_NATIVE_TARGET="+string(manifest))
	command.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: uint32(f.uid), Gid: uint32(f.gid)}}
	return command
}

func (f *nativeFixture) inspect(t *testing.T) NativeInspector {
	return func(context.Context) (NativeStatus, error) {
		data, err := f.command(t, "inspect", nil).CombinedOutput()
		if err != nil {
			return NativeStatus{}, fmt.Errorf("service inspector: %w: %s", err, data)
		}
		var status NativeStatus
		err = json.Unmarshal(data, &status)
		return status, err
	}
}

func (f *nativeFixture) qualify(t *testing.T, action string) NativeQualifier {
	return func(_ context.Context, target Manifest) error {
		data, err := f.command(t, action, &target).CombinedOutput()
		if err != nil {
			return fmt.Errorf("service qualifier: %w: %s", err, data)
		}
		return nil
	}
}

func TestNativeServiceProcess(t *testing.T) {
	action := os.Getenv("DEM_NATIVE_ACTION")
	if action == "" {
		return
	}
	err := nativeServiceProcess(action)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func nativeServiceProcess(action string) error {
	var config NativeConfig
	if err := json.Unmarshal([]byte(os.Getenv("DEM_NATIVE_CONFIG")), &config); err != nil {
		return err
	}
	if action == "initialize" {
		return InitializeNativeProof(config)
	}
	native, err := OpenNative(config)
	if err != nil {
		return err
	}
	defer native.Close()
	if action == "inspect" {
		if err := native.Sweep(); err != nil {
			return err
		}
		status, err := native.Inspect()
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(status)
	}
	if action == "expect-closed" {
		fmt.Fprintln(os.Stdout, "started")
		consumer, err := native.AcquireSelected()
		if consumer != nil {
			consumer.Close()
		}
		if !errors.Is(err, ErrNativeClosed) {
			return fmt.Errorf("expected closed native admission, got %v", err)
		}
		return nil
	}
	var consumer *NativeConsumer
	if action == "qualify" || action == "qualify-fail" || action == "hold-candidate" {
		var target Manifest
		if err := json.Unmarshal([]byte(os.Getenv("DEM_NATIVE_TARGET")), &target); err != nil {
			return err
		}
		consumer, err = native.AcquireCandidate(target)
	} else {
		consumer, err = native.AcquireSelected()
	}
	if err != nil {
		return err
	}
	defer consumer.Close()
	if action == "hold" || action == "hold-candidate" {
		fmt.Fprintln(os.Stdout, "ready")
		_, err := bufio.NewReader(os.Stdin).ReadByte()
		return err
	}
	for i := 0; i < 3; i++ {
		use, err := consumer.BeginUse()
		if err != nil {
			return err
		}
		if action == "pending" {
			return use.Finish(false)
		}
		if helper := os.Getenv("DEM_RUNTIME_TEST_HELPER"); helper != "" {
			restore := ndexec.SetRunnerPathsForTests(helper, "")
			process, err := ndexec.StartUnprivilegedProcessTree(context.Background(), ndexec.TreeOptions{Completion: use.Completion}, "/bin/true")
			if err != nil {
				restore()
				return errors.Join(err, use.Finish(true))
			}
			drained := process.Wait().Drained
			restore()
			if err := use.Finish(drained); err != nil {
				return err
			}
			if !drained {
				return errors.New("fixture process did not drain")
			}
		} else if err := use.Finish(true); err != nil {
			return err
		}
		if action == "qualify-fail" {
			return errors.New("qualification fixture failed")
		}
	}
	return nil
}

func (f *nativeFixture) hold(t *testing.T, action string, target *Manifest) func() {
	t.Helper()
	command := f.command(t, action, target)
	input, err := command.StdinPipe()
	require.NoError(t, err)
	output, err := command.StdoutPipe()
	require.NoError(t, err)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	require.NoError(t, command.Start())
	line, err := bufio.NewReader(output).ReadString('\n')
	require.NoError(t, err, "%s", stderr.String())
	require.Equal(t, "ready\n", line, "%s", stderr.String())
	var once sync.Once
	stop := func() {
		once.Do(func() {
			_, err := io.WriteString(input, "x")
			require.NoError(t, err)
			require.NoError(t, input.Close())
			require.NoError(t, command.Wait(), "%s", stderr.String())
		})
	}
	t.Cleanup(stop)
	return stop
}

func TestNativeAdmissionLifecycle(t *testing.T) {
	f := newNativeFixture(t)
	ctx := context.Background()
	data, err := f.command(t, "expect-closed", nil).CombinedOutput()
	require.NoError(t, err, "%s", data)
	calls := 0
	require.NoError(t, f.control.QualifyAndReady(ctx, f.inspect(t), func(ctx context.Context, target Manifest) error {
		calls++
		data, err := f.command(t, "expect-closed", nil).CombinedOutput()
		if err != nil {
			return fmt.Errorf("ordinary admission during qualification: %w: %s", err, data)
		}
		stop := f.hold(t, "hold-candidate", &target)
		require.ErrorIs(t, f.control.CloseAdmission(ctx, f.inspect(t)), ErrInUse)
		stop()
		return f.qualify(t, "qualify")(ctx, target)
	}))
	require.Equal(t, 1, calls)
	stop := f.hold(t, "hold", nil)
	require.ErrorIs(t, f.control.CloseAdmission(ctx, f.inspect(t)), ErrInUse)
	state, err := readNativeControl(f.config)
	require.NoError(t, err)
	require.Equal(t, "ready", state.Phase)
	stop()
	require.NoError(t, f.control.CloseAdmission(ctx, f.inspect(t)))
	state, err = readNativeControl(f.config)
	require.NoError(t, err)
	require.Equal(t, "closed", state.Phase)
	require.Nil(t, state.Target)
	require.NoError(t, f.control.QualifyAndReady(ctx, f.inspect(t), f.qualify(t, "qualify")))
}

func TestNativeFailedQualificationCannotOpenAdmission(t *testing.T) {
	f := newNativeFixture(t)
	ctx := context.Background()
	require.Error(t, f.control.QualifyAndReady(ctx, f.inspect(t), f.qualify(t, "qualify-fail")))
	data, err := f.command(t, "expect-closed", nil).CombinedOutput()
	require.NoError(t, err, "%s", data)
	status, err := f.inspect(t)(ctx)
	require.NoError(t, err)
	require.NoError(t, status.empty())
	state, err := readNativeControl(f.config)
	require.NoError(t, err)
	require.Equal(t, "qualifying", state.Phase, "empty proof does not establish qualification success")
	require.NoError(t, f.control.QualifyAndReady(ctx, f.inspect(t), f.qualify(t, "qualify")))
}

func TestNativePendingExecutionPreventsMutation(t *testing.T) {
	f := newNativeFixture(t)
	ctx := context.Background()
	require.NoError(t, f.control.QualifyAndReady(ctx, f.inspect(t), f.qualify(t, "qualify")))
	data, err := f.command(t, "pending", nil).CombinedOutput()
	require.NoError(t, err, "%s", data)
	status, err := f.inspect(t)(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, status.ExecutionPins)
	require.Equal(t, 1, status.UncertainPins)
	require.ErrorIs(t, f.control.CloseAdmission(ctx, f.inspect(t)), ErrInUse)
}

func TestNativeGateSerializesNewAdmission(t *testing.T) {
	f := newNativeFixture(t)
	ctx := context.Background()
	require.NoError(t, f.control.QualifyAndReady(ctx, f.inspect(t), f.qualify(t, "qualify")))
	command := f.command(t, "expect-closed", nil)
	output, err := command.StdoutPipe()
	require.NoError(t, err)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	require.NoError(t, f.control.CloseAdmission(ctx, func(ctx context.Context) (NativeStatus, error) {
		require.NoError(t, command.Start())
		line, err := bufio.NewReader(output).ReadString('\n')
		require.NoError(t, err)
		require.Equal(t, "started\n", line)
		return f.inspect(t)(ctx)
	}))
	require.NoError(t, command.Wait(), "%s", stderr.String())
}

func TestNativeReadyStateNeverReinitializesMissingProof(t *testing.T) {
	f := newNativeFixture(t)
	ctx := context.Background()
	proof := filepath.Join(f.config.ControlPath, "owners", "uses")
	require.NoError(t, os.Rename(proof, proof+".saved"))
	data, err := f.command(t, "inspect", nil).CombinedOutput()
	require.Error(t, err, "%s", data)
	_, err = os.Lstat(proof)
	require.ErrorIs(t, err, os.ErrNotExist)
	called := false
	_, err = InitializeNativeControl(ctx, f.config, f.uid, f.gid, func(context.Context) error { called = true; return nil })
	require.NoError(t, err)
	require.False(t, called, "established state must not invoke bootstrap recovery")
	require.Error(t, f.control.CloseAdmission(ctx, f.inspect(t)))
}

func TestNativeRejectsWritableOrMisboundPayload(t *testing.T) {
	for _, damage := range []string{"writable-file", "writable-ancestor", "foreign-owner", "misbound", "external-link"} {
		t.Run(damage, func(t *testing.T) {
			f := newNativeFixture(t)
			switch damage {
			case "writable-file":
				require.NoError(t, os.Chmod(filepath.Join(f.config.PayloadPath, f.manifest.Layout.EntryPoint), 0666))
			case "writable-ancestor":
				require.NoError(t, os.Chmod(filepath.Dir(f.config.PayloadPath), 0777))
			case "foreign-owner":
				require.NoError(t, os.Chown(filepath.Join(f.config.PayloadPath, f.manifest.Layout.EntryPoint), f.uid, f.gid))
			case "misbound":
				require.NoError(t, os.WriteFile(filepath.Join(f.config.PayloadPath, "chrome"), []byte("#!/bin/sh\nexit 0\n"), 0755))
			case "external-link":
				require.NoError(t, os.Symlink("/bin/true", filepath.Join(f.config.PayloadPath, "outside")))
			}
			called := false
			err := f.control.QualifyAndReady(context.Background(), f.inspect(t), func(context.Context, Manifest) error { called = true; return nil })
			require.Error(t, err)
			require.False(t, called)
		})
	}
}

func TestNativeQualificationRequiresReleasedCandidate(t *testing.T) {
	f := newNativeFixture(t)
	ctx := context.Background()
	var stop func()
	err := f.control.QualifyAndReady(ctx, f.inspect(t), func(_ context.Context, target Manifest) error {
		stop = f.hold(t, "hold-candidate", &target)
		return nil
	})
	require.ErrorIs(t, err, ErrInUse)
	stop()
	status, err := f.inspect(t)(ctx)
	require.NoError(t, err)
	require.NoError(t, status.empty())
	state, err := readNativeControl(f.config)
	require.NoError(t, err)
	require.Equal(t, "qualifying", state.Phase)
	require.NoError(t, f.control.QualifyAndReady(ctx, f.inspect(t), f.qualify(t, "qualify")))
}

func TestNativeClosedInspectionDoesNotNeedPayload(t *testing.T) {
	f := newNativeFixture(t)
	require.NoError(t, os.Rename(f.config.PayloadPath, f.config.PayloadPath+".removed"))
	status, err := f.inspect(t)(context.Background())
	require.NoError(t, err)
	require.NoError(t, status.empty())
	called := false
	err = f.control.QualifyAndReady(context.Background(), f.inspect(t), func(context.Context, Manifest) error { called = true; return nil })
	require.Error(t, err)
	require.False(t, called)
	data, err := f.command(t, "expect-closed", nil).CombinedOutput()
	require.NoError(t, err, "%s", data)
}

func TestNativeBootstrapInterruptionAndGateIdentity(t *testing.T) {
	f := newNativeFixture(t)
	f.config.ControlPath += "-interrupted"
	f.config.PayloadPath += "-interrupted"
	ctx := context.Background()
	_, err := InitializeNativeControl(ctx, f.config, f.uid, f.gid, func(context.Context) error { return errors.New("bootstrap fixture interrupted") })
	require.Error(t, err)
	state, err := readNativeControl(f.config)
	require.NoError(t, err)
	require.Equal(t, "initializing", state.Phase)
	gate := filepath.Join(f.config.ControlPath, nativeGateName)
	before, err := os.Stat(gate)
	require.NoError(t, err)
	_, err = InitializeNativeControl(ctx, f.config, f.uid, f.gid, func(context.Context) error {
		data, err := f.command(t, "initialize", nil).CombinedOutput()
		if err != nil {
			return fmt.Errorf("initialize service proof: %w: %s", err, data)
		}
		return nil
	})
	require.NoError(t, err)
	after, err := os.Stat(gate)
	require.NoError(t, err)
	require.True(t, os.SameFile(before, after))
	_, err = OpenNative(f.config)
	require.ErrorContains(t, err, "service UID/GID", "root cannot inspect receipts through the service API")
	require.NoError(t, os.Rename(gate, gate+".saved"))
	called := false
	_, err = InitializeNativeControl(ctx, f.config, f.uid, f.gid, func(context.Context) error { called = true; return nil })
	require.Error(t, err)
	require.False(t, called)
	_, err = os.Lstat(gate)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestNativeMissingControlBesidePayloadIsNotFirstInstall(t *testing.T) {
	f := newNativeFixture(t)
	require.NoError(t, os.Rename(f.config.ControlPath, f.config.ControlPath+".lost"))
	called := false
	_, err := InitializeNativeControl(context.Background(), f.config, f.uid, f.gid, func(context.Context) error { called = true; return nil })
	require.ErrorContains(t, err, "existing payload")
	require.False(t, called)
	_, err = os.Lstat(f.config.ControlPath)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestNativeMissingPackageIsUnavailable(t *testing.T) {
	path := t.TempDir()
	config := NativeConfig{ControlPath: filepath.Join(path, "control"), PayloadPath: filepath.Join(path, "payload")}
	native, err := OpenNative(config)
	require.ErrorIs(t, err, ErrUnavailable)
	require.Nil(t, native)
	for _, path := range []string{config.ControlPath, config.PayloadPath} {
		_, err := os.Lstat(path)
		require.ErrorIs(t, err, os.ErrNotExist)
	}
	// An existing payload with lost control is damaged state, not absence.
	require.NoError(t, os.Mkdir(config.PayloadPath, 0755))
	_, err = OpenNative(config)
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrUnavailable)
}
