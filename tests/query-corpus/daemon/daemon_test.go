// SPDX-License-Identifier: GPL-3.0-or-later

package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("QUERY_CORPUS_TEST_DAEMON_ENV") == "1" {
		info, statErr := os.Stat(os.Getenv("NETDATA_RUN_DIR"))
		ready := "false"
		if statErr == nil && info.IsDir() {
			ready = "true"
		}
		err := json.NewEncoder(os.Stdout).Encode(map[string]string{
			"run": os.Getenv("NETDATA_RUN_DIR"), "pipe": os.Getenv("NETDATA_PIPENAME"), "ready": ready,
		})
		if err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestDaemonRuntimeEnvironmentIsPrivate(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	parent := map[string]string{"run": "parent-runtime", "pipe": "parent-cli-socket"}
	t.Setenv("QUERY_CORPUS_TEST_DAEMON_ENV", "1")
	t.Setenv("NETDATA_RUN_DIR", parent["run"])
	t.Setenv("NETDATA_PIPENAME", parent["pipe"])
	seen := make(map[string]bool)
	for _, name := range []string{"first", "second"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, "log"), 0o755); err != nil {
				t.Fatal(err)
			}
			d := &Daemon{Opts: Options{Binary: executable, RunDir: root}, BaseURL: "invalid://environment-test"}
			// The helper records the actual child environment and exits before readiness.
			if err := d.launch(); err == nil || d.process != nil {
				t.Fatalf("environment helper did not exit and reap: %v", err)
			}
			output, err := os.ReadFile(filepath.Join(root, "log", "stdout.log"))
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]string
			if err := json.Unmarshal(output, &got); err != nil {
				t.Fatalf("environment helper output %q: %v", output, err)
			}
			run := got["run"]
			if !filepath.IsAbs(run) || run == parent["run"] || seen[run] {
				t.Fatalf("runtime path is not private to this launch: %v", got)
			}
			seen[run] = true
			want := map[string]string{"run": run, "pipe": filepath.Join(run, "netdata.pipe"), "ready": "true"}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("child environment = %v, want private paths %v", got, want)
			}
			if _, err := os.Stat(run); !os.IsNotExist(err) {
				t.Errorf("reaped helper retained its runtime directory: %v", err)
			}
		})
	}
	gotParent := map[string]string{"run": os.Getenv("NETDATA_RUN_DIR"), "pipe": os.Getenv("NETDATA_PIPENAME")}
	if !reflect.DeepEqual(gotParent, parent) {
		t.Errorf("launcher changed parent environment: %v", gotParent)
	}
}

func TestDaemonRejectsLongRuntimePathBeforeLaunch(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "log"), 0o700); err != nil {
		t.Fatal(err)
	}
	tmpDir := filepath.Join(t.TempDir(), strings.Repeat("x", len(syscall.RawSockaddrUnix{}.Path)))
	if err := os.Mkdir(tmpDir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", tmpDir)
	t.Setenv("QUERY_CORPUS_TEST_DAEMON_ENV", "1")
	d := &Daemon{Opts: Options{Binary: executable, RunDir: root}, BaseURL: "invalid://environment-test"}
	err = d.launch()
	if err == nil || !strings.Contains(err.Error(), "IPC socket path") || !strings.Contains(err.Error(), "TMPDIR") {
		t.Errorf("long runtime path did not produce an actionable error: %v", err)
	}
	if d.process != nil || d.processPID != 0 || d.waitCh != nil {
		t.Error("long runtime path reached child startup")
	}
	if _, err := os.Stat(filepath.Join(root, "log", "stdout.log")); !os.IsNotExist(err) {
		t.Errorf("long runtime path reached log creation: %v", err)
	}
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 || d.runtimeDir != "" {
		t.Errorf("rejected launch retained runtime state: entries=%v, directory=%q", entries, d.runtimeDir)
	}
}

