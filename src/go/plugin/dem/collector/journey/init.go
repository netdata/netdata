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
	return c.Job.Init(ctx, c.CheckEvery, c.UpdateEvery, c.newRequest)
}

// newRequest validates the journey options and builds the execution request. The script path shape is validated
// before the file is opened.
func (c *Collector) newRequest(vnodeGUID string) (model.Request, error) {
	if c.TraceOnFailure && !c.ScreenshotOnFailure {
		return model.Request{}, errors.New("trace_on_failure requires screenshot_on_failure")
	}
	secrets := make(map[string]string, len(c.Secrets))
	for _, secret := range c.Secrets {
		if _, exists := secrets[secret.Name]; exists {
			return model.Request{}, errors.New("secret names must be unique")
		}
		secrets[secret.Name] = secret.Value
	}
	request := model.Request{
		Kind:                model.Journey,
		VnodeGUID:           vnodeGUID,
		Name:                c.Name,
		Script:              c.Script,
		ScriptPath:          c.ScriptPath,
		Secrets:             secrets,
		Timeout:             time.Duration(c.Timeout),
		Capture:             c.ScreenshotOnFailure,
		Trace:               c.TraceOnFailure,
		BrowserErrorSummary: c.BrowserErrorSummary,
	}
	if err := model.ValidateRequest(request); err != nil {
		return model.Request{}, err
	}
	if c.ScriptPath != "" {
		if err := checkScriptFile(c.ScriptPath); err != nil {
			return model.Request{}, err
		}
	}
	return request, nil
}

// checkScriptFile requires a regular file both before and after opening it.
func checkScriptFile(path string) error {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("script_path must reference a readable regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return errors.New("script_path must be readable")
	}
	info, err = f.Stat()
	_ = f.Close()
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("script_path must reference a regular file")
	}
	return nil
}
