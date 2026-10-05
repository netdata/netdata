// SPDX-License-Identifier: GPL-3.0-or-later

package control

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

// Controller owns process-lifetime DynCfg state. Collector service instances
// only borrow configuration snapshots and report revision-specific readiness.
type Controller struct {
	mu       sync.Mutex
	input    io.ReadCloser
	output   io.Writer
	endpoint string
	storage  string
	jobs     map[string]*Job
	revision uint64
	changed  chan struct{}
	terminal chan struct{}
	err      error
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	started  bool
}

func New(input io.ReadCloser, output io.Writer, endpoint, storage string) (*Controller, error) {
	if input == nil || output == nil || !filepath.IsAbs(storage) {
		return nil, errors.New("input, output and an absolute state directory are required")
	}
	if _, _, err := net.SplitHostPort(endpoint); err != nil {
		return nil, errors.New("OTLP endpoint must be host:port")
	}
	return &Controller{
		input: input, output: output, endpoint: endpoint, storage: storage,
		jobs: map[string]*Job{}, revision: 1,
		changed: make(chan struct{}), terminal: make(chan struct{}),
	}, nil
}

func (c *Controller) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.started {
		return nil
	}
	capsule, err := functions.NewInputCapsule(c.input)
	if err != nil {
		return err
	}
	if err := c.emitLocked(func(api *netdataapi.API) {
		for _, kind := range []string{"filelogs", "hostmetrics"} {
			api.CONFIGCREATE(netdataapi.ConfigOpts{
				ID: Prefix + kind, Status: "accepted", ConfigType: "template", Path: "/collectors/otel-poc",
				SourceType: "internal", Source: "otel-facade-poc", SupportedCommands: "schema add test",
			})
		}
	}); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel, c.started = cancel, true
	c.wg.Add(2)
	go func() {
		defer c.wg.Done()
		err := capsule.Run(ctx, c)
		if err == nil {
			err = io.EOF
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		c.failLocked(err)
	}()
	go func() {
		defer c.wg.Done()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-c.terminal:
				return
			case <-ticker.C:
				c.mu.Lock()
				if _, err := io.WriteString(c.output, "PLUGIN_KEEPALIVE\n"); err != nil {
					c.failLocked(err)
				}
				c.mu.Unlock()
			}
		}
	}()
	return nil
}

func (c *Controller) Close() error {
	c.mu.Lock()
	if c.cancel != nil {
		c.cancel()
	}
	c.failLocked(io.EOF)
	c.mu.Unlock()
	// This process owns stdin. Closing it releases the parser during shutdown.
	err := c.input.Close()
	c.wg.Wait()
	return err
}

func (c *Controller) failLocked(err error) {
	if c.err == nil {
		c.err = err
		close(c.terminal)
	}
}

// Snapshot atomically captures the config and its one-shot invalidation channel.
// Changes during the resolver's Close -> Retrieve gap are included in the map.
func (c *Controller) Snapshot() (map[string]any, <-chan struct{}, <-chan struct{}, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return nil, nil, nil, c.err
	}
	return c.configLocked(), c.changed, c.terminal, nil
}

func (c *Controller) Ready(revision uint64, ready bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if revision != c.revision || c.err != nil {
		return nil
	}
	return c.emitLocked(func(api *netdataapi.API) {
		for _, id := range c.jobIDsLocked() {
			job := c.jobs[id]
			if !job.Enabled {
				continue
			}
			status := "accepted"
			if ready {
				status = "running"
			}
			if job.Status != status {
				job.Status = status
				api.CONFIGSTATUS(id, status)
			}
		}
	})
}

// Every frame, including readiness and keepalive, shares this owner lock.
func (c *Controller) emitLocked(encode func(*netdataapi.API)) error {
	var frame bytes.Buffer
	encode(netdataapi.New(&frame))
	n, err := c.output.Write(frame.Bytes())
	if err == nil && n != frame.Len() {
		err = io.ErrShortWrite
	}
	if err != nil {
		c.failLocked(err)
	}
	return err
}

func result(api *netdataapi.API, uid string, code int, payload any) {
	data, _ := json.Marshal(payload)
	api.FUNCRESULT(netdataapi.FunctionResult{
		UID: uid, Code: strconv.Itoa(code), ContentType: "application/json", ExpireTimestamp: "0", Payload: string(data),
	})
}

func (c *Controller) HandleCall(ctx context.Context, call functions.Call) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return c.err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// All commands are bounded local validation/state changes, never pipeline startup.
	code, payload, notify, reload := c.command(call)
	if reload {
		c.revision++
		for _, job := range c.jobs {
			if job.Enabled {
				job.Status = "accepted"
			}
		}
	}
	if err := c.emitLocked(func(api *netdataapi.API) {
		result(api, call.UID, code, payload)
		if notify != nil {
			notify(api)
		}
		if reload {
			for _, id := range c.jobIDsLocked() {
				if c.jobs[id].Enabled {
					api.CONFIGSTATUS(id, "accepted")
				}
			}
		}
	}); err != nil {
		return err
	}
	if reload {
		// Notify only after the successful acceptance response. Closing a channel
		// coalesces changes without blocking command ingress on Collector reload.
		close(c.changed)
		c.changed = make(chan struct{})
	}
	return nil
}