func TestRuntimeSocketPathLimits(t *testing.T) {
	maxPathBytes := len(syscall.RawSockaddrUnix{}.Path) - 1
	maxRunBytes := maxPathBytes - len("/netdata-spawn-plugins.sock")
	for _, tc := range []struct {
		name, runDir, rejectedSocket string
	}{
		{"boundary", "/" + strings.Repeat("x", maxRunBytes-1), ""},
		{"spawn-only-overflow", "/" + strings.Repeat("x", maxRunBytes), "netdata-spawn-plugins.sock"},
		{"unclean-spawn-path", strings.Repeat("/", maxRunBytes) + "x", "netdata-spawn-plugins.sock"},
		{"cli-overflow", "/" + strings.Repeat("x", maxPathBytes-len("/netdata.pipe")), "netdata.pipe"},
		{"multibyte-boundary", "/" + strings.Repeat("é", (maxRunBytes-1)/2) + strings.Repeat("x", (maxRunBytes-1)%2), ""},
		{"multibyte-overflow", "/" + strings.Repeat("é", (maxRunBytes+1)/2), "netdata-spawn-plugins.sock"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateRuntimeSocketPaths(tc.runDir)
			if tc.rejectedSocket == "" {
				if err != nil {
					t.Fatalf("valid socket paths rejected: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.rejectedSocket) || !strings.Contains(err.Error(), "TMPDIR") {
				t.Fatalf("missing socket-specific path diagnostic: %v", err)
			}
		})
	}
}

func TestNetdataConfigDisablesUnlistedPlugins(t *testing.T) {
	const disabled = "\n    enable running new plugins = no\n"
	if !strings.Contains(netdataConfTemplate, disabled) {
		t.Fatal("generated [plugins] configuration allows unlisted installed collectors")
	}
}

func TestStorageOptionsAreStrictAndRenderExactly(t *testing.T) {
	for _, tc := range []struct {
		name string
		o    Options
	}{
		{name: "default"},
		{name: "gorilla-dbengine", o: Options{DBEnginePageType: "gorilla", StreamMemoryMode: "dbengine"}},
		{name: "raw-ram", o: Options{DBEnginePageType: "raw", StreamMemoryMode: "ram"}},
		{name: "raw-alloc", o: Options{DBEnginePageType: "raw", StreamMemoryMode: "alloc"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateOptions(tc.o); err != nil {
				t.Fatalf("valid options rejected: %v", err)
			}
		})
	}

	for _, tc := range []struct {
		name string
		o    Options
	}{
		{name: "unknown page type", o: Options{DBEnginePageType: "array"}},
		{name: "unknown memory mode", o: Options{StreamMemoryMode: "heap"}},
		{name: "case variant", o: Options{DBEnginePageType: "Raw"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateOptions(tc.o); err == nil {
				t.Fatal("invalid storage option accepted")
			}
		})
	}

	if got := streamMemoryMode(Options{}); got != "dbengine" {
		t.Fatalf("default stream memory mode = %q, want dbengine", got)
	}
	if got := fmt.Sprintf(streamConfTemplate, "key", "alloc"); !strings.Contains(got, "[key]") || !strings.Contains(got, "default memory mode = alloc") {
		t.Fatalf("stream configuration did not render selected memory mode:\n%s", got)
	}
}

func TestQueryRetentionRejectsMalformedNumbers(t *testing.T) {
	doc := func(first, last any) map[string]any {
		return map[string]any{"db": map[string]any{
			"first_entry": first,
			"last_entry":  last,
		}}
	}

	maxInt64Float := math.Nextafter(float64(uint64(1)<<63), 0)
	valid := []struct {
		name        string
		first, last float64
		want        Retention
	}{
		{"ordinary", 10, 20, Retention{FirstEntry: 10, LastEntry: 20}},
		{"int64 bounds", -float64(uint64(1) << 63), maxInt64Float,
			Retention{FirstEntry: -1 << 63, LastEntry: int64(maxInt64Float)}},
	}
	for _, tc := range valid {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := QueryRetention(doc(tc.first, tc.last))
			if !ok || got != tc.want {
				t.Fatalf("QueryRetention() = %+v/%v, want %+v/true", got, ok, tc.want)
			}
		})
	}

	invalid := []struct {
		name        string
		first, last any
	}{
		{"fractional first", 10.5, 20.0},
		{"fractional last", 10.0, 20.5},
		{"nan", math.NaN(), 20.0},
		{"positive infinity", 10.0, math.Inf(1)},
		{"negative infinity", math.Inf(-1), 20.0},
		{"above int64", 10.0, float64(uint64(1) << 63)},
		{"below int64", math.Nextafter(-float64(uint64(1)<<63), math.Inf(-1)), 20.0},
		{"wrong type", "10", 20.0},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			if got, ok := QueryRetention(doc(tc.first, tc.last)); ok {
				t.Fatalf("QueryRetention() accepted malformed numbers: %+v", got)
			}
		})
	}
}

