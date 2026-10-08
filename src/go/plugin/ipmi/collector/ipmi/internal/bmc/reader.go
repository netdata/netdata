// SPDX-License-Identifier: GPL-3.0-or-later

// Package bmc reads the sensors and the System Event Log size of the local BMC
// through Linux OpenIPMI. It never changes BMC configuration.
package bmc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bougou/go-ipmi/pkg/command/app"
	"github.com/bougou/go-ipmi/pkg/command/storage"
	"github.com/bougou/go-ipmi/pkg/types"
)

// Config selects the local device and bounds each IPMI command. The caller validates it.
type Config struct {
	// Device is the OpenIPMI device number (/dev/ipmi<N>).
	Device int32
	// Timeout bounds each command, including the SDK's blocking receive.
	Timeout time.Duration
}

// Reader reads the BMC one call at a time because the SDK client owns mutable
// state. Collect opens the device on demand and reopens it after a failure;
// Check closes it again after its probe.
type Reader struct {
	mu  sync.Mutex
	cfg Config
	now func() time.Time

	newTransport func() (transport, error)
	conn         transport

	// The SDR inventory is cached between repository changes; see refreshInventory.
	inventory       []descriptor
	inventoryRepo   storage.GetSDRRepoInfoResponse
	inventoryReadAt time.Time
}

// New returns a Reader. It performs no I/O.
func New(cfg Config) *Reader {
	return &Reader{
		cfg:          cfg,
		now:          time.Now,
		newTransport: func() (transport, error) { return newOpenTransport(cfg) },
	}
}

// Check opens the device, probes the BMC with Get Device ID and closes the
// device again, so a probe leaves nothing to release.
func (r *Reader) Check(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.connect(ctx); err != nil {
		return err
	}
	probeErr := r.exchange(ctx, &app.GetDeviceIDRequest{}, &app.GetDeviceIDResponse{})
	closeErr := r.disconnect(ctx)
	if probeErr != nil {
		return fmt.Errorf("get IPMI device ID: %w", probeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close IPMI device: %w", closeErr)
	}
	return nil
}

// Collect reads every inventory sensor and, when requested, the SEL entry
// count. A completion-code error or an unusable reading of one sensor, and any
// SEL failure, degrade only that data and are summarized in Snapshot.Warnings.
// Inventory failures, other sensor command failures and cancellation fail the
// collection and close the device.
func (r *Reader) Collect(ctx context.Context, collectSEL bool) (_ *Snapshot, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.connect(ctx); err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = r.disconnect(ctx)
		}
	}()

	if err := r.refreshInventory(ctx); err != nil {
		return nil, fmt.Errorf("read IPMI inventory: %w", err)
	}

	var warnings collectionWarnings
	snapshot := &Snapshot{
		Sensors:     make([]Sensor, 0, len(r.inventory)),
		CollectedAt: r.now(),
	}
	for _, d := range r.inventory {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		s, err := r.readSensor(ctx, d, &warnings)
		if err != nil {
			return nil, err
		}
		snapshot.Sensors = append(snapshot.Sensors, s)
	}
	if collectSEL {
		snapshot.SELEntries = r.readSELEntries(ctx, &warnings)
	}
	// A cancellation during the fail-soft SEL read surfaces here.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	snapshot.Warnings = warnings.messages()
	return snapshot, nil
}

// Close closes the device. It is safe to call more than once.
func (r *Reader) Close(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.disconnect(ctx)
}

// connect opens the device unless it is already open.
func (r *Reader) connect(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.conn != nil {
		return nil
	}

	conn, err := r.newTransport()
	if err != nil {
		return err
	}
	connectCtx, cancel := context.WithTimeout(ctx, r.cfg.Timeout)
	defer cancel()
	if err := conn.Connect(connectCtx); err != nil {
		_ = conn.Close(ctx)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("connect IPMI: %w", err)
	}
	r.conn = conn
	return nil
}

// disconnect closes the device. The next connection rediscovers the inventory,
// because it may have changed while the device was closed.
func (r *Reader) disconnect(ctx context.Context) error {
	r.inventoryReadAt = time.Time{}
	if r.conn == nil {
		return nil
	}
	err := r.conn.Close(ctx)
	r.conn = nil
	return err
}

// exchange sends one command bounded by the configured timeout. The caller's
// cancellation takes precedence over the command's own error.
func (r *Reader) exchange(ctx context.Context, req types.Request, res types.Response) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	cmdCtx, cancel := context.WithTimeout(ctx, r.cfg.Timeout)
	defer cancel()

	err := r.conn.Exchange(cmdCtx, req, res)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return err
}

// readSELEntries returns the raw SEL entry count, or nil when the BMC does not provide it.
func (r *Reader) readSELEntries(ctx context.Context, warnings *collectionWarnings) *int {
	var res storage.GetSELInfoResponse
	if err := r.exchange(ctx, &storage.GetSELInfoRequest{}, &res); err != nil {
		warnings.selUnavailable = true
		return nil
	}
	entries := int(res.Entries)
	return &entries
}
