// SPDX-License-Identifier: GPL-3.0-or-later

// Package ipmiapi reads IPMI sensors without changing BMC configuration.
package ipmiapi

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bougou/go-ipmi/pkg/client"
	"github.com/bougou/go-ipmi/pkg/command/app"
	"github.com/bougou/go-ipmi/pkg/command/sensor"
	"github.com/bougou/go-ipmi/pkg/command/storage"
	"github.com/bougou/go-ipmi/pkg/types"
)

type Config struct {
	Driver   string
	Device   int32
	Hostname string
	Port     int
	Username string
	Password string
	Timeout  time.Duration
}

type Snapshot struct {
	Sensors     []Sensor
	SEL         *float64
	CollectedAt time.Time
	Warnings    []string
}

type Sensor struct {
	Key, Name, Type, Component, Unit, Metric, State string
	Value                                           *float64
}

// transport keeps tests at the command/response boundary, including wire decoding.
type transport interface {
	Connect(context.Context) error
	Exchange(context.Context, types.Request, types.Response) error
	Close(context.Context) error
}

type connection struct {
	*client.Client
	config Config
}

func (c *connection) Connect(ctx context.Context) error {
	if c.config.Driver == "open" {
		return c.ConnectOpen(ctx, c.config.Device)
	}
	return c.Client.Connect(ctx)
}

// Reader serializes calls because the client owns mutable session state.
type Reader struct {
	mu            sync.Mutex
	config        Config
	factory       func() (transport, error)
	conn          transport
	inventory     []descriptor
	inventoryInfo storage.GetSDRRepoInfoResponse
	inventoryAt   time.Time
	now           func() time.Time
}

func New(config Config) (*Reader, error) {
	switch config.Driver {
	case "open", "lan", "lanplus":
	default:
		return nil, fmt.Errorf("unsupported IPMI driver %q", config.Driver)
	}
	if config.Device < 0 {
		return nil, errors.New("IPMI device must be nonnegative")
	}
	if config.Timeout <= 0 {
		return nil, errors.New("IPMI timeout must be positive")
	}
	if config.Driver != "open" && (config.Hostname == "" || config.Port < 1 || config.Port > 65535) {
		return nil, errors.New("remote IPMI requires a hostname and port 1–65535")
	}
	// Avoid upstream validation errors that echo the username.
	if len(config.Username) > 16 {
		return nil, errors.New("IPMI username exceeds 16 bytes")
	}
	r := &Reader{config: config, now: time.Now}
	r.factory = func() (transport, error) {
		var c *client.Client
		var err error
		if config.Driver == "open" {
			// NewClient initializes LAN sessions only; changing its interface does not
			// initialize the OpenIPMI state that ConnectOpen requires.
			c, err = client.NewOpenClient()
		} else {
			c, err = client.NewClient(config.Hostname, config.Port, config.Username, config.Password)
		}
		if err != nil {
			return nil, errors.New("create IPMI client failed")
		}
		c.WithInterface(client.Interface(config.Driver)).WithTimeout(config.Timeout).WithRetry(0).WithMaxPrivilegeLevel(types.PrivilegeLevelUser)
		return &connection{Client: c, config: config}, nil
	}
	return r, nil
}

func (r *Reader) ensure(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.conn != nil {
		return nil
	}
	c, err := r.factory()
	if err != nil {
		return err
	}
	connectCtx, cancel := context.WithTimeout(ctx, r.config.Timeout)
	defer cancel()
	if err = c.Connect(connectCtx); err != nil {
		closeCtx, closeCancel := context.WithTimeout(context.WithoutCancel(ctx), r.config.Timeout)
		_ = c.Close(closeCtx)
		closeCancel()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("connect IPMI: %w", r.safeError(err))
	}
	r.conn = c
	return nil
}

func (r *Reader) exchange(ctx context.Context, req types.Request, res types.Response) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	commandCtx, cancel := context.WithTimeout(ctx, r.config.Timeout)
	defer cancel()
	err := r.conn.Exchange(commandCtx, req, res)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return r.safeError(err)
}

func (r *Reader) Check(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ensure(ctx); err != nil {
		return err
	}
	err := r.exchange(ctx, &app.GetDeviceIDRequest{}, &app.GetDeviceIDResponse{})
	if err != nil {
		r.disconnect(ctx)
		return fmt.Errorf("get IPMI device ID: %w", err)
	}
	return nil
}

func (r *Reader) disconnect(ctx context.Context) {
	if r.conn != nil {
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), r.config.Timeout)
		_ = r.conn.Close(closeCtx)
		cancel()
		r.conn = nil
	}
	// Reconnection must discover the BMC's current inventory.
	r.inventoryAt = time.Time{}
}

