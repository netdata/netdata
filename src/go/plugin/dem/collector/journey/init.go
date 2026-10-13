// SPDX-License-Identifier: GPL-3.0-or-later
package journey

import (
	"context"
	"errors"
	"os"
	"time"

	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

func (c *Collector) Init(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.deps.Executor == nil || c.deps.Registry == nil {
		return errors.New("missing synthetic executor or observation registry")
	}
	if c.UpdateEvery <= 0 {
		return errors.New("update_every must be positive")
	}
	secrets := make(map[string]string, len(c.Secrets))
	for _, secret := range c.Secrets {
		if _, exists := secrets[secret.Name]; exists {
			return errors.New("secret names must be unique")
		}
		secrets[secret.Name] = secret.Value
	}
	request := model.Request{
		Kind:       model.Journey,
		Name:       c.Name,
		Script:     c.Script,
		ScriptPath: c.ScriptPath,
		Secrets:    secrets,
		Timeout:    time.Duration(c.Timeout),
		Capture:    c.ScreenshotOnFailure,
	}
	if err := model.ValidateRequest(request); err != nil {
		return err
	}
	if c.ScriptPath != "" {
		info, err := os.Stat(c.ScriptPath)
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("script_path must reference a readable regular file")
		}
		f, err := os.Open(c.ScriptPath)
		if err != nil {
			return errors.New("script_path must be readable")
		}
		info, err = f.Stat()
		_ = f.Close()
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("script_path must reference a regular file")
		}
	}
	c.request = request
	c.initialized = true
	return nil
}
func (c *Collector) Check(ctx context.Context) error {
	if !c.initialized {
		return errors.New("collector is not initialized")
	}
	// Preparation has a fixed budget independent of the public attempt timeout.
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return c.deps.Executor.Check(ctx, c.request.Kind)
}
func (c *Collector) Cleanup(context.Context) {}