type notification func(*netdataapi.API)

func (c *Controller) command(call functions.Call) (int, any, notification, bool) {
	reject := func(code int, message string) (int, any, notification, bool) {
		return code, map[string]any{"status": code, "message": message}, nil, false
	}
	if call.Method != "config" || len(call.Args) < 2 {
		return reject(400, "expected config <id> <command>")
	}
	id, command := call.Args[0], call.Args[1]
	parts := strings.Split(strings.TrimPrefix(id, Prefix), ":")
	if !strings.HasPrefix(id, Prefix) || len(parts) > 2 || (parts[0] != "filelogs" && parts[0] != "hostmetrics") {
		return reject(404, "unknown configuration")
	}
	kind := parts[0]
	job := c.jobs[id]
	if len(parts) == 2 && job == nil {
		return reject(404, "job not found")
	}
	if command == "test" && len(parts) == 1 && len(call.Args) == 3 {
		if !validName.MatchString(call.Args[2]) {
			return reject(400, "invalid test job name")
		}
	} else if command != "add" && len(call.Args) != 2 {
		return reject(400, "unexpected command arguments")
	}
	switch command {
	case "schema":
		return 200, schema(kind), nil, false
	case "get":
		if job == nil {
			return reject(400, "get requires a job")
		}
		return 200, job.Config, nil, false
	case "test", "add", "update":
		if !call.HasPayload || call.ContentType != "application/json" {
			return reject(400, "application/json payload required")
		}
		cfg, err := parseConfig(kind, call.Payload)
		if err != nil {
			return reject(400, err.Error())
		}
		if command == "test" {
			return reject(200, "configuration validated; runtime resources were not acquired")
		}
		if command == "add" {
			if job != nil || len(parts) != 1 || len(call.Args) != 3 || !validName.MatchString(call.Args[2]) {
				return reject(400, "add requires a template and an ASCII job name containing letters, digits, _ or -")
			}
			name := call.Args[2]
			newID := Prefix + kind + ":" + name
			if c.jobs[newID] != nil {
				return reject(409, "job already exists")
			}
			job = &Job{Kind: kind, Name: name, Config: cfg, Status: "accepted"}
			c.jobs[newID] = job
			return 202, map[string]any{"status": 202, "message": "job accepted; waiting for enable or disable"}, func(api *netdataapi.API) {
				api.CONFIGCREATE(netdataapi.ConfigOpts{
					ID: newID, Status: job.Status, ConfigType: "job", Path: "/collectors/otel-poc",
					SourceType: "dyncfg", Source: "otel-facade-poc",
					SupportedCommands: "schema get update test enable disable remove restart",
				})
			}, false
		}
		if job == nil || !job.Decided {
			return reject(400, "update requires a job with an enable or disable decision")
		}
		if reflect.DeepEqual(cfg, job.Config) {
			code := 200
			if job.Enabled && job.Status != "running" {
				code = 202
			}
			return reject(code, "configuration unchanged")
		}
		job.Config = cfg
		if job.Enabled {
			return 202, map[string]any{"status": 202, "message": "configuration accepted; reloading pipelines"}, nil, true
		}
		return reject(200, "disabled configuration updated")
	case "enable":
		if job == nil {
			return reject(400, "enable requires a job")
		}
		if job.Enabled {
			code := 202
			if job.Status == "running" {
				code = 200
			}
			return reject(code, "job already enabled")
		}
		job.Enabled, job.Decided = true, true
		return 202, map[string]any{"status": 202, "message": "enabled intent accepted; reloading pipelines"}, nil, true
	case "disable":
		if job == nil {
			return reject(400, "disable requires a job")
		}
		reload := job.Enabled
		job.Enabled, job.Decided, job.Status = false, true, "disabled"
		return 200, map[string]any{"status": 200, "message": "disabled intent accepted; active pipelines stop during reload"}, func(api *netdataapi.API) {
			api.CONFIGSTATUS(id, "disabled")
		}, reload
	case "remove":
		if job == nil {
			return reject(400, "remove requires a job")
		}
		delete(c.jobs, id)
		return 200, map[string]any{"status": 200, "message": "job removed; active pipelines stop during reload"}, func(api *netdataapi.API) {
			api.CONFIGDELETE(id)
		}, job.Enabled
	case "restart":
		if job == nil || !job.Enabled {
			return reject(400, "restart requires an enabled job")
		}
		return 202, map[string]any{"status": 202, "message": "Collector service reload requested"}, nil, true
	default:
		return reject(400, "unsupported command")
	}
}

func (c *Controller) HandleCancel(context.Context, string) error { return nil }

func (c *Controller) HandleReject(_ context.Context, uid string, code int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.emitLocked(func(api *netdataapi.API) {
		result(api, uid, code, map[string]any{"status": code, "message": "invalid Function frame"})
	})
}

func (c *Controller) HandleQuit(context.Context) error { return nil }

func (c *Controller) TerminalError() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err == nil {
		return fmt.Errorf("DynCfg input closed")
	}
	return c.err
}
