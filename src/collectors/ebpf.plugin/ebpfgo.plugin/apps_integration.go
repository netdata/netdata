// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"fmt"
	"sync"
)

// AppsIntegrationService coordinates shared memory publication for apps/cgroups integration.
// All collectors share a single ebpfSharedMemoryStore and SHM segment.
// This service manages the lifecycle and coordination.
type AppsIntegrationService struct {
	mu       sync.RWMutex
	store    *ebpfSharedMemoryStore
	shmPublisher *SharedPidMemoryPublisher
	pidTableSize uint32
	updateEvery  int
}

var (
	appsInstance *AppsIntegrationService
	appsOnce     sync.Once
)

// GetAppsIntegration returns the singleton apps/cgroups coordination service.
func GetAppsIntegration() *AppsIntegrationService {
	appsOnce.Do(func() {
		appsInstance = &AppsIntegrationService{
			pidTableSize: defaultPidTableSize,
			updateEvery: 1,
		}
	})
	return appsInstance
}

// Store returns the shared ebpfSharedMemoryStore for all collectors.
// Lazily initializes on first access.
func (a *AppsIntegrationService) Store() *ebpfSharedMemoryStore {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.store == nil {
		a.store = NewEbpfSharedMemoryStore()
	}
	return a.store
}

// PublisherSharedMemory opens or returns the singleton SHM segment.
// Called by collector Publish() to write per-app/cgroup data.
func (a *AppsIntegrationService) PublisherSharedMemory() (*SharedPidMemoryPublisher, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Lazy open: retry every cycle so transient failures self-heal
	if a.shmPublisher == nil {
		pub, err := NewSharedPidMemoryPublisher(
			productionSHMName,
			productionSEMName,
			a.pidTableSize,
			uint32(a.updateEvery),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to open SHM: %w", err)
		}
		a.shmPublisher = pub
	}
	return a.shmPublisher, nil
}

// Close closes the SHM publisher (called during shutdown).
func (a *AppsIntegrationService) Close() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.shmPublisher != nil {
		_ = a.shmPublisher.Close()
		a.shmPublisher = nil
	}
}

// SetUpdateEvery configures the collection interval (used for SHM freshness).
func (a *AppsIntegrationService) SetUpdateEvery(updateEvery int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.updateEvery = updateEvery
}

// defaultPidTableSize matches the legacy collectors' SHM PID table size.
const defaultPidTableSize = 10000
