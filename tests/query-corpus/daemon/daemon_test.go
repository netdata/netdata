// SPDX-License-Identifier: GPL-3.0-or-later

package daemon

import (
	"errors"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

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
		waitCh:      waitCh,
		termTimeout: time.Millisecond,
		killTimeout: time.Millisecond,
	}
}

func TestStopIsCheckedAndBounded(t *testing.T) {
	t.Run("term and reap", func(t *testing.T) {
		waitCh := make(chan error, 1)
		process := &fakeProcess{onSignal: func() { waitCh <- errors.New("signal: terminated") }}
		d := testStoppingDaemon(process, waitCh)

		if err := d.Stop(); err != nil {
			t.Fatal(err)
		}
		if len(process.signals) != 1 || process.signals[0] != syscall.SIGTERM || process.kills != 0 {
			t.Fatalf("signals=%v kills=%d", process.signals, process.kills)
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

		started := time.Now()
		err := d.Stop()
		if err == nil || !strings.Contains(err.Error(), "reap") {
			t.Fatalf("Stop() error = %v, want reap timeout", err)
		}
		if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
			t.Fatalf("Stop() took %v, want bounded completion", elapsed)
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