func TestInfoHasDaemonIdentity(t *testing.T) {
	valid := func() map[string]any {
		return map[string]any{
			"uid": "local-guid",
			"mirrored_hosts_status": []any{
				map[string]any{
					"hostname":  "query-corpus-a",
					"hops":      0.0,
					"reachable": true,
					"guid":      "local-guid",
				},
				map[string]any{
					"hostname":  "child",
					"hops":      1.0,
					"reachable": true,
					"guid":      "child-guid",
				},
			},
		}
	}

	if err := infoHasDaemonIdentity(valid(), "query-corpus-a"); err != nil {
		t.Fatalf("valid identity rejected: %v", err)
	}

	tests := map[string]func(map[string]any){
		"missing statuses": func(doc map[string]any) {
			delete(doc, "mirrored_hosts_status")
		},
		"wrong hostname": func(doc map[string]any) {
			status := doc["mirrored_hosts_status"].([]any)[0].(map[string]any)
			status["hostname"] = "query-corpus-b"
		},
		"child hop": func(doc map[string]any) {
			status := doc["mirrored_hosts_status"].([]any)[0].(map[string]any)
			status["hops"] = 1.0
		},
		"unreachable": func(doc map[string]any) {
			status := doc["mirrored_hosts_status"].([]any)[0].(map[string]any)
			status["reachable"] = false
		},
		"guid differs from uid": func(doc map[string]any) {
			status := doc["mirrored_hosts_status"].([]any)[0].(map[string]any)
			status["guid"] = "other-guid"
		},
		"duplicate local identity": func(doc map[string]any) {
			status := doc["mirrored_hosts_status"].([]any)[0].(map[string]any)
			doc["mirrored_hosts_status"] = append(
				doc["mirrored_hosts_status"].([]any),
				map[string]any{
					"hostname": status["hostname"], "hops": 0.0,
					"reachable": true, "guid": status["guid"],
				})
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			doc := valid()
			mutate(doc)
			if err := infoHasDaemonIdentity(doc, "query-corpus-a"); err == nil {
				t.Fatal("malformed or wrong daemon identity accepted")
			}
		})
	}
}

func TestNewDaemonIdentityIsUnique(t *testing.T) {
	hostnameA, keyA, err := newDaemonIdentity()
	if err != nil {
		t.Fatal(err)
	}
	hostnameB, keyB, err := newDaemonIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if hostnameA == hostnameB || keyA == keyB {
		t.Fatalf("identities repeated: %q/%q and %q/%q", hostnameA, keyA, hostnameB, keyB)
	}
	if !strings.HasPrefix(hostnameA, "query-corpus-") || hostnameA == "" || keyA == "" {
		t.Fatalf("invalid identity %q/%q", hostnameA, keyA)
	}
}

func TestStartWithPortRetriesOnlyAutomaticCollisions(t *testing.T) {
	collision := errors.New("bind collision")
	other := errors.New("configuration failure")

	t.Run("automatic collision retries", func(t *testing.T) {
		ports := []int{12001, 12002}
		picks, attempts := 0, 0
		got, err := startWithPortRetries(
			Options{},
			func(o Options) (*Daemon, error) {
				attempts++
				if attempts == 1 {
					return nil, collision
				}
				return &Daemon{Opts: o}, nil
			},
			func() (int, error) {
				port := ports[picks]
				picks++
				return port, nil
			},
			func(err error) bool { return errors.Is(err, collision) },
		)
		if err != nil {
			t.Fatal(err)
		}
		if attempts != 2 || picks != 2 || got.Opts.Port != 12002 {
			t.Fatalf("attempts=%d picks=%d port=%d, want 2/2/12002", attempts, picks, got.Opts.Port)
		}
	})

	t.Run("explicit port fails fast", func(t *testing.T) {
		attempts, picks := 0, 0
		_, err := startWithPortRetries(
			Options{Port: 12003},
			func(Options) (*Daemon, error) {
				attempts++
				return nil, collision
			},
			func() (int, error) {
				picks++
				return 0, nil
			},
			func(err error) bool { return errors.Is(err, collision) },
		)
		if !errors.Is(err, collision) || attempts != 1 || picks != 0 {
			t.Fatalf("err=%v attempts=%d picks=%d, want collision/1/0", err, attempts, picks)
		}
	})

	t.Run("non-collision does not retry", func(t *testing.T) {
		attempts := 0
		_, err := startWithPortRetries(
			Options{},
			func(Options) (*Daemon, error) {
				attempts++
				return nil, other
			},
			func() (int, error) { return 12004, nil },
			func(err error) bool { return errors.Is(err, collision) },
		)
		if !errors.Is(err, other) || attempts != 1 {
			t.Fatalf("err=%v attempts=%d, want configuration failure/1", err, attempts)
		}
	})

	t.Run("retry exhaustion is bounded", func(t *testing.T) {
		attempts := 0
		_, err := startWithPortRetries(
			Options{},
			func(Options) (*Daemon, error) {
				attempts++
				return nil, collision
			},
			func() (int, error) { return 12005 + attempts, nil },
			func(err error) bool { return errors.Is(err, collision) },
		)
		if err == nil || attempts != autoPortAttempts {
			t.Fatalf("err=%v attempts=%d, want error/%d", err, attempts, autoPortAttempts)
		}
	})
}

