// SPDX-License-Identifier: GPL-3.0-or-later

package hostidentity

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	sdkjournal "github.com/netdata/systemd-journal-sdk/go/journal"
)

type testProvider struct{}

func (testProvider) MachineID() sdkjournal.UUID { return sdkjournal.UUID{} }
func (testProvider) BootID() sdkjournal.UUID    { return sdkjournal.UUID{} }
func (testProvider) MonotonicUsec() uint64      { return 1 }

func TestServiceIsLazy(t *testing.T) {
	var configCalls atomic.Int64
	var loadCalls atomic.Int64
	_ = newWithLoader(
		func() LoadConfig {
			configCalls.Add(1)
			return LoadConfig{}
		},
		func(LoadConfig) (Provider, error) {
			loadCalls.Add(1)
			return testProvider{}, nil
		},
	)
	if configCalls.Load() != 0 || loadCalls.Load() != 0 {
		t.Fatalf("construction loaded identity: config=%d load=%d", configCalls.Load(), loadCalls.Load())
	}
}

func TestFreshJournalLoadsEveryAttempt(t *testing.T) {
	var calls atomic.Int64
	service := newWithLoader(
		func() LoadConfig { return LoadConfig{StateDir: "/state"} },
		func(LoadConfig) (Provider, error) {
			if calls.Add(1) == 1 {
				return nil, errors.New("unavailable")
			}
			return testProvider{}, nil
		},
	)

	if _, err := service.FreshJournal(); err == nil || err.Error() != "load local journal host identity using state directory /state: unavailable" {
		t.Fatalf("first FreshJournal error = %v", err)
	}
	if _, err := service.FreshJournal(); err != nil {
		t.Fatalf("second FreshJournal failed: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("loader calls = %d, want 2", calls.Load())
	}
}

func TestCachedFallbackCachesSuccessUnderConcurrency(t *testing.T) {
	var calls atomic.Int64
	provider := testProvider{}
	service := newWithLoader(
		func() LoadConfig { return LoadConfig{} },
		func(LoadConfig) (Provider, error) {
			calls.Add(1)
			return provider, nil
		},
	)

	var wg sync.WaitGroup
	for range 64 {
		wg.Go(func() {
			got, err := service.CachedFallback()
			if err != nil || got != provider {
				t.Errorf("CachedFallback = %v/%v", got, err)
			}
		})
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("loader calls = %d, want 1", calls.Load())
	}
}

func TestCachedFallbackCachesErrorButDoesNotPoisonFresh(t *testing.T) {
	wantErr := errors.New("cached failure")
	var calls atomic.Int64
	service := newWithLoader(
		func() LoadConfig { return LoadConfig{} },
		func(LoadConfig) (Provider, error) {
			if calls.Add(1) == 1 {
				return nil, wantErr
			}
			return testProvider{}, nil
		},
	)

	for range 2 {
		if _, err := service.CachedFallback(); !errors.Is(err, wantErr) {
			t.Fatalf("CachedFallback error = %v, want %v", err, wantErr)
		}
	}
	if _, err := service.FreshJournal(); err != nil {
		t.Fatalf("FreshJournal after cached failure: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("loader calls = %d, want 2", calls.Load())
	}
}

func TestFreshFailureDoesNotPoisonCachedFallback(t *testing.T) {
	wantErr := errors.New("fresh failure")
	var calls atomic.Int64
	service := newWithLoader(
		func() LoadConfig { return LoadConfig{} },
		func(LoadConfig) (Provider, error) {
			if calls.Add(1) == 1 {
				return nil, wantErr
			}
			return testProvider{}, nil
		},
	)

	if _, err := service.FreshJournal(); !errors.Is(err, wantErr) {
		t.Fatalf("FreshJournal error = %v, want %v", err, wantErr)
	}
	if _, err := service.CachedFallback(); err != nil {
		t.Fatalf("CachedFallback after fresh failure: %v", err)
	}
}
