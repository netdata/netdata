// SPDX-License-Identifier: GPL-3.0-or-later

package collector

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/java/protocol"
)

type targetState struct {
	protocol.Process
	Status, Detail, Runtime string
	LastSeen                time.Time
}

type admission struct{ instance, application string }

func (c *Collector) scan(ctx context.Context, j *journal) error {
	found, err := c.helper.discover(ctx)
	if err != nil {
		return err
	}
	live := make(map[string]bool, len(found))
	for _, t := range found {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if t.BootID != j.state.BootID {
			return fmt.Errorf("discovery boot identity changed")
		}
		key := t.Instance()
		live[key] = true
		state := targetState{Process: t}
		switch {
		case c.excluded(t.Application):
			state.Status, state.Detail = "Excluded", "Excluded by application name; loaded instrumentation requires an application restart to unload"
			c.revoke(key)
		case !t.Eligible:
			state.Status, state.Detail = "Unsupported", t.Reason
			c.revoke(key)
		default:
			a, exists := j.state.Attempts[key]
			if !exists {
				token, err := newToken()
				if err != nil {
					return err
				}
				a = attempt{Process: t, Token: token, Status: protocol.Unknown, Detail: "Attachment recorded before launch; outcome unknown until acknowledged"}
				j.state.Attempts[key] = a
				if err := j.save(); err != nil {
					return err
				}
			}
			// Bind credentials before loading the agent; its first export may arrive
			// before the helper acknowledges attachment.
			c.admit(a)
			if !exists {
				c.setTarget(key, targetState{Process: t, Status: "Attaching", Detail: a.Detail})
				result := c.helper.attach(ctx, protocol.AttachRequest{PID: t.PID, StartTime: t.StartTime, BootID: t.BootID, Token: a.Token, Port: j.state.Port})
				a.Status, a.Detail = result.Status, result.Detail
				j.state.Attempts[key] = a
				if err := j.save(); err != nil {
					return err
				}
			}
			state.Status, state.Detail = a.Status, a.Detail
		}
		c.setTarget(key, state)
	}
	c.mu.Lock()
	var removed []string
	for key := range c.targets {
		if !live[key] {
			removed = append(removed, key)
			delete(c.targets, key)
		}
	}
	c.mu.Unlock()
	for _, key := range removed {
		c.revoke(key)
	}
	// An omitted discovery row alone is not proof of exit. Forget attempts only
	// when procfs confirms the identity is gone, never after a permission error.
	changed := false
	for key, a := range j.state.Attempts {
		if !live[key] && processGone(c.runtime.ProcDir, a.Process) {
			delete(j.state.Attempts, key)
			changed = true
		}
	}
	if changed {
		return j.save()
	}
	return nil
}

func processGone(root string, p protocol.Process) bool {
	data, err := os.ReadFile(filepath.Join(root, strconv.Itoa(p.PID), "stat"))
	if err != nil {
		return os.IsNotExist(err)
	}
	end := strings.LastIndex(string(data), ") ")
	if end < 0 {
		return false
	}
	fields := strings.Fields(string(data)[end+2:])
	if len(fields) <= 19 {
		return false
	}
	start, err := strconv.ParseUint(fields[19], 10, 64)
	return err == nil && start != p.StartTime
}

func (c *Collector) admit(a attempt) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.credentials[sha256.Sum256([]byte(a.Token))] = admission{instance: a.Process.Instance(), application: a.Process.Application}
	c.ingress.Admit(a.Process.Instance(), a.Process.Application)
}

func (c *Collector) revoke(instance string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for token, a := range c.credentials {
		if a.instance == instance {
			delete(c.credentials, token)
		}
	}
	c.ingress.Remove(instance)
}

func (c *Collector) setTarget(key string, state targetState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	old := c.targets[key]
	state.LastSeen, state.Runtime = old.LastSeen, old.Runtime
	c.targets[key] = state
}