type fakeProcess struct {
	signalErr error
	killErr   error
	signals   []os.Signal
	kills     int
	onSignal  func()
	onKill    func()
}

func (p *fakeProcess) Signal(signal os.Signal) error {
	p.signals = append(p.signals, signal)
	if p.onSignal != nil {
		p.onSignal()
	}
	return p.signalErr
}

func (p *fakeProcess) Kill() error {
	p.kills++
	if p.onKill != nil {
		p.onKill()
	}
	return p.killErr
}

func testStoppingDaemon(process *fakeProcess, waitCh chan error) *Daemon {
	return &Daemon{
		process:     process,
		processPID:  4242,
		waitCh:      waitCh,
		termTimeout: time.Millisecond,
		killTimeout: time.Millisecond,
	}
}

func TestStopIsCheckedAndBounded(t *testing.T) {
	t.Run("term and reap", func(t *testing.T) {
		waitCh := make(chan error, 1)
		process := &fakeProcess{onSignal: func() { waitCh <- nil }}
		d := testStoppingDaemon(process, waitCh)
		runtimeDir := t.TempDir()
		d.runtimeDir = runtimeDir

		if err := d.Stop(); err != nil {
			t.Fatal(err)
		}
		if len(process.signals) != 1 || process.signals[0] != syscall.SIGTERM || process.kills != 0 {
			t.Fatalf("signals=%v kills=%d", process.signals, process.kills)
		}
		if _, err := os.Stat(runtimeDir); !os.IsNotExist(err) {
			t.Errorf("reaped process retained its runtime directory: %v", err)
		}
	})

	t.Run("graceful wait failure is reported", func(t *testing.T) {
		waitCh := make(chan error, 1)
		waitErr := errors.New("exit status 7")
		process := &fakeProcess{onSignal: func() { waitCh <- waitErr }}
		d := testStoppingDaemon(process, waitCh)

		if err := d.Stop(); !errors.Is(err, waitErr) {
			t.Fatalf("Stop() error = %v, want wait failure", err)
		}
	})

	t.Run("term timeout escalates and reaps", func(t *testing.T) {
		waitCh := make(chan error, 1)
		process := &fakeProcess{onKill: func() { waitCh <- errors.New("signal: killed") }}
		d := testStoppingDaemon(process, waitCh)

		if err := d.Stop(); err != nil {
			t.Fatal(err)
		}
		if process.kills != 1 {
			t.Fatalf("kills=%d, want 1", process.kills)
		}
	})

	t.Run("term failure is reported", func(t *testing.T) {
		waitCh := make(chan error, 1)
		termErr := errors.New("term failed")
		process := &fakeProcess{
			signalErr: termErr,
			onKill:    func() { waitCh <- errors.New("signal: killed") },
		}
		d := testStoppingDaemon(process, waitCh)

		if err := d.Stop(); !errors.Is(err, termErr) {
			t.Fatalf("Stop() error = %v, want TERM failure", err)
		}
	})

	t.Run("kill failure is reported", func(t *testing.T) {
		waitCh := make(chan error, 1)
		killErr := errors.New("kill failed")
		process := &fakeProcess{killErr: killErr}
		d := testStoppingDaemon(process, waitCh)

		if err := d.Stop(); !errors.Is(err, killErr) {
			t.Fatalf("Stop() error = %v, want KILL failure", err)
		}
	})

	t.Run("missing reap is bounded", func(t *testing.T) {
		waitCh := make(chan error)
		process := &fakeProcess{}
		d := testStoppingDaemon(process, waitCh)
		d.runtimeDir = t.TempDir()

		started := time.Now()
		err := d.Stop()
		if err == nil || !strings.Contains(err.Error(), "reap") || !strings.Contains(err.Error(), "PID 4242") {
			t.Fatalf("Stop() error = %v, want reap timeout identifying PID 4242", err)
		}
		if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
			t.Fatalf("Stop() took %v, want bounded completion", elapsed)
		}
		if _, err := os.Stat(d.runtimeDir); err != nil {
			t.Errorf("unreaped process lost its runtime directory: %v", err)
		}
	})

	t.Run("already-finished signal is not an error after reap", func(t *testing.T) {
		waitCh := make(chan error, 1)
		waitCh <- nil
		process := &fakeProcess{signalErr: os.ErrProcessDone}
		d := testStoppingDaemon(process, waitCh)

		if err := d.Stop(); err != nil {
			t.Fatalf("Stop() error = %v", err)
		}
	})
}