func (r *Reader) Close(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.conn == nil {
		return nil
	}
	err := r.conn.Close(ctx)
	r.conn = nil
	r.inventoryAt = time.Time{}
	return r.safeError(err)
}

// Warning counts have a fixed vocabulary, so a large SDR cannot flood logs.
type warningCounts struct {
	unsupported, reading, conversion int
	sel                              bool
}

func (w warningCounts) strings() []string {
	var out []string
	if w.unsupported > 0 {
		out = append(out, fmt.Sprintf("%d sensor records have unsupported ownership, sharing, or ID encoding", w.unsupported))
	}
	if w.reading > 0 {
		out = append(out, fmt.Sprintf("%d sensor readings are unavailable or incomplete", w.reading))
	}
	if w.conversion > 0 {
		out = append(out, fmt.Sprintf("%d sensor values have unsupported units/conversion or invalid results", w.conversion))
	}
	if w.sel {
		out = append(out, "SEL entry count unavailable")
	}
	return out
}

func (r *Reader) Collect(ctx context.Context, collectSEL bool) (_ *Snapshot, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err = r.ensure(ctx); err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			r.disconnect(ctx)
		}
	}()
	if err = r.refresh(ctx); err != nil {
		return nil, fmt.Errorf("read IPMI inventory: %w", err)
	}
	snapshot := &Snapshot{CollectedAt: r.now()}
	var warnings warningCounts
	for _, d := range r.inventory {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		s := d.sensor
		if !d.supported {
			warnings.unsupported++
			snapshot.Sensors = append(snapshot.Sensors, s)
			continue
		}
		commandCtx := client.WithCommandContext(ctx, (&client.CommandContext{}).WithResponderAddr(0x20).WithResponderLUN(d.owner.LUN()))
		var reading readingResponse
		if err = r.exchange(commandCtx, &sensor.GetSensorReadingRequest{SensorNumber: d.number}, &reading); err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if _, ok := types.IsResponseError(err); !ok {
				return nil, fmt.Errorf("read IPMI sensor: %w", err)
			}
			warnings.reading++
			snapshot.Sensors = append(snapshot.Sensors, s)
			continue
		}
		if reading.ReadingUnavailable || reading.SensorScanningDisabled {
			warnings.reading++
			snapshot.Sensors = append(snapshot.Sensors, s)
			continue
		}
		if reading.length < 3 {
			warnings.reading++
		} else {
			s.State = sensorState(d.event, d.kind, reading)
		}
		if d.analog {
			if d.sensor.Metric == "" {
				warnings.conversion++
			} else {
				factors := d.factors
				linear := d.linear
				valid := true
				if linear == types.LinearizationFunc_NonLinear {
					var res sensor.GetSensorReadingFactorsResponse
					if factorErr := r.exchange(commandCtx, &sensor.GetSensorReadingFactorsRequest{SensorNumber: d.number, Reading: reading.Reading}, &res); factorErr != nil {
						if ctx.Err() != nil {
							return nil, ctx.Err()
						}
						if _, ok := types.IsResponseError(factorErr); !ok {
							return nil, fmt.Errorf("read IPMI conversion factors: %w", factorErr)
						}
						valid = false
					} else {
						factors = res.ReadingFactors
						linear = types.LinearizationFunc_Linear
					}
				}
				if valid {
					s.Value = convertValue(reading.Reading, d.unit, factors, linear)
				}
				if s.Value == nil {
					warnings.conversion++
				}
			}
		}
		snapshot.Sensors = append(snapshot.Sensors, s)
	}
	if collectSEL {
		var res storage.GetSELInfoResponse
		if selErr := r.exchange(ctx, &storage.GetSELInfoRequest{}, &res); selErr != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			warnings.sel = true
		} else {
			value := float64(res.Entries)
			snapshot.SEL = &value
		}
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	snapshot.Warnings = warnings.strings()
	return snapshot, nil
}

// Get Sensor Reading makes status bytes optional; upstream currently loses their presence.
type readingResponse struct {
	sensor.GetSensorReadingResponse
	length int
}

func (r *readingResponse) Unpack(data []byte) error {
	r.length = len(data)
	return r.GetSensorReadingResponse.Unpack(data)
}

// Preserve error classification while preventing SDK errors from echoing credentials.
type redactedError struct {
	cause   error
	message string
}

func (e redactedError) Error() string { return e.message }
func (e redactedError) Unwrap() error { return e.cause }
func (r *Reader) safeError(err error) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	for _, secret := range []string{r.config.Password, r.config.Username} {
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "[redacted]")
		}
	}
	return redactedError{cause: err, message: message}
}
