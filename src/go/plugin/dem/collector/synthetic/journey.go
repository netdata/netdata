// SPDX-License-Identifier: GPL-3.0-or-later

package synthetic

import (
	"context"
	_ "embed"
	"errors"
	"os"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/internal/syntheticjob"
	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed config_schema_journey.json
var journeySchema string

type Secret struct {
	Name  string `yaml:"name"  json:"name"`
	Value string `yaml:"value" json:"value"`
}

type JourneyConfig struct {
	BrowserErrorSummary bool             `yaml:"browser_error_summary"  json:"browser_error_summary"`
	CheckEvery          int              `yaml:"check_every"            json:"check_every"`
	Vnode               string           `yaml:"vnode,omitempty"        json:"vnode,omitempty"`
	Name                string           `yaml:"name"                   json:"name"`
	UpdateEvery         int              `yaml:"update_every,omitempty" json:"update_every"`
	Timeout             confopt.Duration `yaml:"timeout"                json:"timeout"`
	Script              string           `yaml:"script,omitempty"       json:"script,omitempty"`
	ScriptPath          string           `yaml:"script_path,omitempty"  json:"script_path,omitempty"`
	Secrets             []Secret         `yaml:"secrets,omitempty"      json:"secrets,omitempty"`
	TraceOnFailure      bool             `yaml:"trace_on_failure"       json:"trace_on_failure"`
	ScreenshotOnFailure bool             `yaml:"screenshot_on_failure"  json:"screenshot_on_failure"`
}

// Journey is the journey module: one job runs one Playwright Test entry.
type Journey struct {
	collectorapi.Base
	JourneyConfig    `yaml:",inline" json:""`
	syntheticjob.Job `yaml:"-"       json:"-"`
}

func JourneyCreator(deps Dependencies) collectorapi.Creator {
	return newCreator(
		deps,
		journeySchema,
		func() any { return &JourneyConfig{} },
		func() collectorapi.CollectorV2 { return NewJourney(deps) },
	)
}

func NewJourney(deps Dependencies) *Journey {
	return &Journey{
		JourneyConfig: JourneyConfig{
			CheckEvery:  900,
			UpdateEvery: 10,
			Timeout:     confopt.Duration(2 * time.Minute),
		},
		Job: syntheticjob.New(deps, model.Journey),
	}
}

func (c *Journey) Configuration() any        { return c.JourneyConfig }
func (c *Journey) ChartTemplateYAML() string { return charts }

// Test rehearses the submitted configuration once without activating it.
func (c *Journey) Test(ctx context.Context) error { return c.Rehearse(ctx) }

func (c *Journey) Init(ctx context.Context) error {
	return c.Job.Init(ctx, c.CheckEvery, c.UpdateEvery, c.newRequest)
}

// newRequest validates the journey options and builds the execution request. The script path shape is validated
// before the file is opened.
func (c *Journey) newRequest(vnodeGUID string) (model.Request, error) {
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
